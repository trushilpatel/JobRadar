package insights

import (
	"testing"

	"github.com/trushilpatel/jobradar/service/insights/dto"
	svc "github.com/trushilpatel/jobradar/service/insights/services"
)

func TestValidateFilter(t *testing.T) {
	v := svc.NewValidationService()

	if err := v.ValidateFilter(dto.InsightsFilter{Days: 1}); err != nil {
		t.Errorf("unexpected error for days=1: %v", err)
	}
	if err := v.ValidateFilter(dto.InsightsFilter{Days: 30}); err != nil {
		t.Errorf("unexpected error for days=30: %v", err)
	}
	if err := v.ValidateFilter(dto.InsightsFilter{Days: 0}); err == nil {
		t.Error("expected error for days=0")
	}
	if err := v.ValidateFilter(dto.InsightsFilter{Days: -1}); err == nil {
		t.Error("expected error for days=-1")
	}
}
