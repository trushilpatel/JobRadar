// Package services holds the decorator chain for the ats domain.
//
// The chain composes RestRouter → RestService → ValidationService →
// CacheService → ImplService → RepositoryService. Each layer overrides only
// the methods it owns; everything else auto-delegates via BaseAtsService so
// adding a method to the interface costs one line per layer that cares.
package services

import (
	"context"

	atsdto "github.com/trushilpatel/jobradar/service/ats/dto"
)

// AtsService is the contract every ats-domain layer honours.
//
// ListATS returns all registered ATS rows in insertion order.
// GetATS returns a single row by primary key name.
// UpdateATS applies the supplied optional-field patch to the named row.
type AtsService interface {
	ListATS(ctx context.Context) ([]atsdto.ATSDTO, error)
	GetATS(ctx context.Context, name string) (atsdto.ATSDTO, error)
	UpdateATS(ctx context.Context, name string, input atsdto.UpdateATSInput) (atsdto.ATSDTO, error)
}

// BaseAtsService auto-delegates every AtsService method to next. Layers embed
// it and override only the methods they have logic for; the rest fall through.
type BaseAtsService struct {
	next AtsService
}

// NewBaseAtsService wraps next. Panics if next is nil — a misconfigured chain
// is a programmer error, never a runtime condition we want to handle.
func NewBaseAtsService(next AtsService) BaseAtsService {
	if next == nil {
		panic("ats.BaseAtsService: next must not be nil — check wire setup")
	}
	return BaseAtsService{next: next}
}

func (b *BaseAtsService) ListATS(ctx context.Context) ([]atsdto.ATSDTO, error) {
	return b.next.ListATS(ctx)
}

func (b *BaseAtsService) GetATS(ctx context.Context, name string) (atsdto.ATSDTO, error) {
	return b.next.GetATS(ctx, name)
}

func (b *BaseAtsService) UpdateATS(ctx context.Context, name string, input atsdto.UpdateATSInput) (atsdto.ATSDTO, error) {
	return b.next.UpdateATS(ctx, name, input)
}
