// Package services holds the decorator chain for the role domain.
//
// Phase 1 keeps the chain shallow — Validation → Impl → Repository — because
// no admin-facing UI ships in this phase. Future phases extend the chain
// (cache, audit, etc.) without touching this contract.
package services

import (
	"context"

	rolemodel "github.com/trushilpatel/jobradar/service/role/models"
)

// Service is the contract every role-domain layer honours. The methods are
// scoped narrowly: the auth middleware needs ListForUser + Grant on first
// login, the future admin UI will need the rest.
//
// Grant takes an optional grantedBy so audit captures who promoted a user;
// nil means the system performed the grant (e.g. ADMIN_EMAILS auto-promote).
type Service interface {
	ListForUser(ctx context.Context, userID string) ([]string, error)
	Grant(ctx context.Context, userID, roleID string, grantedBy *string) error
	Revoke(ctx context.Context, userID, roleID string) error
	ListAll(ctx context.Context) ([]rolemodel.Role, error)
}

// BaseService auto-delegates every Service method to next. Layers embed it
// and override only the methods they have logic for; the rest fall through.
type BaseService struct {
	next Service
}

// NewBaseService wraps next. Panics if next is nil — a misconfigured chain
// is a programmer error, never a runtime condition we want to handle.
func NewBaseService(next Service) BaseService {
	if next == nil {
		panic("role.BaseService: next must not be nil — check wire setup")
	}
	return BaseService{next: next}
}

func (b *BaseService) ListForUser(ctx context.Context, userID string) ([]string, error) {
	return b.next.ListForUser(ctx, userID)
}

func (b *BaseService) Grant(ctx context.Context, userID, roleID string, grantedBy *string) error {
	return b.next.Grant(ctx, userID, roleID, grantedBy)
}

func (b *BaseService) Revoke(ctx context.Context, userID, roleID string) error {
	return b.next.Revoke(ctx, userID, roleID)
}

func (b *BaseService) ListAll(ctx context.Context) ([]rolemodel.Role, error) {
	return b.next.ListAll(ctx)
}
