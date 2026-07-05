package services

import (
	"context"
	"strings"
	"unicode/utf8"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	resumecommon "github.com/trushilpatel/jobradar/service/resume/common"
	"github.com/trushilpatel/jobradar/service/resume/dto"
	"github.com/trushilpatel/jobradar/service/resume/filetype"
	resumemodel "github.com/trushilpatel/jobradar/service/resume/models"
)

type validationService struct{ BaseService }

func NewValidationService(next Service) Service {
	return &validationService{BaseService: NewBaseService(next)}
}

// Upload validates size/filename bounds, sniffs the real content type via
// magic bytes (never trusting the client's declared type or extension), and
// falls back the label to the filename (minus extension) when blank.
func (v *validationService) Upload(ctx context.Context, userID string, input dto.UploadInput) (*resumemodel.Resume, error) {
	var errs domainerrs.ValidationErrors

	if len(input.RawBytes) == 0 {
		errs = append(errs, domainerrs.ValidationError{Field: "file", Code: "REQUIRED", Message: "file must not be empty"})
	}
	if int64(len(input.RawBytes)) > resumecommon.MaxFileSizeBytes {
		errs = append(errs, domainerrs.ValidationError{Field: "file", Code: "TOO_LARGE", Message: "file exceeds the 2MB limit"})
	}
	if utf8.RuneCountInString(input.OriginalFilename) > resumecommon.OriginalFilenameMaxLen {
		errs = append(errs, domainerrs.ValidationError{Field: "filename", Code: "TOO_LONG", Message: "filename is too long"})
	}
	if len(errs) > 0 {
		return nil, errs
	}

	contentType, ext, detectErr := filetype.Detect(input.RawBytes)
	if detectErr != nil {
		return nil, domainerrs.ValidationErrors{{
			Field: "file", Code: "UNSUPPORTED_TYPE", Message: "only PDF and DOCX files are supported",
		}}
	}
	input.ContentType = contentType
	input.Extension = ext

	label := strings.TrimSpace(input.Label)
	if label == "" {
		label = strings.TrimSuffix(input.OriginalFilename, "."+ext)
	}
	if utf8.RuneCountInString(label) > resumecommon.LabelMaxLen {
		return nil, domainerrs.ValidationErrors{{Field: "label", Code: "TOO_LONG", Message: "label must be at most 200 characters"}}
	}
	input.Label = label

	return v.next.Upload(ctx, userID, input)
}

// Rename requires a non-empty (after trim) label — unlike Upload, there's no
// original filename in scope to fall back to for an explicit rename, and an
// explicit rename-to-blank is ambiguous UX rather than an obvious "reset".
func (v *validationService) Rename(ctx context.Context, userID, id, label string) (*resumemodel.Resume, error) {
	trimmed := strings.TrimSpace(label)
	if trimmed == "" {
		return nil, domainerrs.ValidationErrors{{Field: "label", Code: "REQUIRED", Message: "label must not be empty"}}
	}
	if utf8.RuneCountInString(trimmed) > resumecommon.LabelMaxLen {
		return nil, domainerrs.ValidationErrors{{Field: "label", Code: "TOO_LONG", Message: "label must be at most 200 characters"}}
	}
	return v.next.Rename(ctx, userID, id, trimmed)
}

var _ Service = (*validationService)(nil)
