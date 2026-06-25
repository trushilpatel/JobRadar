// Package services holds the decorator chain for the user domain.
//
// The chain composes RestService → ValidationService → CacheService →
// ImplService → RepositoryService. Each layer overrides only the methods it
// owns; everything else auto-delegates via BaseService so adding a method to
// the interface costs one line per layer that genuinely cares about it.
package services

import (
	"context"

	userdto "github.com/trushilpatel/jobradar/service/user/dto"
	usermodel "github.com/trushilpatel/jobradar/service/user/models"
)

// Service is the contract every user-domain layer honours.
//
// Provision is idempotent: callers (the auth middleware on first login) hand
// it the JWT subject + email and receive a row whether or not one already
// exists. Delete is hard delete — the impl layer is responsible for calling
// the Clerk admin API before the DB delete so the two stay in sync.
type Service interface {
	Get(ctx context.Context, id string) (*usermodel.User, error)
	GetByEmail(ctx context.Context, email string) (*usermodel.User, error)
	Provision(ctx context.Context, id, email string) (*usermodel.User, error)
	UpdateProfile(ctx context.Context, id string, displayName string) error
	Delete(ctx context.Context, id string) error
	GetConfig(ctx context.Context, userID string) (*userdto.UserConfig, error)
	PatchConfig(ctx context.Context, userID string, partial userdto.UserConfig) (*userdto.UserConfig, error)
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
		panic("user.BaseService: next must not be nil — check wire setup")
	}
	return BaseService{next: next}
}

func (b *BaseService) Get(ctx context.Context, id string) (*usermodel.User, error) {
	return b.next.Get(ctx, id)
}

func (b *BaseService) GetByEmail(ctx context.Context, email string) (*usermodel.User, error) {
	return b.next.GetByEmail(ctx, email)
}

func (b *BaseService) Provision(ctx context.Context, id, email string) (*usermodel.User, error) {
	return b.next.Provision(ctx, id, email)
}

func (b *BaseService) UpdateProfile(ctx context.Context, id string, displayName string) error {
	return b.next.UpdateProfile(ctx, id, displayName)
}

func (b *BaseService) Delete(ctx context.Context, id string) error {
	return b.next.Delete(ctx, id)
}

func (b *BaseService) GetConfig(ctx context.Context, userID string) (*userdto.UserConfig, error) {
	return b.next.GetConfig(ctx, userID)
}

func (b *BaseService) PatchConfig(ctx context.Context, userID string, partial userdto.UserConfig) (*userdto.UserConfig, error) {
	return b.next.PatchConfig(ctx, userID, partial)
}
