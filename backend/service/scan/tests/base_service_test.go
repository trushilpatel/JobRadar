package scan

import (
	"context"
	"testing"

	"github.com/trushilpatel/jobradar/service/scan/dto"
	svc "github.com/trushilpatel/jobradar/service/scan/services"
)

// TestNewBaseService_NilNextPanics asserts the wiring guard fires when
// callers forget to wire the next layer. The constructor panics so wiring
// bugs surface at startup, not during the first request.
func TestNewBaseService_NilNextPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil next")
		}
	}()
	_ = svc.NewBaseService(nil)
}

// basePassthroughService composes BaseService with a noop next and exposes
// no overrides — it is used to assert that BaseService auto-delegates every
// Service method to next. Methods overridden by implService (TriggerScan,
// ListQueue) would otherwise never reach the BaseService variant, so this
// is the only path that exercises those delegation methods.
type basePassthroughService struct {
	svc.BaseService
}

func newBasePassthroughService(next svc.Service) svc.Service {
	return &basePassthroughService{BaseService: svc.NewBaseService(next)}
}

func TestBaseService_DelegatesTriggerScanAndListQueue(t *testing.T) {
	noop := &noopService{}
	base := newBasePassthroughService(noop)

	if err := base.TriggerScan(context.Background(), "any"); err != nil {
		t.Errorf("TriggerScan delegate error = %v", err)
	}
	if _, err := base.ListQueue(context.Background(), dto.QueueFilter{}); err != nil {
		t.Errorf("ListQueue delegate error = %v", err)
	}
}

// TestBaseService_DelegatesGetEffectiveScanConfig exercises the auto-delegation
// of the two new methods through BaseService.
func TestBaseService_DelegatesGetEffectiveScanConfig(t *testing.T) {
	noop := &noopService{}
	base := newBasePassthroughService(noop)

	if _, err := base.GetEffectiveScanConfig(context.Background(), "stripe"); err != nil {
		t.Errorf("GetEffectiveScanConfig delegate error = %v", err)
	}
}

func TestBaseService_DelegatesUpdateScanIntervalOverride(t *testing.T) {
	noop := &noopService{}
	base := newBasePassthroughService(noop)

	if err := base.UpdateScanIntervalOverride(context.Background(), "stripe", nil); err != nil {
		t.Errorf("UpdateScanIntervalOverride delegate error = %v", err)
	}
}
