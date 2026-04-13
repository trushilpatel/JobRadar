package store

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// ScanQueueItem is one row in company_scan_queue.
type ScanQueueItem struct {
	ID        string     `json:"id"`
	CompanyID string     `json:"company_id"`
	Status    string     `json:"status"`
	Source    string     `json:"source"`
	Priority  int        `json:"priority"`
	QueuedAt  time.Time  `json:"queued_at"`
	StartedAt *time.Time `json:"started_at"`
}

// ScanQueueStore manages the company scan work queue.
type ScanQueueStore interface {
	// Enqueue inserts a company into the queue. Silently no-ops if the company
	// already has an active entry (queued or processing).
	Enqueue(companyID, source string) error

	// ClaimNext atomically claims the next queued item and marks it processing.
	// Returns nil, nil if the queue is empty.
	// Ordered by priority DESC, queued_at ASC.
	ClaimNext(ctx context.Context) (*ScanQueueItem, error)

	// Complete removes the queue entry. Call after scan succeeds or fails.
	Complete(id string) error

	// Remove deletes a queued entry by ID. No-ops if entry is processing or missing.
	Remove(id string) error

	// MoveUp raises this item one position in the queue by increasing its priority.
	// No-op if the item is already first or does not exist.
	MoveUp(id string) error

	// MoveDown lowers this item one position in the queue by increasing the
	// priority of the item currently below it.
	// No-op if the item is already last or does not exist.
	MoveDown(id string) error

	// ResetStuck resets processing rows that started more than staleDur ago back
	// to queued status. Call once on crawler startup to recover from crashes.
	ResetStuck(staleDur time.Duration) error

	// ListQueue returns all active queue entries ordered by priority DESC, queued_at ASC.
	ListQueue() ([]ScanQueueItem, error)
}

type scanQueueStore struct {
	db *gorm.DB
}

func NewScanQueueStore(db *gorm.DB) ScanQueueStore {
	return &scanQueueStore{db: db}
}

func (s *scanQueueStore) Enqueue(companyID, source string) error {
	return s.db.Exec(`
		INSERT INTO company_scan_queue (company_id, source)
		VALUES (?, ?)
		ON CONFLICT DO NOTHING
	`, companyID, source).Error
}

func (s *scanQueueStore) ClaimNext(ctx context.Context) (*ScanQueueItem, error) {
	var item ScanQueueItem
	err := s.db.WithContext(ctx).Raw(`
		UPDATE company_scan_queue
		SET status = 'processing', started_at = NOW()
		WHERE id = (
			SELECT id FROM company_scan_queue
			WHERE status = 'queued'
			ORDER BY priority DESC, queued_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, company_id, status, source, priority, queued_at, started_at
	`).Scan(&item).Error
	if err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, nil
	}
	return &item, nil
}

func (s *scanQueueStore) Complete(id string) error {
	return s.db.Exec(`DELETE FROM company_scan_queue WHERE id = ?`, id).Error
}

func (s *scanQueueStore) Remove(id string) error {
	return s.db.Exec(`DELETE FROM company_scan_queue WHERE id = ? AND status = 'queued'`, id).Error
}

func (s *scanQueueStore) MoveUp(id string) error {
	items, err := s.listOrdered()
	if err != nil {
		return err
	}
	pos := -1
	for i, item := range items {
		if item.ID == id {
			pos = i
			break
		}
	}
	if pos <= 0 {
		return nil // already first or not found
	}
	above := items[pos-1]
	newPriority := above.Priority + 1
	return s.db.Exec(`UPDATE company_scan_queue SET priority = ? WHERE id = ?`, newPriority, id).Error
}

func (s *scanQueueStore) MoveDown(id string) error {
	items, err := s.listOrdered()
	if err != nil {
		return err
	}
	pos := -1
	for i, item := range items {
		if item.ID == id {
			pos = i
			break
		}
	}
	if pos < 0 || pos >= len(items)-1 {
		return nil // already last or not found
	}
	this := items[pos]
	below := items[pos+1]
	newPriority := this.Priority + 1
	return s.db.Exec(`UPDATE company_scan_queue SET priority = ? WHERE id = ?`, newPriority, below.ID).Error
}

func (s *scanQueueStore) ResetStuck(staleDur time.Duration) error {
	return s.db.Exec(`
		UPDATE company_scan_queue
		SET status = 'queued', started_at = NULL
		WHERE status = 'processing'
		  AND started_at < NOW() - INTERVAL '1 second' * ?
	`, int(staleDur.Seconds())).Error
}

func (s *scanQueueStore) ListQueue() ([]ScanQueueItem, error) {
	return s.listOrdered()
}

// listOrdered is the shared implementation used by ListQueue, MoveUp, and MoveDown.
func (s *scanQueueStore) listOrdered() ([]ScanQueueItem, error) {
	var items []ScanQueueItem
	err := s.db.Raw(`
		SELECT id, company_id, status, source, priority, queued_at, started_at
		FROM company_scan_queue
		ORDER BY priority DESC, queued_at ASC
	`).Scan(&items).Error
	return items, err
}
