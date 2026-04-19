package services

import (
	"context"

	"github.com/trushilpatel/jobradar/service/insights/dto"
)

// Service defines all analytics queries for the insights domain.
type Service interface {
	TechStack(ctx context.Context) (interface{}, error)
	CompanyActivity(ctx context.Context) (interface{}, error)
	Funnel(ctx context.Context) (interface{}, error)
	Location(ctx context.Context) (interface{}, error)
	Sponsorship(ctx context.Context, filter dto.InsightsFilter) (interface{}, error)
	Velocity(ctx context.Context) (interface{}, error)
	RoleCategories(ctx context.Context) (interface{}, error)
	GetPipelineCounts(ctx context.Context) (dto.PipelineCounts, error)
}

// Store provides aggregated analytics over the job pipeline.
type Store interface {
	GetTechStackTrends(ctx context.Context, limit int) ([]dto.TechCount, error)
	GetCompanyActivity(ctx context.Context) ([]dto.CompanyActivityItem, error)
	GetPipelineFunnel(ctx context.Context) ([]dto.FunnelStep, error)
	GetLocationBreakdown(ctx context.Context) ([]dto.LocationCount, error)
	GetUSAStateBreakdown(ctx context.Context) ([]dto.StateCount, error)
	GetSponsorshipTrend(ctx context.Context, days int) ([]dto.SponsorshipDay, error)
	GetCompanyVelocity(ctx context.Context, limit int) ([]dto.CompanyVelocity, error)
	GetRoleCategoryBreakdown(ctx context.Context) ([]dto.RoleCategoryCount, error)
	GetPipelineCounts(ctx context.Context) (dto.PipelineCounts, error)
}

// Validator validates user-submitted input for the insights domain.
type Validator interface {
	ValidateFilter(filter dto.InsightsFilter) error
}
