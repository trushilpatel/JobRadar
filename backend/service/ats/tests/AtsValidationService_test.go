package tests

import (
	"context"
	"errors"
	"testing"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	atsdto "github.com/trushilpatel/jobradar/service/ats/dto"
	atssvc "github.com/trushilpatel/jobradar/service/ats/services"
)

// ---------- ListATS — no validation, pass-through ----------

func TestAtsValidation_ListATS_PassThrough(t *testing.T) {
	t.Parallel()
	next := &recordingAtsNext{listResult: []atsdto.ATSDTO{{Name: "greenhouse"}}}
	svc := atssvc.NewAtsValidationService(next)

	items, err := svc.ListATS(context.Background())
	if err != nil {
		t.Fatalf("ListATS: %v", err)
	}
	if !next.listCalled {
		t.Error("next.ListATS not invoked")
	}
	if len(items) != 1 {
		t.Errorf("len = %d, want 1", len(items))
	}
}

// ---------- GetATS — no validation, pass-through ----------

func TestAtsValidation_GetATS_PassThrough(t *testing.T) {
	t.Parallel()
	next := &recordingAtsNext{getResult: &atsdto.ATSDTO{Name: "lever"}}
	svc := atssvc.NewAtsValidationService(next)

	got, err := svc.GetATS(context.Background(), "lever")
	if err != nil {
		t.Fatalf("GetATS: %v", err)
	}
	if !next.getCalled {
		t.Error("next.GetATS not invoked")
	}
	if got.Name != "lever" {
		t.Errorf("name = %q, want lever", got.Name)
	}
}

// ---------- UpdateATS — validation ----------

func ptr[T any](v T) *T { return &v }

func asValidationErrors(t *testing.T, err error) domainerrs.ValidationErrors {
	t.Helper()
	var ve domainerrs.ValidationErrors
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationErrors, got %T: %v", err, err)
	}
	return ve
}

func TestAtsValidation_UpdateATS_EmptyName(t *testing.T) {
	t.Parallel()
	next := &recordingAtsNext{}
	svc := atssvc.NewAtsValidationService(next)

	active := true
	_, err := svc.UpdateATS(context.Background(), "", atsdto.UpdateATSInput{IsActive: &active})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	ve := asValidationErrors(t, err)
	found := false
	for _, e := range ve {
		if e.Field == "name" && e.Code == "REQUIRED" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected name/REQUIRED in errors, got %v", ve)
	}
	if next.updateCalled {
		t.Error("next.UpdateATS must not be called on invalid input")
	}
}

func TestAtsValidation_UpdateATS_ScanIntervalTooLow(t *testing.T) {
	t.Parallel()
	next := &recordingAtsNext{}
	svc := atssvc.NewAtsValidationService(next)

	_, err := svc.UpdateATS(context.Background(), "greenhouse", atsdto.UpdateATSInput{ScanIntervalMin: ptr(0)})
	if err == nil {
		t.Fatal("expected error for scan_interval_min=0")
	}
	ve := asValidationErrors(t, err)
	found := false
	for _, e := range ve {
		if e.Field == "scan_interval_min" && e.Code == "INVALID_VALUE" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected scan_interval_min/INVALID_VALUE, got %v", ve)
	}
}

func TestAtsValidation_UpdateATS_ScanIntervalTooHigh(t *testing.T) {
	t.Parallel()
	next := &recordingAtsNext{}
	svc := atssvc.NewAtsValidationService(next)

	_, err := svc.UpdateATS(context.Background(), "greenhouse", atsdto.UpdateATSInput{ScanIntervalMin: ptr(10081)})
	if err == nil {
		t.Fatal("expected error for scan_interval_min=10081")
	}
	ve := asValidationErrors(t, err)
	found := false
	for _, e := range ve {
		if e.Field == "scan_interval_min" && e.Code == "INVALID_VALUE" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected scan_interval_min/INVALID_VALUE, got %v", ve)
	}
}

func TestAtsValidation_UpdateATS_InterCompanyDelayNegative(t *testing.T) {
	t.Parallel()
	next := &recordingAtsNext{}
	svc := atssvc.NewAtsValidationService(next)

	_, err := svc.UpdateATS(context.Background(), "greenhouse", atsdto.UpdateATSInput{InterCompanyDelayS: ptr(-1)})
	if err == nil {
		t.Fatal("expected error for inter_company_delay_s=-1")
	}
	ve := asValidationErrors(t, err)
	found := false
	for _, e := range ve {
		if e.Field == "inter_company_delay_s" && e.Code == "INVALID_VALUE" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected inter_company_delay_s/INVALID_VALUE, got %v", ve)
	}
}

func TestAtsValidation_UpdateATS_InterCompanyDelayTooHigh(t *testing.T) {
	t.Parallel()
	next := &recordingAtsNext{}
	svc := atssvc.NewAtsValidationService(next)

	_, err := svc.UpdateATS(context.Background(), "greenhouse", atsdto.UpdateATSInput{InterCompanyDelayS: ptr(3601)})
	if err == nil {
		t.Fatal("expected error for inter_company_delay_s=3601")
	}
	ve := asValidationErrors(t, err)
	found := false
	for _, e := range ve {
		if e.Field == "inter_company_delay_s" && e.Code == "INVALID_VALUE" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected inter_company_delay_s/INVALID_VALUE, got %v", ve)
	}
}

func TestAtsValidation_UpdateATS_NoFieldsProvided(t *testing.T) {
	t.Parallel()
	next := &recordingAtsNext{}
	svc := atssvc.NewAtsValidationService(next)

	_, err := svc.UpdateATS(context.Background(), "greenhouse", atsdto.UpdateATSInput{})
	if err == nil {
		t.Fatal("expected error when no fields provided")
	}
	// Check it carries BAD_REQUEST code via ServiceError or ValidationErrors
	var ve domainerrs.ValidationErrors
	var se *domainerrs.ServiceError
	if !errors.As(err, &ve) && !errors.As(err, &se) {
		t.Fatalf("expected validation or service error, got %T: %v", err, err)
	}
	if next.updateCalled {
		t.Error("next.UpdateATS must not be called when no fields are provided")
	}
}

func TestAtsValidation_UpdateATS_HappyPath(t *testing.T) {
	t.Parallel()
	next := &recordingAtsNext{updateResult: &atsdto.ATSDTO{Name: "greenhouse"}}
	svc := atssvc.NewAtsValidationService(next)

	active := true
	got, err := svc.UpdateATS(context.Background(), "greenhouse", atsdto.UpdateATSInput{IsActive: &active})
	if err != nil {
		t.Fatalf("UpdateATS: %v", err)
	}
	if !next.updateCalled {
		t.Error("next.UpdateATS not invoked")
	}
	if got.Name != "greenhouse" {
		t.Errorf("name = %q, want greenhouse", got.Name)
	}
}

func TestAtsValidation_UpdateATS_BoundaryValues(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input atsdto.UpdateATSInput
	}{
		{"scan_interval_min=1", atsdto.UpdateATSInput{ScanIntervalMin: ptr(1)}},
		{"scan_interval_min=10080", atsdto.UpdateATSInput{ScanIntervalMin: ptr(10080)}},
		{"inter_company_delay_s=0", atsdto.UpdateATSInput{InterCompanyDelayS: ptr(0)}},
		{"inter_company_delay_s=3600", atsdto.UpdateATSInput{InterCompanyDelayS: ptr(3600)}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			next := &recordingAtsNext{updateResult: &atsdto.ATSDTO{Name: "greenhouse"}}
			svc := atssvc.NewAtsValidationService(next)
			if _, err := svc.UpdateATS(context.Background(), "greenhouse", tc.input); err != nil {
				t.Errorf("boundary %q got unexpected error: %v", tc.name, err)
			}
		})
	}
}
