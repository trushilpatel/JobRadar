package scan

import (
	"time"

	"github.com/trushilpatel/jobradar/api/response"
	"github.com/trushilpatel/jobradar/model"
	"github.com/trushilpatel/jobradar/service/company"
)

// Service defines all business operations for the scan domain.
type Service interface {
	TriggerScan(companyID string) error
	GetStats(companyID string) (*ScanStatsView, error)
	ListQueue(filter QueueFilter) ([]QueueEntryView, error)
	RemoveFromQueue(entryID string) error
	MoveUp(entryID string) error
	MoveDown(entryID string) error
}

type service struct {
	queue      ScanQueueStore
	scanLog    ScanLogReader
	companySvc company.Service
	config     Config
}

// NewService constructs a Service implementation.
func NewService(queue ScanQueueStore, scanLog ScanLogReader, companySvc company.Service, cfg Config) Service {
	return &service{
		queue:      queue,
		scanLog:    scanLog,
		companySvc: companySvc,
		config:     cfg,
	}
}

// TriggerScan enqueues a manual scan for a company. Verifies the company exists first.
func (s *service) TriggerScan(companyID string) error {
	if _, err := s.companySvc.Get(companyID); err != nil {
		return err // already a ServiceError from company.Service
	}
	if err := s.queue.Enqueue(companyID, model.QueueSourceManual); err != nil {
		return response.ErrInternal.WithMessage("failed to trigger scan")
	}
	return nil
}

// GetStats returns a summary of scan history for the given company.
func (s *service) GetStats(companyID string) (*ScanStatsView, error) {
	stats, err := s.scanLog.GetScanStats(companyID)
	if err != nil {
		return nil, response.ErrInternal.WithMessage("failed to fetch scan stats")
	}
	return &stats, nil
}

// ListQueue returns all active queue entries enriched with company name, adapter, and next_scan_at.
func (s *service) ListQueue(filter QueueFilter) ([]QueueEntryView, error) {
	items, err := s.queue.ListQueue()
	if err != nil {
		return nil, response.ErrInternal.WithMessage("failed to fetch queue")
	}

	allCompanies, err := s.companySvc.GetAll()
	if err != nil {
		return nil, response.ErrInternal.WithMessage("failed to fetch companies")
	}

	companyMap := make(map[string]company.CompanyView, len(allCompanies))
	for _, c := range allCompanies {
		companyMap[c.ID] = c
	}

	views := make([]QueueEntryView, 0, len(items))
	for _, item := range items {
		// Apply status filter if set
		if filter.Status != "" && item.Status != filter.Status {
			continue
		}
		// Apply source filter if set
		if filter.Source != "" && item.Source != filter.Source {
			continue
		}

		v := QueueEntryView{
			ID:        item.ID,
			CompanyID: item.CompanyID,
			Status:    item.Status,
			Source:    item.Source,
			Priority:  item.Priority,
			QueuedAt:  item.QueuedAt,
			StartedAt: item.StartedAt,
		}
		if c, ok := companyMap[item.CompanyID]; ok {
			v.CompanyName = c.Name
			v.AdapterType = c.AdapterType
			if c.LastScanAt != nil {
				t := c.LastScanAt.Add(time.Duration(s.config.ScanIntervalMinutes) * time.Minute)
				v.NextScanAt = &t
			}
		}
		views = append(views, v)
	}

	return views, nil
}

// RemoveFromQueue removes a queued entry by ID.
func (s *service) RemoveFromQueue(entryID string) error {
	if err := s.queue.Remove(entryID); err != nil {
		return response.ErrInternal.WithMessage("failed to remove queue entry")
	}
	return nil
}

// MoveUp raises a queue entry one position.
func (s *service) MoveUp(entryID string) error {
	if err := s.queue.MoveUp(entryID); err != nil {
		return response.ErrInternal.WithMessage("failed to move queue entry")
	}
	return nil
}

// MoveDown lowers a queue entry one position.
func (s *service) MoveDown(entryID string) error {
	if err := s.queue.MoveDown(entryID); err != nil {
		return response.ErrInternal.WithMessage("failed to move queue entry")
	}
	return nil
}
