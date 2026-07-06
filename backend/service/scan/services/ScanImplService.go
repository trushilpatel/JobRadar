package services

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
	"github.com/trushilpatel/jobradar/service/scan/dto"
)

// Config holds per-service configuration for the scan domain.
type Config struct {
	ScanIntervalMinutes int
}

// implService is the business-logic decorator. It embeds BaseService so all
// methods not overridden here are auto-delegated to the next layer.
//
// Phase 2 wiring: scan-side reads of company adapter/scan state route through
// `companysvc.ScanSettingsRepository` (the single point of access to the
// `company_scan_settings` table). The admin chain is consulted only for the
// human-readable company name used to enrich queue listings.
type implService struct {
	BaseService
	scanSettings companysvc.ScanSettingsRepository
	companyAdmin companysvc.CompanyAdminService
	config       Config
}

// NewService constructs the ImplService decorator.
func NewService(next Service, scanSettings companysvc.ScanSettingsRepository, companyAdmin companysvc.CompanyAdminService, cfg Config) Service {
	return &implService{
		BaseService:  NewBaseService(next),
		scanSettings: scanSettings,
		companyAdmin: companyAdmin,
		config:       cfg,
	}
}

// TriggerScan verifies the company has scan_settings before enqueuing.
// Companies without a `company_scan_settings` row are post-cutover
// un-onboarded — we surface this as BAD_REQUEST so admins can see the
// exact remediation rather than enqueuing a row the crawler will skip.
func (s *implService) TriggerScan(ctx context.Context, companyID string) (err error) {
	ctx, span := tracing.Start(ctx, "scan.impl.TriggerScan")
	defer tracing.End(span, &err)
	if _, err := s.scanSettings.Get(ctx, companyID); err != nil {
		var sErr *apperr.ServiceError
		if stderrors.As(err, &sErr) && sErr.Code == "NOT_FOUND" {
			return apperr.ErrBadRequest.WithMessage("company has no scan settings configured")
		}
		return err // already a ServiceError
	}
	return s.next.TriggerScan(ctx, companyID)
}

// ListQueue fetches raw entries from next, then enriches them with company
// metadata. Company name comes from the admin view; adapter_type and
// last_scan_at come from the scan_settings join carried on that view.
func (s *implService) ListQueue(ctx context.Context, filter dto.QueueFilter) (_ []dto.QueueEntryView, err error) {
	ctx, span := tracing.Start(ctx, "scan.impl.ListQueue")
	defer tracing.End(span, &err)

	views, err := s.next.ListQueue(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("scan.impl.ListQueue: %w", err)
	}

	seen := make(map[string]struct{}, len(views))
	ids := make([]string, 0, len(views))
	for _, v := range views {
		if _, ok := seen[v.CompanyID]; !ok {
			seen[v.CompanyID] = struct{}{}
			ids = append(ids, v.CompanyID)
		}
	}
	if len(ids) == 0 {
		return views, nil
	}

	companies, err := s.companyAdmin.GetByIDs(ctx, ids)
	if err != nil {
		return nil, apperr.ErrInternal.WithMessage("failed to fetch companies")
	}

	companyByID := make(map[string]int, len(companies))
	for i := range companies {
		companyByID[companies[i].ID] = i
	}

	for i, v := range views {
		idx, ok := companyByID[v.CompanyID]
		if !ok {
			continue
		}
		c := companies[idx]
		views[i].CompanyName = c.Name
		if c.ScanSettings != nil {
			views[i].AdapterType = c.ScanSettings.AdapterType
			if c.ScanSettings.LastScanAt != nil {
				t := c.ScanSettings.LastScanAt.Add(time.Duration(s.config.ScanIntervalMinutes) * time.Minute)
				views[i].NextScanAt = &t
			}
		}
	}

	return views, nil
}
