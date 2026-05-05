package company_test

import (
	"context"
	"testing"

	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
)

// noopPublic is the terminal pass-through used under the validation layer.
type noopPublic struct{}

func (n *noopPublic) ListPublic(_ context.Context, _ companydto.CompanyFilter, _, _ int) (companydto.CompanyPublicListResult, error) {
	return companydto.CompanyPublicListResult{}, nil
}
func (n *noopPublic) GetPublic(_ context.Context, _ string) (*companydto.CompanyPublicView, error) {
	return nil, nil
}

func newPublicValidator() companysvc.CompanyService {
	return companysvc.NewValidationService(&noopPublic{})
}

func TestPublicValidator_ListPublic(t *testing.T) {
	v := newPublicValidator()

	cases := []struct {
		name    string
		filter  companydto.CompanyFilter
		page    int
		limit   int
		wantErr bool
	}{
		{name: "ok defaults", page: 1, limit: 20},
		{name: "page zero", page: 0, limit: 20, wantErr: true},
		{name: "limit too large", page: 1, limit: 200, wantErr: true},
		{name: "search too long", filter: companydto.CompanyFilter{Search: stringOfLen(101)}, page: 1, limit: 20, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := v.ListPublic(context.Background(), tc.filter, tc.page, tc.limit)
			if (err != nil) != tc.wantErr {
				t.Errorf("ListPublic: err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func stringOfLen(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'x'
	}
	return string(b)
}

var _ companysvc.CompanyService = (*noopPublic)(nil)
