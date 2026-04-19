package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/domain"
	appcommon "github.com/trushilpatel/jobradar/service/application/common"
	"github.com/trushilpatel/jobradar/service/application/dto"
	appmodel "github.com/trushilpatel/jobradar/service/application/models"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

type implService struct {
	BaseService
	jobSvc jobsvc.Service
}

// NewApplicationImplService constructs the business-logic layer of the application chain.
func NewApplicationImplService(next ApplicationService, jobSvc jobsvc.Service) ApplicationService {
	return &implService{
		BaseService: NewBaseService(next),
		jobSvc:      jobSvc,
	}
}

// Create verifies the job exists, inserts the application row, then marks the job applied when appropriate.
// All other methods auto-delegate to next via BaseService.
func (s *implService) Create(ctx context.Context, jobID string, input dto.CreateApplicationInput) (*appmodel.JobApplication, error) {
	if _, err := s.jobSvc.Get(ctx, jobID); err != nil {
		return nil, err
	}
	app, err := s.next.Create(ctx, jobID, input)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to create application")
	}
	if input.Stage == appcommon.StageApplied {
		_ = s.jobSvc.MarkApplied(ctx, jobID)
	}
	return app, nil
}
