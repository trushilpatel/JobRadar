package services

import (
	"context"
	"math"

	"github.com/trushilpatel/jobradar/common/tracing"
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
func (s *implService) Velocity(ctx context.Context) (_ []dto.CompanyVelocity, err error) {
	ctx, span := tracing.Start(ctx, "insights.impl.Velocity")
	defer tracing.End(span, &err)
	rows, err := s.next.Velocity(ctx)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].Multiplier, rows[i].VelocityTag = classifyVelocity(
			rows[i].WeekCurrent, rows[i].Week1, rows[i].Week2, rows[i].Week3,
		)
	}
	return rows, nil
}

// classifyVelocity computes a multiplier and velocity tag from 4-week counts.
// When the prior three weeks are all zero and the current week has postings,
// we classify as spike rather than normal — a multiplier of 1.0 (old fallback)
// is always below the 1.3 rising threshold, causing new companies to show "—".
func classifyVelocity(current, w1, w2, w3 int) (float64, string) {
	avgPrior := float64(w1+w2+w3) / 3.0
	var multiplier float64
	switch {
	case avgPrior == 0 && current > 0:
		// No baseline yet but jobs posted this week — classify as spike.
		multiplier = 3.0
	case avgPrior == 0:
		// No baseline and no current week postings — truly inactive.
		multiplier = 0
	default:
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
