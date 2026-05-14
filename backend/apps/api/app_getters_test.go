package main

import (
	"testing"

	companysvc "github.com/trushilpatel/jobradar/service/company/services"
	insightssvc "github.com/trushilpatel/jobradar/service/insights/services"
)

// TestApp_InsightsHandler_ReturnsWiredHandler asserts the getter surfaces the
// exact insights router passed into NewApp — main.go relies on this to mount
// insights under /admin via buildMultiUser.
func TestApp_InsightsHandler_ReturnsWiredHandler(t *testing.T) {
	// Build a real (empty-deps) insights router so the pointer comparison is
	// meaningful — non-nil and matched against the same instance.
	insights := &insightssvc.RestRouter{}
	app := NewApp(nil, nil, nil, nil, nil, nil, insights, &fakePipelineService{}, nil)
	if got := app.InsightsHandler(); got != insights {
		t.Fatalf("InsightsHandler() = %p, want %p", got, insights)
	}
}

func TestApp_CompanyAdminHandler_ReturnsWiredHandler(t *testing.T) {
	companyAdmin := &companysvc.AdminRestRouter{}
	app := NewApp(nil, nil, nil, nil, nil, nil, nil, &fakePipelineService{}, companyAdmin)
	if got := app.CompanyAdminHandler(); got != companyAdmin {
		t.Fatalf("CompanyAdminHandler() = %p, want %p", got, companyAdmin)
	}
}

func TestApp_PipelineSvc_ReturnsWiredSvc(t *testing.T) {
	pipe := &fakePipelineService{}
	app := NewApp(nil, nil, nil, nil, nil, nil, nil, pipe, nil)
	if got := app.PipelineSvc(); got != pipe {
		t.Fatalf("PipelineSvc() = %p, want %p", got, pipe)
	}
}

// TestApp_Getters_NilWhenNotWired covers the path where main builds an App
// with nil for optional handlers (rare in production but happens in tests).
// The getters must surface nil — never panic.
func TestApp_Getters_NilWhenNotWired(t *testing.T) {
	app := NewApp(nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if app.InsightsHandler() != nil {
		t.Error("InsightsHandler should be nil when not wired")
	}
	if app.CompanyAdminHandler() != nil {
		t.Error("CompanyAdminHandler should be nil when not wired")
	}
	if app.PipelineSvc() != nil {
		t.Error("PipelineSvc should be nil when not wired")
	}
}
