package company_test

import (
	"context"
	"errors"
	"testing"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
)

// TestScanSettingsRepository_RejectsEmptyCompanyID locks down the per-method
// guard the repo uses to refuse calls without a companyID. Workers and
// admin paths both depend on this — a missing id must never silently
// broaden a query to the entire table.
//
// The repo here is constructed with a nil *gorm.DB on purpose: the guard
// runs before any DB call, so reaching the DB would itself be a failure.
func TestScanSettingsRepository_RejectsEmptyCompanyID(t *testing.T) {
	repo := companysvc.NewScanSettingsRepositoryService(nil)

	cases := []struct {
		name string
		fn   func() error
	}{
		{
			name: "Get",
			fn: func() error {
				_, err := repo.Get(context.Background(), "")
				return err
			},
		},
		{
			name: "Create",
			fn: func() error {
				return repo.Create(context.Background(), companymodel.CompanyScanSettings{})
			},
		},
		{
			name: "Update",
			fn: func() error {
				return repo.Update(context.Background(), "", companydto.CompanyScanSettingsInput{Active: boolPtrTrue()})
			},
		},
		{
			name: "UpdateScanState",
			fn: func() error {
				return repo.UpdateScanState(context.Background(), "", companydto.CompanyScanStateUpdate{})
			},
		},
		{
			name: "Delete",
			fn: func() error {
				return repo.Delete(context.Background(), "")
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn()
			if err == nil {
				t.Fatalf("expected error from %s with empty companyID", tc.name)
			}
			var sErr *apperr.ServiceError
			if !errors.As(err, &sErr) {
				t.Fatalf("expected ServiceError, got %T: %v", err, err)
			}
			if sErr.Code != "INTERNAL" {
				t.Errorf("expected INTERNAL guard error, got %s (%s)", sErr.Code, sErr.InternalMessage)
			}
		})
	}
}

func boolPtrTrue() *bool { b := true; return &b }
