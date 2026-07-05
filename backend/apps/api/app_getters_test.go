package main

import (
	"testing"

	atssvc "github.com/trushilpatel/jobradar/service/ats/services"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
	pipelinesvc "github.com/trushilpatel/jobradar/service/pipeline/services"
)

func TestApp_CompanyAdminHandler_ReturnsWiredHandler(t *testing.T) {
	companyAdmin := &companysvc.AdminRestRouter{}
	app := NewApp(nil, nil, nil, nil, nil, companyAdmin, nil, nil)
	if got := app.CompanyAdminHandler(); got != companyAdmin {
		t.Fatalf("CompanyAdminHandler() = %p, want %p", got, companyAdmin)
	}
}

func TestApp_AtsHandler_ReturnsWiredRouter(t *testing.T) {
	ats := &atssvc.AtsRestRouter{}
	app := NewApp(nil, nil, ats, nil, nil, nil, nil, nil)
	if got := app.AtsHandler(); got != ats {
		t.Fatalf("AtsHandler() = %p, want %p", got, ats)
	}
}

func TestApp_AtsHandler_NilWhenNotWired(t *testing.T) {
	app := NewApp(nil, nil, nil, nil, nil, nil, nil, nil)
	if app.AtsHandler() != nil {
		t.Error("AtsHandler should be nil when not wired")
	}
}

func TestApp_CompanyAdminHandler_NilWhenNotWired(t *testing.T) {
	app := NewApp(nil, nil, nil, nil, nil, nil, nil, nil)
	if app.CompanyAdminHandler() != nil {
		t.Error("CompanyAdminHandler should be nil when not wired")
	}
}

func TestApp_PipelineHandler_ReturnsWiredRouter(t *testing.T) {
	pipeline := &pipelinesvc.PipelineRestRouter{}
	app := NewApp(nil, nil, nil, nil, nil, nil, nil, pipeline)
	if got := app.PipelineHandler(); got != pipeline {
		t.Fatalf("PipelineHandler() = %p, want %p", got, pipeline)
	}
}

func TestApp_PipelineHandler_NilWhenNotWired(t *testing.T) {
	app := NewApp(nil, nil, nil, nil, nil, nil, nil, nil)
	if app.PipelineHandler() != nil {
		t.Error("PipelineHandler should be nil when not wired")
	}
}
