package services

import (
	"context"

	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	"github.com/trushilpatel/jobradar/service/scan/dto"
)

// RestService handles Huma operations for the scan domain.
// It holds a single next Service dependency; routing belongs in RestRouter.
type RestService struct {
	next Service
}

// NewRestService constructs a RestService.
func NewRestService(next Service) *RestService {
	return &RestService{next: next}
}

// listQueueInput is the Huma input for GET /api/companies/queue.
type listQueueInput struct {
	Status string `query:"status"`
	Source string `query:"source"`
}

// ListQueue serves GET /api/companies/queue.
func (h *RestService) ListQueue(ctx context.Context, in *listQueueInput) (*apihuma.SingleOutput[[]dto.QueueEntryView], error) {
	filter := dto.QueueFilter{Status: in.Status, Source: in.Source}
	views, err := h.next.ListQueue(ctx, filter)
	if err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[[]dto.QueueEntryView]{}
	out.Body.Data = views
	return out, nil
}

// removeFromQueueInput is the Huma input for DELETE /api/companies/queue/{entry_id}.
type removeFromQueueInput struct {
	EntryID string `path:"entry_id"`
}

// RemoveFromQueue serves DELETE /api/companies/queue/{entry_id}.
func (h *RestService) RemoveFromQueue(ctx context.Context, in *removeFromQueueInput) (*apihuma.SingleOutput[map[string]string], error) {
	if err := h.next.RemoveFromQueue(ctx, in.EntryID); err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[map[string]string]{}
	out.Body.Data = map[string]string{"id": in.EntryID}
	return out, nil
}

// TriggerScanAll serves POST /api/companies/scan-all.
func (h *RestService) TriggerScanAll(ctx context.Context, _ *struct{}) (*apihuma.SingleOutput[map[string]any], error) {
	count, err := h.next.TriggerScanAll(ctx)
	if err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[map[string]any]{}
	out.Body.Data = map[string]any{"queued": count}
	return out, nil
}

// scanStatsInput is the Huma input for GET /api/companies/{id}/scan-stats.
type scanStatsInput struct {
	ID string `path:"id"`
}

// ScanStats serves GET /api/companies/{id}/scan-stats.
func (h *RestService) ScanStats(ctx context.Context, in *scanStatsInput) (*apihuma.SingleOutput[*dto.ScanStatsView], error) {
	stats, err := h.next.GetStats(ctx, in.ID)
	if err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[*dto.ScanStatsView]{}
	out.Body.Data = stats
	return out, nil
}

// triggerScanInput is the Huma input for POST /api/companies/{id}/scan.
type triggerScanInput struct {
	ID string `path:"id"`
}

// TriggerScan serves POST /api/companies/{id}/scan.
func (h *RestService) TriggerScan(ctx context.Context, in *triggerScanInput) (*apihuma.SingleOutput[map[string]string], error) {
	if err := h.next.TriggerScan(ctx, in.ID); err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[map[string]string]{}
	out.Body.Data = map[string]string{"company_id": in.ID, "status": "scan_queued"}
	return out, nil
}

// analyticsInput is the Huma input for GET /api/companies/{id}/analytics.
type analyticsInput struct {
	ID string `path:"id"`
}

// Analytics serves GET /api/companies/{id}/analytics.
func (h *RestService) Analytics(ctx context.Context, in *analyticsInput) (*apihuma.SingleOutput[*dto.CompanyAnalytics], error) {
	a, err := h.next.GetAnalytics(ctx, in.ID)
	if err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[*dto.CompanyAnalytics]{}
	out.Body.Data = a
	return out, nil
}

// scanConfigInput is the Huma input for GET /api/companies/{id}/scan-config.
type scanConfigInput struct {
	ID string `path:"id"`
}

// GetEffectiveScanConfig serves GET /api/companies/{id}/scan-config.
func (h *RestService) GetEffectiveScanConfig(ctx context.Context, in *scanConfigInput) (*apihuma.SingleOutput[dto.EffectiveScanConfigDTO], error) {
	config, err := h.next.GetEffectiveScanConfig(ctx, in.ID)
	if err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[dto.EffectiveScanConfigDTO]{}
	out.Body.Data = config
	return out, nil
}

// updateScanIntervalOverrideInput is the Huma input for
// PATCH /api/companies/{id}/scan-interval-override.
type updateScanIntervalOverrideInput struct {
	ID   string `path:"id"`
	Body struct {
		// required:"false" — matches the pre-Huma handler's plain
		// json.Decode; a nil (or absent) override clears it.
		ScanIntervalMinutesOverride *int `json:"scan_interval_minutes_override" required:"false"`
	}
}

// UpdateScanIntervalOverride serves PATCH /api/companies/{id}/scan-interval-override.
func (h *RestService) UpdateScanIntervalOverride(ctx context.Context, in *updateScanIntervalOverrideInput) (*apihuma.SingleOutput[map[string]string], error) {
	if err := h.next.UpdateScanIntervalOverride(ctx, in.ID, in.Body.ScanIntervalMinutesOverride); err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[map[string]string]{}
	out.Body.Data = map[string]string{"company_id": in.ID, "status": "updated"}
	return out, nil
}
