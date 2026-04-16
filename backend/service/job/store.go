package job

import (
	"strings"
	"time"

	"github.com/trushilpatel/jobradar/model"
	"gorm.io/gorm"
)

// --- Job store ---

// DailyJobCount holds the number of jobs added on a given date.
type DailyJobCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// JobReader is for the UI/API layer to query enriched jobs.
type JobReader interface {
	GetJobs(filter JobFilter, page, limit int) ([]model.Job, int64, error)
	GetJobByID(id string) (*model.Job, error)
	GetJobCounts() (*JobCounts, error)
	GetJobsAddedByDay(days int) ([]DailyJobCount, error)
}

// JobWriter is for the LLM enricher to insert completed job results.
type JobWriter interface {
	InsertJob(job *model.Job) error
	DeleteJob(id string) error
	UpdateJobStatus(id, status string) error
	UpdateJobSaved(id string, saved bool) error
	UpdateJobNotes(id, notes string) error
}

// JobStore composes both for contexts that need full access.
type JobStore interface {
	JobReader
	JobWriter
}

// --- Seen ledger ---

// SeenLedgerReader checks if a job has already been processed.
type SeenLedgerReader interface {
	HasSeen(jobID string) (bool, error)
}

// SeenLedgerWriter marks jobs as seen or updates their reason.
type SeenLedgerWriter interface {
	MarkSeen(jobID, companyID, reason string) error
	UpdateSeenReason(jobID, reason string) error
	UpdateSeenReasonBatch(jobIDs []string, reason string) error
	MarkRemovedBatch(jobIDs []string) error
}

// SeenLedgerCompanyReader provides company-scoped reads on the ledger.
type SeenLedgerCompanyReader interface {
	GetActiveJobIDsByCompany(companyID string) ([]string, error)
}

// SeenLedgerStore composes all ledger interfaces.
type SeenLedgerStore interface {
	SeenLedgerReader
	SeenLedgerWriter
	SeenLedgerCompanyReader
}

// --- Discard ---

// DiscardStore writes dismissed jobs to the discarded table.
type DiscardStore interface {
	InsertDiscard(jobResultID, reason, note string) error
}

// --- Snapshot ---

// SnapshotInput carries the data the enricher writes at pipeline time.
type SnapshotInput struct {
	ID                string
	CompanyID         string
	Country           string
	State             string
	Remote            bool
	Sponsorship       bool
	RequiresCitizen   bool
	SecurityClearance bool
	EmployType        string
	ExperienceYears   *int
	TechStack         []byte
	RoleCategory      string
}

// SnapshotWriter is used by the enricher and API to record each enriched job.
type SnapshotWriter interface {
	InsertSnapshot(input SnapshotInput) error
	UpdateSnapshotAction(id, action string) error
}

// --- GORM implementations ---

type jobStore struct {
	db *gorm.DB
}

// NewJobStore constructs a JobStore implementation.
func NewJobStore(db *gorm.DB) JobStore {
	return &jobStore{db: db}
}

func (s *jobStore) GetJobs(filter JobFilter, page, limit int) ([]model.Job, int64, error) {
	query := s.db.Model(&model.Job{})

	// Status — always applied; "saved" is a boolean flag, not a status value
	if filter.Status == model.JobStatusSaved {
		query = query.Where("saved = ?", true)
	} else if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}

	// Company — exact match on company_id, OR across values
	if len(filter.Companies) > 0 {
		query = query.Where("company_id IN ?", filter.Companies)
	}

	// Remote
	if filter.Remote != nil {
		query = query.Where("remote = ?", *filter.Remote)
	}

	// On-site (remote = false)
	if filter.OnSite {
		query = query.Where("remote = ?", false)
	}

	// Employment type — OR across values
	if len(filter.EmployTypes) > 0 {
		query = query.Where("employ_type IN ?", filter.EmployTypes)
	}

	// Sponsorship
	if filter.Sponsorship != nil {
		query = query.Where("sponsorship = ?", *filter.Sponsorship)
	}

	// No citizenship / clearance requirements
	if filter.NoRequirements {
		query = query.Where("requires_citizen = ? AND security_clearance = ?", false, false)
	}

	// Citizenship required
	if filter.RequiresCitizen != nil {
		query = query.Where("requires_citizen = ?", *filter.RequiresCitizen)
	}

	// Experience ranges — OR across selected bands
	if len(filter.ExperienceRanges) > 0 {
		var clauses []string
		for _, r := range filter.ExperienceRanges {
			switch r {
			case "0-2":
				clauses = append(clauses, "experience_years <= 2")
			case "3-5":
				clauses = append(clauses, "(experience_years >= 3 AND experience_years <= 5)")
			case "5+":
				clauses = append(clauses, "experience_years > 5")
			}
		}
		if len(clauses) > 0 {
			query = query.Where("(" + strings.Join(clauses, " OR ") + ")")
		}
	}

	// Days added — added_at >= NOW() - N days
	if filter.DaysAdded != nil {
		query = query.Where("added_at >= NOW() - ? * INTERVAL '1 day'", *filter.DaysAdded)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var jobs []model.Job
	offset := (page - 1) * limit
	err := query.Order("added_at DESC").Offset(offset).Limit(limit).Preload("Company").Find(&jobs).Error
	return jobs, total, err
}

func (s *jobStore) GetJobByID(id string) (*model.Job, error) {
	var job model.Job
	err := s.db.Preload("Company").Where("id = ?", id).First(&job).Error
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *jobStore) InsertJob(job *model.Job) error {
	return s.db.Create(job).Error
}

func (s *jobStore) DeleteJob(id string) error {
	return s.db.Where("id = ?", id).Delete(&model.Job{}).Error
}

func (s *jobStore) UpdateJobStatus(id, status string) error {
	return s.db.Model(&model.Job{}).
		Where("id = ?", id).
		Update("status", status).Error
}

func (s *jobStore) UpdateJobSaved(id string, saved bool) error {
	if saved {
		return s.db.Model(&model.Job{}).
			Where("id = ? AND status != ?", id, model.JobStatusApplied).
			Updates(map[string]any{"saved": true, "status": model.JobStatusSaved}).Error
	}
	return s.db.Exec(
		`UPDATE job_feed SET saved = false, status = CASE WHEN status = ? THEN ? ELSE status END WHERE id = ?`,
		model.JobStatusSaved, model.JobStatusNew, id,
	).Error
}

func (s *jobStore) UpdateJobNotes(id, notes string) error {
	return s.db.Model(&model.Job{}).
		Where("id = ?", id).
		Update("notes", notes).Error
}

func (s *jobStore) GetJobCounts() (*JobCounts, error) {
	var rows []CountsResult
	err := s.db.Raw(`
		SELECT status, COUNT(*) as count
		FROM job_feed
		WHERE status IN (?, ?, ?, ?, ?)
		GROUP BY status
	`,
		model.JobStatusNew,
		model.JobStatusSaved,
		model.JobStatusApplied,
		model.JobStatusInterviewing,
		model.JobStatusOffer,
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	counts := &JobCounts{}
	for _, r := range rows {
		switch r.Status {
		case model.JobStatusNew:
			counts.New = r.Count
		case model.JobStatusSaved:
			counts.Saved = r.Count
		case model.JobStatusApplied:
			counts.Applied = r.Count
		case model.JobStatusInterviewing:
			counts.Interviewing = r.Count
		case model.JobStatusOffer:
			counts.Offer = r.Count
		}
	}
	return counts, nil
}

func (s *jobStore) GetJobsAddedByDay(days int) ([]DailyJobCount, error) {
	var results []DailyJobCount
	err := s.db.Model(&model.Job{}).
		Select("DATE(added_at) as date, COUNT(*) as count").
		Where("added_at >= NOW() - ? * INTERVAL '1 day'", days).
		Group("DATE(added_at)").
		Order("date ASC").
		Scan(&results).Error
	return results, err
}

// --- Seen ledger store ---

type seenLedgerStore struct {
	db *gorm.DB
}

// NewSeenLedgerStore constructs a SeenLedgerStore implementation.
func NewSeenLedgerStore(db *gorm.DB) SeenLedgerStore {
	return &seenLedgerStore{db: db}
}

func (s *seenLedgerStore) HasSeen(jobID string) (bool, error) {
	var count int64
	err := s.db.Model(&model.JobSeenLedger{}).
		Where("job_id = ?", jobID).
		Count(&count).Error
	return count > 0, err
}

func (s *seenLedgerStore) MarkSeen(jobID, companyID, reason string) error {
	row := model.JobSeenLedger{
		JobID:     jobID,
		CompanyID: companyID,
		Reason:    reason,
	}
	return s.db.Create(&row).Error
}

func (s *seenLedgerStore) UpdateSeenReason(jobID, reason string) error {
	return s.db.Model(&model.JobSeenLedger{}).
		Where("job_id = ?", jobID).
		Update("reason", reason).Error
}

func (s *seenLedgerStore) UpdateSeenReasonBatch(jobIDs []string, reason string) error {
	if len(jobIDs) == 0 {
		return nil
	}
	return s.db.Model(&model.JobSeenLedger{}).
		Where("job_id IN ?", jobIDs).
		Update("reason", reason).Error
}

func (s *seenLedgerStore) MarkRemovedBatch(jobIDs []string) error {
	if len(jobIDs) == 0 {
		return nil
	}
	now := time.Now()
	return s.db.Model(&model.JobSeenLedger{}).
		Where("job_id IN ?", jobIDs).
		Where("removed_at IS NULL").
		Update("removed_at", now).Error
}

func (s *seenLedgerStore) GetActiveJobIDsByCompany(companyID string) ([]string, error) {
	var ids []string
	err := s.db.Model(&model.JobSeenLedger{}).
		Where("company_id = ? AND removed_at IS NULL", companyID).
		Pluck("job_id", &ids).Error
	return ids, err
}

// --- Discard store ---

type discardStore struct {
	db *gorm.DB
}

// NewDiscardStore constructs a DiscardStore implementation.
func NewDiscardStore(db *gorm.DB) DiscardStore {
	return &discardStore{db: db}
}

func (s *discardStore) InsertDiscard(jobResultID, reason, note string) error {
	var notePtr *string
	if note != "" {
		notePtr = &note
	}
	row := model.Discarded{
		JobResultID: jobResultID,
		Reason:      reason,
		Note:        notePtr,
	}
	return s.db.Create(&row).Error
}

// --- Snapshot store ---

type snapshotStore struct {
	db *gorm.DB
}

// NewSnapshotStore constructs a SnapshotWriter implementation.
func NewSnapshotStore(db *gorm.DB) SnapshotWriter {
	return &snapshotStore{db: db}
}

func (s *snapshotStore) InsertSnapshot(input SnapshotInput) error {
	row := model.JobSnapshot{
		ID:                input.ID,
		CompanyID:         input.CompanyID,
		SnapshottedAt:     time.Now(),
		Remote:            input.Remote,
		Sponsorship:       input.Sponsorship,
		RequiresCitizen:   input.RequiresCitizen,
		SecurityClearance: input.SecurityClearance,
		ExperienceYears:   input.ExperienceYears,
		TechStack:         input.TechStack,
	}
	if input.Country != "" {
		row.Country = &input.Country
	}
	if input.State != "" {
		row.State = &input.State
	}
	if input.EmployType != "" {
		row.EmployType = &input.EmployType
	}
	if input.RoleCategory != "" {
		row.RoleCategory = &input.RoleCategory
	}
	return s.db.Create(&row).Error
}

func (s *snapshotStore) UpdateSnapshotAction(id, action string) error {
	now := time.Now()
	return s.db.Model(&model.JobSnapshot{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"user_action": action,
			"actioned_at": now,
		}).Error
}
