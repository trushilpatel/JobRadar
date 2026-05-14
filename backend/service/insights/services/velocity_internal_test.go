package services

// Internal tests for the unexported classifyVelocity helper.
//
// classifyVelocity owns the only branching logic on the Impl layer — it
// translates raw 4-week post counts into a (multiplier, velocity tag) pair
// that the UI uses to render "spike / rising / normal" pills. The branch
// table is small but every boundary is observable to the user, so each
// boundary is exercised explicitly.

import (
	"math"
	"testing"

	"github.com/trushilpatel/jobradar/service/insights/common"
)

func TestClassifyVelocity_AllZero(t *testing.T) {
	mult, tag := classifyVelocity(0, 0, 0, 0)
	if mult != 0 {
		t.Errorf("multiplier = %v, want 0", mult)
	}
	if tag != common.VelocityNormal {
		t.Errorf("tag = %q, want %q", tag, common.VelocityNormal)
	}
}

func TestClassifyVelocity_ZeroBaselineWithCurrent(t *testing.T) {
	// No baseline yet but jobs posted this week — classified as spike (3.0).
	mult, tag := classifyVelocity(5, 0, 0, 0)
	if mult != 3.0 {
		t.Errorf("multiplier = %v, want 3.0", mult)
	}
	if tag != common.VelocitySpike {
		t.Errorf("tag = %q, want %q", tag, common.VelocitySpike)
	}
}

func TestClassifyVelocity_Spike_AtBoundary(t *testing.T) {
	// avgPrior = 2, current = 5 -> 2.5 exactly -> spike (>= 2.5).
	mult, tag := classifyVelocity(5, 2, 2, 2)
	if mult != 2.5 {
		t.Errorf("multiplier = %v, want 2.5", mult)
	}
	if tag != common.VelocitySpike {
		t.Errorf("tag = %q, want %q", tag, common.VelocitySpike)
	}
}

func TestClassifyVelocity_Spike_AboveBoundary(t *testing.T) {
	// avgPrior = 1, current = 4 -> 4.0 -> spike.
	mult, tag := classifyVelocity(4, 1, 1, 1)
	if mult != 4.0 {
		t.Errorf("multiplier = %v, want 4.0", mult)
	}
	if tag != common.VelocitySpike {
		t.Errorf("tag = %q, want %q", tag, common.VelocitySpike)
	}
}

func TestClassifyVelocity_Rising_AtBoundary(t *testing.T) {
	// avgPrior = 10, current = 13 -> 1.3 exactly -> rising (>= 1.3, < 2.5).
	mult, tag := classifyVelocity(13, 10, 10, 10)
	if mult != 1.3 {
		t.Errorf("multiplier = %v, want 1.3", mult)
	}
	if tag != common.VelocityRising {
		t.Errorf("tag = %q, want %q", tag, common.VelocityRising)
	}
}

func TestClassifyVelocity_Rising_AboveBoundary(t *testing.T) {
	// avgPrior = 4, current = 8 -> 2.0 -> rising.
	mult, tag := classifyVelocity(8, 4, 4, 4)
	if mult != 2.0 {
		t.Errorf("multiplier = %v, want 2.0", mult)
	}
	if tag != common.VelocityRising {
		t.Errorf("tag = %q, want %q", tag, common.VelocityRising)
	}
}

func TestClassifyVelocity_Normal_JustBelowRising(t *testing.T) {
	// avgPrior = 10, current = 12 -> 1.2 -> normal (< 1.3).
	mult, tag := classifyVelocity(12, 10, 10, 10)
	if mult != 1.2 {
		t.Errorf("multiplier = %v, want 1.2", mult)
	}
	if tag != common.VelocityNormal {
		t.Errorf("tag = %q, want %q", tag, common.VelocityNormal)
	}
}

func TestClassifyVelocity_Normal_BelowOne(t *testing.T) {
	// avgPrior = 10, current = 5 -> 0.5 -> normal.
	mult, tag := classifyVelocity(5, 10, 10, 10)
	if mult != 0.5 {
		t.Errorf("multiplier = %v, want 0.5", mult)
	}
	if tag != common.VelocityNormal {
		t.Errorf("tag = %q, want %q", tag, common.VelocityNormal)
	}
}

func TestClassifyVelocity_Rounding_HalfUp(t *testing.T) {
	// avgPrior = (1+1+2)/3 = 4/3 ≈ 1.333..., current = 1 -> 0.75 -> rounds to 0.8.
	mult, tag := classifyVelocity(1, 1, 1, 2)
	want := math.Round((1.0/(4.0/3.0))*10) / 10
	if mult != want {
		t.Errorf("multiplier = %v, want %v", mult, want)
	}
	// 0.8 < 1.3 -> normal
	if tag != common.VelocityNormal {
		t.Errorf("tag = %q, want %q", tag, common.VelocityNormal)
	}
}

func TestClassifyVelocity_Rounding_BoundaryFromAbove(t *testing.T) {
	// avgPrior = 3, current = 8 -> 2.666... -> rounds to 2.7 -> spike (>= 2.5).
	mult, tag := classifyVelocity(8, 3, 3, 3)
	if mult != 2.7 {
		t.Errorf("multiplier = %v, want 2.7", mult)
	}
	if tag != common.VelocitySpike {
		t.Errorf("tag = %q, want %q", tag, common.VelocitySpike)
	}
}

func TestClassifyVelocity_MixedPriorWeeks(t *testing.T) {
	// avgPrior = (10+0+0)/3 ≈ 3.33, current = 5 -> 5/3.33 ≈ 1.5 -> rising.
	mult, tag := classifyVelocity(5, 10, 0, 0)
	if mult != 1.5 {
		t.Errorf("multiplier = %v, want 1.5", mult)
	}
	if tag != common.VelocityRising {
		t.Errorf("tag = %q, want %q", tag, common.VelocityRising)
	}
}
