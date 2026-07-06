// Package services holds the decorator chain for the resume domain.
package services

import (
	"context"
	"io"

	"github.com/trushilpatel/jobradar/service/resume/dto"
	resumemodel "github.com/trushilpatel/jobradar/service/resume/models"
)

type Service interface {
	List(ctx context.Context, userID string) ([]resumemodel.Resume, error)
	Get(ctx context.Context, userID, id string) (*resumemodel.Resume, error)
	Upload(ctx context.Context, userID string, input dto.UploadInput) (*resumemodel.Resume, error)
	Rename(ctx context.Context, userID, id, label string) (*resumemodel.Resume, error)
	Delete(ctx context.Context, userID, id string) error
	// Download returns a reader over the resume's object-storage bytes plus
	// its metadata row (for content-type/filename/size) — the REST layer
	// streams the reader directly to the client. Callers must Close() it.
	Download(ctx context.Context, userID, id string) (io.ReadCloser, *resumemodel.Resume, error)
}

type BaseService struct {
	next Service
}

func NewBaseService(next Service) BaseService {
	if next == nil {
		panic("resume.BaseService: next must not be nil — check wire setup")
	}
	return BaseService{next: next}
}

func (b *BaseService) List(ctx context.Context, userID string) ([]resumemodel.Resume, error) {
	return b.next.List(ctx, userID)
}

func (b *BaseService) Get(ctx context.Context, userID, id string) (*resumemodel.Resume, error) {
	return b.next.Get(ctx, userID, id)
}

func (b *BaseService) Upload(ctx context.Context, userID string, input dto.UploadInput) (*resumemodel.Resume, error) {
	return b.next.Upload(ctx, userID, input)
}

func (b *BaseService) Rename(ctx context.Context, userID, id, label string) (*resumemodel.Resume, error) {
	return b.next.Rename(ctx, userID, id, label)
}

func (b *BaseService) Delete(ctx context.Context, userID, id string) error {
	return b.next.Delete(ctx, userID, id)
}

func (b *BaseService) Download(ctx context.Context, userID, id string) (io.ReadCloser, *resumemodel.Resume, error) {
	return b.next.Download(ctx, userID, id)
}
