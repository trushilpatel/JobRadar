package company_test

import (
	"context"
	"testing"

	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
)

// noopPublic is the terminal pass-through used under the validation layer.
type noopPublic struct{}

func (n *noopPublic) ListPublic(_ context.Context, _ companydto.CompanyFilter, _ int) (companydto.CompanyPublicListResult, error) {
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
		limit   int
		wantErr bool
	}{
		{name: "ok defaults", limit: 20},
		{name: "limit too large", limit: 200, wantErr: true},
		{name: "search too long", filter: companydto.CompanyFilter{Search: stringOfLen(101)}, limit: 20, wantErr: true},
		{name: "limit=0 rejects", limit: 0, wantErr: true},
		{name: "search exactly 100 chars passes", filter: companydto.CompanyFilter{Search: stringOfLen(100)}, limit: 20},
		{name: "page=1 limit=1 minimum valid", limit: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := v.ListPublic(context.Background(), tc.filter, tc.limit)
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
