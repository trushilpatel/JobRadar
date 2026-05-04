package services

import (
	"context"
	"math"

	"github.com/trushilpatel/jobradar/service/insights/common"
	"github.com/trushilpatel/jobradar/service/insights/dto"
)

type implService struct {
	BaseService
}

// NewImplService constructs an InsightsService that delegates to next, overriding Velocity
// to enrich rows with a 4-week multiplier and velocity tag.
func NewImplService(next InsightsService) InsightsService {
	return &implService{BaseService: NewBaseService(next)}
}

// Velocity enriches repository rows with multiplier and velocity_tag before returning.
func (s *implService) Velocity(ctx context.Context) (_ any, err error) {
	raw, err := s.next.Velocity(ctx)
	if err != nil {
		return nil, err
	}
	rows, ok := raw.([]dto.CompanyVelocity)
	if !ok {
		return raw, nil
	}
	for i := range rows {
		rows[i].Multiplier, rows[i].VelocityTag = classifyVelocity(
			rows[i].WeekCurrent, rows[i].Week1, rows[i].Week2, rows[i].Week3,
		)
	}
	return rows, nil
}

func classifyVelocity(current, w1, w2, w3 int) (float64, string) {
	avgPrior := float64(w1+w2+w3) / 3.0
	var multiplier float64
	if avgPrior == 0 {
		multiplier = 1.0
	} else {
		multiplier = float64(current) / avgPrior
	}
	multiplier = math.Round(multiplier*10) / 10
	switch {
	case multiplier >= 2.5:
		return multiplier, common.VelocitySpike
	case multiplier >= 1.3:
		return multiplier, common.VelocityRising
	default:
		return multiplier, common.VelocityNormal
	}
}
