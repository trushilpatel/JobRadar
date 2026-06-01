package company_test

import (
	"context"
	"testing"

	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
)

// publicSearchPassThrough is a terminal CompanyService that records whether
// ListPublic was reached, so the validator's gate can be observed.
type publicSearchPassThrough struct {
	listed bool
}

func (p *publicSearchPassThrough) ListPublic(_ context.Context, _ companydto.CompanyFilter, _ int) (companydto.CompanyPublicListResult, error) {
	p.listed = true
	return companydto.CompanyPublicListResult{}, nil
}

func (p *publicSearchPassThrough) GetPublic(_ context.Context, _ string) (*companydto.CompanyPublicView, error) {
	return nil, nil
}

var _ companysvc.CompanyService = (*publicSearchPassThrough)(nil)

// TestPublicValidator_SearchLengthBoundary verifies P2 parity: the public list
// validator caps search at 100 chars exactly — 100 passes through to the chain,
// 101 is rejected before reaching the DB (the chain is never invoked).
func TestPublicValidator_SearchLengthBoundary(t *testing.T) {
	cases := []struct {
		name      string
		searchLen int
		wantErr   bool
	}{
		{"empty ok", 0, false},
		{"100 ok", 100, false},
		{"101 rejected", 101, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			term := &publicSearchPassThrough{}
			v := companysvc.NewValidationService(term)
			_, err := v.ListPublic(context.Background(),
				companydto.CompanyFilter{Search: stringOfLen(tc.searchLen)}, 20)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErr && term.listed {
				t.Errorf("over-length search must be rejected before reaching the chain")
			}
			if !tc.wantErr && !term.listed {
				t.Errorf("valid search must reach the chain")
			}
		})
	}
}
