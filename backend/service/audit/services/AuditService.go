// Package services holds the decorator chain for the audit domain.
//
// Phase 1 keeps the chain shallow — Impl → Repository — because no audit UI
// ships. Future phases (admin viewer, retention policy, export) extend the
// chain without changing this contract.
package services

import (
	"context"

	auditmodel "github.com/trushilpatel/jobradar/service/audit/models"
)

// ListFilter is the filter shape consumed by Service.List. All fields are
// optional: zero values mean "no filter".
type ListFilter struct {
	ActorUserID *string
	Action      *string
	Limit       int
	Offset      int
}

// Service is the contract every audit-domain layer honours.
type Service interface {
	Append(ctx context.Context, entry auditmodel.Entry) error
	List(ctx context.Context, filter ListFilter) (entries []auditmodel.Entry, total int64, err error)
}

// BaseService auto-delegates every Service method to next.
type BaseService struct {
	next Service
}

// NewBaseService wraps next.
func NewBaseService(next Service) BaseService {
	if next == nil {
		panic("audit.BaseService: next must not be nil — check wire setup")
	}
	return BaseService{next: next}
}

func (b *BaseService) Append(ctx context.Context, entry auditmodel.Entry) error {
	return b.next.Append(ctx, entry)
}

func (b *BaseService) List(ctx context.Context, filter ListFilter) ([]auditmodel.Entry, int64, error) {
	return b.next.List(ctx, filter)
}
