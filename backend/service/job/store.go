package job

import (
	"context"
	"strings"
	"time"

	"github.com/trushilpatel/jobradar/infra/tracing"
	"github.com/trushilpatel/jobradar/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// --- Job store ---

// DailyJobCount holds the number of jobs added on a given date.
type DailyJobCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// JobReader is for the UI/API layer to query enriched jobs.
type JobReader interface {
	GetJobs(ctx context.Context, filter JobFilter, page, limit int) ([]model.Job, int64, error)
	GetJobByID(ctx context.Context, id string) (*model.Job, error)
	GetJobCounts(ctx context.Context) (*JobCounts, error)
	GetJobsAddedByDay(ctx context.Context, days int) ([]DailyJobCount, error)
}

// JobWriter is for the LLM enricher to insert completed job results.
type JobWriter interface {
	InsertJob(ctx context.Context, job *model.Job) error
	DeleteJob(ctx context.Context, id string) error
	UpdateJobStatus(ctx context.Context, id, status string) error
	UpdateJobSaved(ctx context.Context, id string, saved bool) error
	UpdateJobNotes(ctx context.Context, id, notes string) error
}

// JobStore composes both for contexts that need full access.
type JobStore interface {
	JobReader
	JobWriter
}

// --- Seen ledger ---

// SeenLedgerReader checks if a job has already been processed.
type SeenLedgerReader interface {
	HasSeen(ctx context.Context, jobID string) (bool, error)
}

// SeenLedgerWriter marks jobs as seen or updates their reason.
type SeenLedgerWriter interface {
	MarkSeen(ctx context.Context, jobID, companyID, reason string) error
	UpdateSeenReason(ctx context.Context, jobID, reason string) error
	UpdateSeenReasonBatch(ctx context.Context, jobIDs []string, reason string) error
	MarkRemovedBatch(ctx context.Context, jobIDs []string) error
}

// SeenLedgerCompanyReader provides company-scoped reads on the ledger.
type SeenLedgerCompanyReader interface {
	GetActiveJobIDsByCompany(ctx context.Context, companyID string) ([]string, error)
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
	InsertDiscard(ctx context.Context, jobResultID, reason, note string) error
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
	InsertSnapshot(ctx context.Context, input SnapshotInput) error
	UpdateSnapshotAction(ctx context.Context, id, action string) error
}

// ApplicationWriter is a narrow write interface used by the job service to
// auto-insert application tracker rows when a job is advanced to applied/interviewing.
type ApplicationWriter interface {
	InsertApplication(jobResultID, stage, note string) error
}

// --- GORM implementations ---

type jobStore struct {
	db *gorm.DB
}

// NewJobStore constructs a JobStore implementation.
func NewJobStore(db *gorm.DB) JobStore {
	return &jobStore{db: db}
}

func (s *jobStore) GetJobs(ctx context.Context, filter JobFilter, page, limit int) (_ []model.Job, _ int64, err error) {
	ctx, span := tracing.Start(ctx, "job.store.GetJobs")
	defer tracing.End(span, &err)
	query := s.db.WithContext(ctx).Model(&model.Job{})

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
		var args []interface{}
		for _, r := range filter.ExperienceRanges {
			if r.Max != nil {
				if r.Min == 0 {
					// lowest band: include NULL (unknown experience level)
					clauses = append(clauses, "(experience_years <= ? OR experience_years IS NULL)")
					args = append(args, *r.Max)
				} else {
					clauses = append(clauses, "(experience_years > ? AND experience_years <= ?)")
					args = append(args, r.Min, *r.Max)
				}
			} else {
				clauses = append(clauses, "experience_years > ?")
				args = append(args, r.Min)
			}
		}
		if len(clauses) > 0 {
			query = query.Where("("+strings.Join(clauses, " OR ")+")", args...)
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
	err = query.Order("added_at DESC").Offset(offset).Limit(limit).Preload("Company").Find(&jobs).Error
	return jobs, total, err
}

func (s *jobStore) GetJobByID(ctx context.Context, id string) (_ *model.Job, err error) {
	ctx, span := tracing.Start(ctx, "job.store.GetJobByID")
	defer tracing.End(span, &err)
	var job model.Job
	err = s.db.WithContext(ctx).Preload("Company").Where("id = ?", id).First(&job).Error
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *jobStore) InsertJob(ctx context.Context, job *model.Job) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.InsertJob")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Create(job).Error
}

func (s *jobStore) DeleteJob(ctx context.Context, id string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.DeleteJob")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Where("id = ?", id).Delete(&model.Job{}).Error
}

func (s *jobStore) UpdateJobStatus(ctx context.Context, id, status string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.UpdateJobStatus")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Model(&model.Job{}).
		Where("id = ?", id).
		Update("status", status).Error
}

func (s *jobStore) UpdateJobSaved(ctx context.Context, id string, saved bool) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.UpdateJobSaved")
	defer tracing.End(span, &err)
	if saved {
		return s.db.WithContext(ctx).Model(&model.Job{}).
			Where("id = ? AND status != ?", id, model.JobStatusApplied).
			Updates(map[string]any{"saved": true, "status": model.JobStatusSaved}).Error
	}
	return s.db.WithContext(ctx).Exec(
		`UPDATE job_feed SET saved = false, status = CASE WHEN status = ? THEN ? ELSE status END WHERE id = ?`,
		model.JobStatusSaved, model.JobStatusNew, id,
	).Error
}

func (s *jobStore) UpdateJobNotes(ctx context.Context, id, notes string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.UpdateJobNotes")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Model(&model.Job{}).
		Where("id = ?", id).
		Update("notes", notes).Error
}

func (s *jobStore) GetJobCounts(ctx context.Context) (_ *JobCounts, err error) {
	ctx, span := tracing.Start(ctx, "job.store.GetJobCounts")
	defer tracing.End(span, &err)
	var rows []CountsResult
	err = s.db.WithContext(ctx).Raw(`
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

func (s *jobStore) GetJobsAddedByDay(ctx context.Context, days int) (results []DailyJobCount, err error) {
	ctx, span := tracing.Start(ctx, "job.store.GetJobsAddedByDay")
	defer tracing.End(span, &err)
	err = s.db.WithContext(ctx).Model(&model.Job{}).
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

func (s *seenLedgerStore) HasSeen(ctx context.Context, jobID string) (_ bool, err error) {
	ctx, span := tracing.Start(ctx, "job.store.HasSeen")
	defer tracing.End(span, &err)
	var count int64
	err = s.db.WithContext(ctx).Model(&model.JobSeenLedger{}).
		Where("job_id = ?", jobID).
		Count(&count).Error
	return count > 0, err
}

func (s *seenLedgerStore) MarkSeen(ctx context.Context, jobID, companyID, reason string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.MarkSeen")
	defer tracing.End(span, &err)
	row := model.JobSeenLedger{
		JobID:     jobID,
		CompanyID: companyID,
		Reason:    reason,
	}
	return s.db.WithContext(ctx).Create(&row).Error
}

func (s *seenLedgerStore) UpdateSeenReason(ctx context.Context, jobID, reason string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.UpdateSeenReason")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Model(&model.JobSeenLedger{}).
		Where("job_id = ?", jobID).
		Update("reason", reason).Error
}

func (s *seenLedgerStore) UpdateSeenReasonBatch(ctx context.Context, jobIDs []string, reason string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.UpdateSeenReasonBatch")
	defer tracing.End(span, &err)
	if len(jobIDs) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Model(&model.JobSeenLedger{}).
		Where("job_id IN ?", jobIDs).
		Update("reason", reason).Error
}

func (s *seenLedgerStore) MarkRemovedBatch(ctx context.Context, jobIDs []string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.MarkRemovedBatch")
	defer tracing.End(span, &err)
	if len(jobIDs) == 0 {
		return nil
	}
	now := time.Now()
	return s.db.WithContext(ctx).Model(&model.JobSeenLedger{}).
		Where("job_id IN ?", jobIDs).
		Where("removed_at IS NULL").
		Update("removed_at", now).Error
}

func (s *seenLedgerStore) GetActiveJobIDsByCompany(ctx context.Context, companyID string) (ids []string, err error) {
	ctx, span := tracing.Start(ctx, "job.store.GetActiveJobIDsByCompany")
	defer tracing.End(span, &err)
	err = s.db.WithContext(ctx).Model(&model.JobSeenLedger{}).
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

func (s *discardStore) InsertDiscard(ctx context.Context, jobResultID, reason, note string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.InsertDiscard")
	defer tracing.End(span, &err)
	var notePtr *string
	if note != "" {
		notePtr = &note
	}
	row := model.Discarded{
		JobResultID: jobResultID,
		Reason:      reason,
		Note:        notePtr,
	}
	return s.db.WithContext(ctx).Create(&row).Error
}

// --- Tech tag store ---

// TechTagWriter writes normalized technology tags for a job into job_tech_tags.
type TechTagWriter interface {
	InsertTechTags(ctx context.Context, jobID, companyID string, techs []string, taggedAt time.Time) error
}

type techTagStore struct {
	db *gorm.DB
}

// NewTechTagStore constructs a TechTagWriter implementation.
func NewTechTagStore(db *gorm.DB) TechTagWriter {
	return &techTagStore{db: db}
}

func (s *techTagStore) InsertTechTags(ctx context.Context, jobID, companyID string, techs []string, taggedAt time.Time) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.InsertTechTags")
	defer tracing.End(span, &err)
	if len(techs) == 0 {
		return nil
	}
	rows := make([]model.JobTechTag, 0, len(techs))
	for _, tech := range techs {
		rows = append(rows, model.JobTechTag{
			JobID:     jobID,
			Tech:      tech,
			CompanyID: companyID,
			TaggedAt:  taggedAt,
		})
	}
	return s.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&rows).Error
}

// --- Snapshot store ---

type snapshotStore struct {
	db *gorm.DB
}

// NewSnapshotStore constructs a SnapshotWriter implementation.
func NewSnapshotStore(db *gorm.DB) SnapshotWriter {
	return &snapshotStore{db: db}
}

func (s *snapshotStore) InsertSnapshot(ctx context.Context, input SnapshotInput) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.InsertSnapshot")
	defer tracing.End(span, &err)
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
	return s.db.WithContext(ctx).Create(&row).Error
}

func (s *snapshotStore) UpdateSnapshotAction(ctx context.Context, id, action string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.UpdateSnapshotAction")
	defer tracing.End(span, &err)
	now := time.Now()
	return s.db.WithContext(ctx).Model(&model.JobSnapshot{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"user_action": action,
			"actioned_at": now,
		}).Error
}
