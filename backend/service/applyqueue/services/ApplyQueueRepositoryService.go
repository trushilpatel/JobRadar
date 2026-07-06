package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	applyqueuecommon "github.com/trushilpatel/jobradar/service/applyqueue/common"
	"github.com/trushilpatel/jobradar/service/applyqueue/dto"
	applyqueuemodel "github.com/trushilpatel/jobradar/service/applyqueue/models"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	_ Service                = (*repositoryService)(nil)
	_ EngagementChecker      = (*repositoryService)(nil)
	_ ResumeOwnershipChecker = (*repositoryService)(nil)
)

// repositoryService is the GORM-backed terminal repo for the apply-queue
// domain. It is stateless (holds only *gorm.DB) so the same struct backs
// three separate constructors (NewRepositoryService, NewEngagementChecker,
// NewResumeOwnershipChecker), each returning a different narrow interface —
// see ApplyQueueService.go for why.
type repositoryService struct {
	db *gorm.DB
}

// NewRepositoryService constructs the Service implementation (List + SetResume).
func NewRepositoryService(db *gorm.DB) Service {
	return &repositoryService{db: db}
}

// NewEngagementChecker constructs the EngagementChecker implementation used
// by ValidationService.
func NewEngagementChecker(db *gorm.DB) EngagementChecker {
	return &repositoryService{db: db}
}

// NewResumeOwnershipChecker constructs the ResumeOwnershipChecker
// implementation used by ValidationService.
func NewResumeOwnershipChecker(db *gorm.DB) ResumeOwnershipChecker {
	return &repositoryService{db: db}
}

// cursorPayload is the sort-aware keyset cursor state serialised between
// pages, mirroring service/job/services/JobRepositoryService.go's
// cursorPayload/encodeCursor/decodeCursor scheme but scoped to this domain's
// fixed sort key: user_job_state.updated_at DESC, job_id DESC.
//
// updated_at (not added_at/saved_at) is the sort key because
// user_job_state already carries a generic autoUpdateTime column (see
// jobmodel.UserJobState) that reflects the most recent engagement action
// (save, status change, resume attach) — the most useful "recently active"
// ordering for a queue the user is actively working through.
type cursorPayload struct {
	UpdatedAt int64  `json:"u"` // user_job_state.updated_at, UnixNano
	JobID     string `json:"j"` // tiebreaker
}

// encodeCursor packs the (updated_at, job_id) tiebreakers into an opaque
// base64-encoded JSON cursor.
func encodeCursor(updatedAt time.Time, jobID string) string {
	p := cursorPayload{UpdatedAt: updatedAt.UnixNano(), JobID: jobID}
	b, _ := json.Marshal(p)
	return base64.RawURLEncoding.EncodeToString(b)
}

// decodeCursor unpacks an opaque cursor. An empty, undecodable, or corrupt
// cursor returns nil (first page) rather than an error — a stale client-side
// cursor gracefully restarts from page 1 instead of surfacing a 400,
// matching job's decodeCursor precedent.
func decodeCursor(cursor string) *cursorPayload {
	if cursor == "" {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil
	}
	var p cursorPayload
	if err := json.Unmarshal(raw, &p); err != nil || p.JobID == "" {
		return nil
	}
	return &p
}

// applyCursorPredicate adds the keyset WHERE predicate bounding the next
// page. p == nil means first page (no predicate applied).
func applyCursorPredicate(query *gorm.DB, p *cursorPayload) *gorm.DB {
	if p == nil {
		return query
	}
	updatedAt := time.Unix(0, p.UpdatedAt)
	return query.Where("(ujs.updated_at, ujs.job_id) < (?, ?)", updatedAt, p.JobID)
}

// queueRow is the raw scan target for the List join query — a shape private
// to this file, distinct from dto.QueueItem, so the derived-status
// computation (deriveStatus) stays out of the SQL layer.
type queueRow struct {
	JobID       string         `gorm:"column:job_id"`
	Title       string         `gorm:"column:title"`
	Company     string         `gorm:"column:company"`
	URL         string         `gorm:"column:url"`
	Status      *string        `gorm:"column:status"`
	UpdatedAt   time.Time      `gorm:"column:updated_at"`
	ResumeID    *string        `gorm:"column:resume_id"`
	ResumeLabel *string        `gorm:"column:resume_label"`
	Location    *string        `gorm:"column:location"`
	Remote      bool           `gorm:"column:remote"`
	SalaryRange datatypes.JSON `gorm:"column:salary_range"`
	AddedAt     time.Time      `gorm:"column:added_at"`
}

// deriveStatus renders the client-facing status for a queue row: "saved"
// when the workflow status column is NULL (only the orthogonal saved_at
// flag is set), otherwise the actual status value verbatim. Terminal
// statuses (dismissed/offer/rejected) never reach here — both filter
// branches in List exclude them — so the only values this ever returns are
// jobcommon.JobStatusSaved, JobStatusApplied, or JobStatusInterviewing,
// matching the frontend's ApplyQueueStatusSchema three-value union
// (frontend/src/features/apply-queue/api/apply-queue.api.ts).
func deriveStatus(status *string) string {
	if status == nil {
		return jobcommon.JobStatusSaved
	}
	return *status
}

// terminalStatusExclusion is appended to every List query regardless of
// filter: a job the user has dismissed, or that reached a terminal
// offer/rejected outcome, has no business in an "apply queue" even if
// saved_at is still set (saved_at survives a dismiss — see
// UserJobStateRepository.Upsert's StatusDismissed case, which preserves it).
// Keeping this filter-independent also keeps deriveStatus's output
// restricted to {saved, applied, interviewing} in every response, matching
// the frontend contract exactly.
const terminalStatusExclusion = "ujs.status IS NULL OR ujs.status NOT IN ?"

// List returns up to limit+1 rows from user_job_state, joined to jobs +
// companies for display fields and LEFT JOINed to resume_job_attachments +
// resumes for any attached resume. The +1 probe row drives next-cursor
// generation, mirroring JobRepositoryService.GetJobs.
//
// filter == FilterSaved: saved_at IS NOT NULL (the orthogonal bookmark flag)
// plus the terminal-status exclusion below.
//
// filter == FilterAll (default / anything else — ValidationService
// whitelists before this is reached): saved_at IS NOT NULL OR status IN
// (applied, interviewing), plus the same terminal-status exclusion.
func (r *repositoryService) List(ctx context.Context, userID, filter, cursor string, limit int) (_ dto.QueueListResult, err error) {
	ctx, span := tracing.Start(ctx, "applyqueue.repo.List")
	defer tracing.End(span, &err)

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	query := r.db.WithContext(ctx).
		Table("user_job_state AS ujs").
		Joins("INNER JOIN jobs ON jobs.id = ujs.job_id").
		Joins("INNER JOIN companies ON companies.id = jobs.company_id").
		Joins("LEFT JOIN resume_job_attachments rja ON rja.user_id = ujs.user_id AND rja.job_id = ujs.job_id").
		Joins("LEFT JOIN resumes ON resumes.id = rja.resume_id").
		Where("ujs.user_id = ?", userID).
		Where(terminalStatusExclusion, []string{jobcommon.JobStatusDismissed, jobcommon.JobStatusOffer, jobcommon.JobStatusRejected})

	switch filter {
	case applyqueuecommon.FilterSaved:
		query = query.Where("ujs.saved_at IS NOT NULL")
	default: // FilterAll
		query = query.Where(
			"ujs.saved_at IS NOT NULL OR ujs.status IN ?",
			[]string{jobcommon.JobStatusApplied, jobcommon.JobStatusInterviewing},
		)
	}

	query = applyCursorPredicate(query, decodeCursor(cursor))

	var rows []queueRow
	if err = query.
		Select(`ujs.job_id AS job_id, jobs.title AS title, companies.name AS company, jobs.url AS url,
			ujs.status AS status, ujs.updated_at AS updated_at,
			resumes.id AS resume_id, resumes.label AS resume_label,
			jobs.location AS location, jobs.remote AS remote,
			jobs.salary_range AS salary_range, jobs.added_at AS added_at`).
		Order("ujs.updated_at DESC, ujs.job_id DESC").
		Limit(limit + 1).
		Find(&rows).Error; err != nil {
		return dto.QueueListResult{}, domainerrs.ErrInternal.WithMessage("applyqueue repo: list: " + err.Error())
	}

	var next string
	if len(rows) > limit {
		last := rows[limit-1]
		next = encodeCursor(last.UpdatedAt, last.JobID)
		rows = rows[:limit]
	}

	items := make([]dto.QueueItem, len(rows))
	for i, row := range rows {
		items[i] = dto.QueueItem{
			JobID:       row.JobID,
			Title:       row.Title,
			Company:     row.Company,
			URL:         row.URL,
			Status:      deriveStatus(row.Status),
			ResumeID:    row.ResumeID,
			ResumeLabel: row.ResumeLabel,
			Location:    row.Location,
			Remote:      row.Remote,
			SalaryRange: row.SalaryRange,
			AddedAt:     row.AddedAt,
		}
	}

	if len(items) > 0 {
		if kwErr := r.attachJobKeywords(ctx, items); kwErr != nil {
			return dto.QueueListResult{}, domainerrs.ErrInternal.WithMessage("applyqueue repo: job keywords: " + kwErr.Error())
		}
	}

	return dto.QueueListResult{Items: items, NextCursor: next}, nil
}

// jobTechTagRow is the raw scan target for a job_tech_tags lookup — one row
// per (job, tech) pair, matching the table's composite primary key.
type jobTechTagRow struct {
	JobID string `gorm:"column:job_id"`
	Tech  string `gorm:"column:tech"`
}

// attachJobKeywords fetches the canonical (techtag.Clean-cleaned at insert
// time — see JobPipelineImplService.go) job_tech_tags for every job on the
// page in a single query and attaches them to each item in place — one query
// per page, never one per row. Deliberately reads job_tech_tags rather than
// the jobs.tech_stack JSONB column, which is uncleaned raw LLM output per
// that table's own doc comment. Plain data, no scoring: matching against a
// resume happens client-side.
func (r *repositoryService) attachJobKeywords(ctx context.Context, items []dto.QueueItem) (err error) {
	ctx, span := tracing.Start(ctx, "applyqueue.repo.attachJobKeywords")
	defer tracing.End(span, &err)

	jobIDs := make([]string, len(items))
	for i, it := range items {
		jobIDs[i] = it.JobID
	}

	var rows []jobTechTagRow
	if dbErr := r.db.WithContext(ctx).Table("job_tech_tags").
		Select("job_id, tech").
		Where("job_id IN ?", jobIDs).
		Find(&rows).Error; dbErr != nil {
		return dbErr
	}

	byJob := make(map[string][]string, len(jobIDs))
	for _, row := range rows {
		byJob[row.JobID] = append(byJob[row.JobID], row.Tech)
	}
	for i := range items {
		items[i].JobKeywords = byJob[items[i].JobID]
	}
	return nil
}

// SetResume attaches or clears the resume_job_attachments row for
// (userID, jobID). resumeID == nil deletes the row (idempotent — no error if
// absent) and returns (nil, nil). resumeID != nil upserts the row via
// ON CONFLICT (user_id, job_id) DO UPDATE — a second attach overwrites the
// first rather than erroring — then reads back the resume's label to build
// the response view.
func (r *repositoryService) SetResume(ctx context.Context, userID, jobID string, resumeID *string) (_ *dto.AttachmentView, err error) {
	ctx, span := tracing.Start(ctx, "applyqueue.repo.SetResume")
	defer tracing.End(span, &err)

	if resumeID == nil {
		if delErr := r.db.WithContext(ctx).
			Where("user_id = ? AND job_id = ?", userID, jobID).
			Delete(&applyqueuemodel.ResumeJobAttachment{}).Error; delErr != nil {
			return nil, domainerrs.ErrInternal.WithMessage("applyqueue repo: clear resume: " + delErr.Error())
		}
		return nil, nil
	}

	row := applyqueuemodel.ResumeJobAttachment{UserID: userID, JobID: jobID, ResumeID: *resumeID}
	if createErr := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "job_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"resume_id", "updated_at"}),
	}).Create(&row).Error; createErr != nil {
		return nil, domainerrs.ErrInternal.WithMessage("applyqueue repo: upsert resume: " + createErr.Error())
	}

	var label string
	if labelErr := r.db.WithContext(ctx).Table("resumes").
		Select("label").Where("id = ?", *resumeID).Scan(&label).Error; labelErr != nil {
		return nil, domainerrs.ErrInternal.WithMessage("applyqueue repo: fetch resume label: " + labelErr.Error())
	}

	return &dto.AttachmentView{JobID: jobID, ResumeID: resumeID, ResumeLabel: &label}, nil
}

// HasEngagement reports whether a user_job_state row exists for
// (userID, jobID) — used by ValidationService to reject attaching a resume
// to a job the user has never engaged with.
func (r *repositoryService) HasEngagement(ctx context.Context, userID, jobID string) (_ bool, err error) {
	ctx, span := tracing.Start(ctx, "applyqueue.repo.HasEngagement")
	defer tracing.End(span, &err)

	var count int64
	if err = r.db.WithContext(ctx).Table("user_job_state").
		Where("user_id = ? AND job_id = ?", userID, jobID).
		Count(&count).Error; err != nil {
		return false, domainerrs.ErrInternal.WithMessage("applyqueue repo: has engagement: " + err.Error())
	}
	return count > 0, nil
}

// ResumeOwnedByUser reports whether resumeID belongs to userID — used by
// ValidationService to reject attaching another user's resume.
func (r *repositoryService) ResumeOwnedByUser(ctx context.Context, userID, resumeID string) (_ bool, err error) {
	ctx, span := tracing.Start(ctx, "applyqueue.repo.ResumeOwnedByUser")
	defer tracing.End(span, &err)

	var count int64
	if err = r.db.WithContext(ctx).Table("resumes").
		Where("id = ? AND user_id = ?", resumeID, userID).
		Count(&count).Error; err != nil {
		return false, domainerrs.ErrInternal.WithMessage("applyqueue repo: resume owned by user: " + err.Error())
	}
	return count > 0, nil
}
