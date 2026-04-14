package insights

import "testing"

func TestValidateFilter(t *testing.T) {
	v := NewValidator()

	if err := v.ValidateFilter(InsightsFilter{Days: 1}); err != nil {
		t.Errorf("unexpected error for days=1: %v", err)
	}
	if err := v.ValidateFilter(InsightsFilter{Days: 30}); err != nil {
		t.Errorf("unexpected error for days=30: %v", err)
	}
	if err := v.ValidateFilter(InsightsFilter{Days: 0}); err == nil {
		t.Error("expected error for days=0")
	}
	if err := v.ValidateFilter(InsightsFilter{Days: -1}); err == nil {
		t.Error("expected error for days=-1")
	}
}
