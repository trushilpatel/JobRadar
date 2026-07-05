package services

import (
	"context"
	"encoding/json"

	"github.com/trushilpatel/jobradar/common/tracing"
)

// jobTechTagRow is the raw scan target for a job_tech_tags lookup — one row
// per (job, tech) pair, matching the table's composite primary key.
type jobTechTagRow struct {
	JobID string `gorm:"column:job_id"`
	Tech  string `gorm:"column:tech"`
}

// resumeKeywordRow is the raw scan target for a per-user resumes lookup.
// Keywords is scanned as raw bytes (JSONB on Postgres, TEXT in the sqlite
// unit-test schema) and parsed separately by parseResumeKeywordRows — kept
// as two steps so a single malformed row never fails the whole batch.
type resumeKeywordRow struct {
	ID       string `gorm:"column:id"`
	Label    string `gorm:"column:label"`
	Keywords []byte `gorm:"column:keywords"`
}

// parsedResumeKeywords is a resumeKeywordRow with Keywords already decoded
// and filtered to non-empty — the shape bestMatchingResume actually compares
// against, so JSON parsing happens once per resume (not once per job).
type parsedResumeKeywords struct {
	ID       string
	Label    string
	Keywords []string
}

// loadJobTechTags fetches the canonical (techtag.Clean-cleaned at insert
// time — see JobPipelineImplService.go) tech tags for every job in jobIDs in
// a single query, returned as job_id -> []tech. Deliberately reads
// job_tech_tags rather than the jobs.tech_stack JSONB column, which is
// uncleaned raw LLM output per that table's own doc comment.
func (r *repositoryService) loadJobTechTags(ctx context.Context, jobIDs []string) (_ map[string][]string, err error) {
	ctx, span := tracing.Start(ctx, "applyqueue.repo.loadJobTechTags")
	defer tracing.End(span, &err)

	var rows []jobTechTagRow
	if dbErr := r.db.WithContext(ctx).Table("job_tech_tags").
		Select("job_id, tech").
		Where("job_id IN ?", jobIDs).
		Find(&rows).Error; dbErr != nil {
		return nil, dbErr
	}

	out := make(map[string][]string, len(jobIDs))
	for _, row := range rows {
		out[row.JobID] = append(out[row.JobID], row.Tech)
	}
	return out, nil
}

// loadUserResumeKeywords fetches every resume owned by userID (id, label,
// keywords), ordered oldest-first so bestMatchingResume's tie-break
// (first-encountered wins) deterministically prefers the resume the user
// uploaded earliest. Bounded by resumecommon.ResumeLimitPerUser (50) per
// user, so this is always a small, cheap query — one per List call, not per
// row (the join-once-per-page requirement).
func (r *repositoryService) loadUserResumeKeywords(ctx context.Context, userID string) (_ []resumeKeywordRow, err error) {
	ctx, span := tracing.Start(ctx, "applyqueue.repo.loadUserResumeKeywords")
	defer tracing.End(span, &err)

	var rows []resumeKeywordRow
	if dbErr := r.db.WithContext(ctx).Table("resumes").
		Select("id, label, keywords").
		Where("user_id = ?", userID).
		Order("created_at ASC, id ASC").
		Find(&rows).Error; dbErr != nil {
		return nil, dbErr
	}
	return rows, nil
}

// parseResumeKeywordRows decodes each row's Keywords JSON, dropping any
// resume with empty or malformed keywords — such a resume is simply never a
// recommendation candidate (extraction never ran, failed, or hasn't been
// retried yet), not an error condition for the whole page.
func parseResumeKeywordRows(rows []resumeKeywordRow) []parsedResumeKeywords {
	out := make([]parsedResumeKeywords, 0, len(rows))
	for _, row := range rows {
		var keywords []string
		if err := json.Unmarshal(row.Keywords, &keywords); err != nil || len(keywords) == 0 {
			continue
		}
		out = append(out, parsedResumeKeywords{ID: row.ID, Label: row.Label, Keywords: keywords})
	}
	return out
}

// jaccardSimilarity returns |A∩B| / |A∪B| for two tech-tag sets. Both inputs
// are expected to already be canonically normalized (techtag.Clean on the
// job side at insert time, techtag.Clean on the resume side at extraction
// time) so exact string equality is a valid set-membership test — no fuzzy
// matching or case-folding needed here.
func jaccardSimilarity(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	setA := make(map[string]struct{}, len(a))
	for _, v := range a {
		setA[v] = struct{}{}
	}
	setB := make(map[string]struct{}, len(b))
	for _, v := range b {
		setB[v] = struct{}{}
	}

	intersection := 0
	for v := range setA {
		if _, ok := setB[v]; ok {
			intersection++
		}
	}
	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// bestMatchingResume picks the highest-Jaccard-scoring resume for jobTags
// among resumes, returning (nil, nil) when jobTags is empty, resumes is
// empty, or every candidate scores exactly 0 — a job with no tech tags, or a
// user with no keyword-bearing resumes, gets no recommendation rather than
// an arbitrary one. Ties keep the first-encountered (oldest-uploaded, per
// loadUserResumeKeywords's ORDER BY) resume, so the result is deterministic
// across calls for identical input.
func bestMatchingResume(jobTags []string, resumes []parsedResumeKeywords) (*string, *string) {
	if len(jobTags) == 0 || len(resumes) == 0 {
		return nil, nil
	}

	var bestScore float64
	var bestID, bestLabel string
	found := false
	for _, r := range resumes {
		score := jaccardSimilarity(jobTags, r.Keywords)
		if score > bestScore {
			bestScore = score
			bestID = r.ID
			bestLabel = r.Label
			found = true
		}
	}
	if !found {
		return nil, nil
	}
	id, label := bestID, bestLabel
	return &id, &label
}
