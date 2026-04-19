package company

import (
	"context"

	"github.com/trushilpatel/jobradar/domain"
	"github.com/trushilpatel/jobradar/infra/tracing"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	"github.com/trushilpatel/jobradar/store"
)

// CrawlerReader is the narrow company read interface for the crawler pipeline.
type CrawlerReader interface {
	GetByID(ctx context.Context, id string) (companymodel.Company, error)
	GetActive(ctx context.Context) ([]companymodel.Company, error)
}

type crawlerReader struct {
	store Store
}

// NewCrawlerReader returns a CrawlerReader backed by the company store.
func NewCrawlerReader(s Store) CrawlerReader {
	return &crawlerReader{store: s}
}

func (r *crawlerReader) GetByID(ctx context.Context, id string) (_ companymodel.Company, err error) {
	ctx, span := tracing.Start(ctx, "company.crawlerreader.GetByID")
	defer tracing.End(span, &err)
	c, err := r.store.GetCompanyByID(ctx, id)
	if err != nil {
		if store.IsNotFound(err) {
			return companymodel.Company{}, domain.ErrNotFound.WithMessage("company not found")
		}
		return companymodel.Company{}, domain.ErrInternal.WithMessage("failed to fetch company")
	}
	return c, nil
}

func (r *crawlerReader) GetActive(ctx context.Context) (_ []companymodel.Company, err error) {
	ctx, span := tracing.Start(ctx, "company.crawlerreader.GetActive")
	defer tracing.End(span, &err)
	companies, err := r.store.GetActiveCompanies(ctx)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch active companies")
	}
	return companies, nil
}
