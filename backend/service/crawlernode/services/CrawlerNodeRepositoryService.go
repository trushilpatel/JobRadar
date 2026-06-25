package services

import (
	"context"
	stderrors "errors"
	"strings"
	"time"

	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	crawlernodemodel "github.com/trushilpatel/jobradar/service/crawlernode/models"
	"gorm.io/gorm"
)

var _ NodeStore = (*nodeRepositoryService)(nil)

// nodeRepositoryService implements NodeStore against the crawler_nodes table.
// Terminal — no next, no BaseNodeService.
type nodeRepositoryService struct {
	db *gorm.DB
}

// NewNodeRepositoryService constructs a NodeStore backed by Postgres.
func NewNodeRepositoryService(db *gorm.DB) NodeStore {
	return &nodeRepositoryService{db: db}
}

func (s *nodeRepositoryService) Create(ctx context.Context, node crawlernodemodel.CrawlerNode) (_ *crawlernodemodel.CrawlerNode, err error) {
	ctx, span := tracing.Start(ctx, "crawlernode.store.Create")
	defer tracing.End(span, &err)

	if result := s.db.WithContext(ctx).Create(&node); result.Error != nil {
		if isUniqueViolation(result.Error) {
			return nil, errors.ErrConflict.WithMessage("crawler node name already exists: " + node.Name)
		}
		return nil, errors.ErrInternal.WithMessage("create crawler node: " + result.Error.Error())
	}
	return &node, nil
}

func (s *nodeRepositoryService) GetByTokenPrefix(ctx context.Context, prefix string) (_ []*crawlernodemodel.CrawlerNode, err error) {
	ctx, span := tracing.Start(ctx, "crawlernode.store.GetByTokenPrefix")
	defer tracing.End(span, &err)

	var nodes []*crawlernodemodel.CrawlerNode
	if result := s.db.WithContext(ctx).Where("token_prefix = ?", prefix).Find(&nodes); result.Error != nil {
		return nil, errors.ErrInternal.WithMessage("get by token prefix: " + result.Error.Error())
	}
	return nodes, nil
}

func (s *nodeRepositoryService) UpdateLastSeen(ctx context.Context, nodeID string) (err error) {
	ctx, span := tracing.Start(ctx, "crawlernode.store.UpdateLastSeen")
	defer tracing.End(span, &err)

	result := s.db.WithContext(ctx).
		Model(&crawlernodemodel.CrawlerNode{}).
		Where("id = ?", nodeID).
		Update("last_seen_at", time.Now().UTC())
	if result.Error != nil {
		return errors.ErrInternal.WithMessage("update last_seen_at: " + result.Error.Error())
	}
	if result.RowsAffected == 0 {
		return errors.ErrNotFound.WithMessage("UpdateLastSeen: no node row for id=" + nodeID)
	}
	return nil
}

func (s *nodeRepositoryService) SetStatus(ctx context.Context, nodeID, status string) (err error) {
	ctx, span := tracing.Start(ctx, "crawlernode.store.SetStatus")
	defer tracing.End(span, &err)

	// Guard: retired is terminal — WHERE excludes retired rows; 0 rows → ErrNotFound.
	result := s.db.WithContext(ctx).
		Model(&crawlernodemodel.CrawlerNode{}).
		Where("id = ? AND status != 'retired'", nodeID).
		Update("status", status)
	if result.Error != nil {
		return errors.ErrInternal.WithMessage("set node status: " + result.Error.Error())
	}
	if result.RowsAffected == 0 {
		return errors.ErrNotFound.WithMessage("SetStatus: no node row for id=" + nodeID)
	}
	return nil
}

func (s *nodeRepositoryService) List(ctx context.Context, status string) (_ []crawlernodemodel.CrawlerNode, err error) {
	ctx, span := tracing.Start(ctx, "crawlernode.store.List")
	defer tracing.End(span, &err)

	q := s.db.WithContext(ctx).Model(&crawlernodemodel.CrawlerNode{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var nodes []crawlernodemodel.CrawlerNode
	if result := q.Order("created_at DESC").Find(&nodes); result.Error != nil {
		return nil, errors.ErrInternal.WithMessage("list crawler nodes: " + result.Error.Error())
	}
	return nodes, nil
}

// Retire sets status='retired' and clears token_hash atomically in one UPDATE.
// Returns ErrNotFound when no row matched (already retired or unknown ID).
func (s *nodeRepositoryService) Retire(ctx context.Context, nodeID string) (err error) {
	ctx, span := tracing.Start(ctx, "crawlernode.store.Retire")
	defer tracing.End(span, &err)

	result := s.db.WithContext(ctx).
		Model(&crawlernodemodel.CrawlerNode{}).
		Where("id = ? AND status != 'retired'", nodeID).
		Updates(map[string]any{
			"status":     "retired",
			"token_hash": nil,
		})
	if result.Error != nil {
		return errors.ErrInternal.WithMessage("retire node: " + result.Error.Error())
	}
	if result.RowsAffected == 0 {
		return errors.ErrNotFound.WithMessage("Retire: no active/disabled node row for id=" + nodeID)
	}
	return nil
}

// isUniqueViolation detects UNIQUE constraint violations across Postgres (SQLSTATE 23505)
// and SQLite (error string contains "UNIQUE constraint failed").
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	type pgErr interface {
		SQLState() string
	}
	var pg pgErr
	if stderrors.As(err, &pg) {
		return pg.SQLState() == "23505"
	}
	// SQLite fallback used in unit tests.
	return strings.Contains(err.Error(), "UNIQUE constraint failed:")
}
