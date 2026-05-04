package services

import (
	"context"

	"github.com/trushilpatel/jobradar/service/insights/dto"
)

// InsightsService defines all analytics queries for the insights domain.
type InsightsService interface {
	TechStack(ctx context.Context) (any, error)
	CompanyActivity(ctx context.Context) (any, error)
	Funnel(ctx context.Context) (any, error)
	Location(ctx context.Context) (any, error)
	Sponsorship(ctx context.Context, filter dto.InsightsFilter) (any, error)
	Velocity(ctx context.Context) (any, error)
	RoleCategories(ctx context.Context) (any, error)
	GetPipelineCounts(ctx context.Context) (dto.PipelineCounts, error)
}

// BaseService auto-delegates all InsightsService methods to the next layer.
// Embed this in Impl and Validation services to avoid re-implementing unchanged methods.
type BaseService struct {
	next InsightsService
}

// NewBaseService constructs a BaseService. Panics if next is nil.
func NewBaseService(next InsightsService) BaseService {
	if next == nil {
		panic("BaseService: next must not be nil — check wire setup")
	}
	return BaseService{next: next}
}

func (b *BaseService) TechStack(ctx context.Context) (any, error) {
	return b.next.TechStack(ctx)
}

func (b *BaseService) CompanyActivity(ctx context.Context) (any, error) {
	return b.next.CompanyActivity(ctx)
}

func (b *BaseService) Funnel(ctx context.Context) (any, error) {
	return b.next.Funnel(ctx)
}

func (b *BaseService) Location(ctx context.Context) (any, error) {
	return b.next.Location(ctx)
}

func (b *BaseService) Sponsorship(ctx context.Context, filter dto.InsightsFilter) (any, error) {
	return b.next.Sponsorship(ctx, filter)
}

func (b *BaseService) Velocity(ctx context.Context) (any, error) {
	return b.next.Velocity(ctx)
}

func (b *BaseService) RoleCategories(ctx context.Context) (any, error) {
	return b.next.RoleCategories(ctx)
}

func (b *BaseService) GetPipelineCounts(ctx context.Context) (dto.PipelineCounts, error) {
	return b.next.GetPipelineCounts(ctx)
}
