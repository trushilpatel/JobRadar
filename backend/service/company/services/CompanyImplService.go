package services

import (
	"context"
	"time"

	"github.com/trushilpatel/jobradar/common/domain"
	"github.com/trushilpatel/jobradar/infra/tracing"
	"github.com/trushilpatel/jobradar/service/company/common"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
)

type implService struct {
	BaseService
	config      common.Config
	scanHistory companydto.ScanHistoryReader
}

// NewService constructs the business-logic decorator.
// It overrides List and GetAll to enrich views with NextScanAt and ScanHistory.
// All other methods auto-delegate through BaseService to next.
func NewService(next Service, cfg common.Config, scanHistory companydto.ScanHistoryReader) Service {
	return &implService{
		BaseService: NewBaseService(next),
		config:      cfg,
		scanHistory: scanHistory,
	}
}

// List returns a filtered, paginated page of companies enriched with next_scan_at
// and scan_history (last ≤10 scan outcomes from company_scan_log).
func (s *implService) List(ctx context.Context, filter companydto.CompanyFilter, page, limit int) (_ companydto.CompanyListResult, err error) {
	ctx, span := tracing.Start(ctx, "company.impl.List")
	defer tracing.End(span, &err)

	result, err := s.next.List(ctx, filter, page, limit)
	if err != nil {
		return companydto.CompanyListResult{}, err
	}

	// Collect IDs for the batch scan-history lookup.
	ids := make([]string, len(result.Items))
	for i, v := range result.Items {
		ids[i] = v.ID
	}
	histories, err := s.scanHistory.GetScanHistories(ctx, ids)
	if err != nil {
		// Non-fatal: degrade gracefully — scan history is cosmetic.
		histories = map[string][]bool{}
	}

	enriched := make([]companydto.CompanyView, 0, len(result.Items))
	for _, v := range result.Items {
		if v.LastScanAt != nil {
			t := v.LastScanAt.Add(time.Duration(s.config.ScanIntervalMinutes) * time.Minute)
			v.NextScanAt = &t
		}
		v.ScanHistory = histories[v.ID]
		enriched = append(enriched, v)
	}
	result.Items = enriched
	return result, nil
}

// GetAll returns all companies as views enriched with NextScanAt (for internal/worker use).
func (s *implService) GetAll(ctx context.Context) (_ []companydto.CompanyView, err error) {
	ctx, span := tracing.Start(ctx, "company.impl.GetAll")
	defer tracing.End(span, &err)

	views, err := s.next.GetAll(ctx)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch companies")
	}

	enriched := make([]companydto.CompanyView, 0, len(views))
	for _, v := range views {
		if v.LastScanAt != nil {
			t := v.LastScanAt.Add(time.Duration(s.config.ScanIntervalMinutes) * time.Minute)
			v.NextScanAt = &t
		}
		enriched = append(enriched, v)
	}
	return enriched, nil
}
