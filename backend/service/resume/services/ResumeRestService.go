package services

import (
	"context"
	"io"
	"mime"
	"strconv"

	"github.com/danielgtaylor/huma/v2"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/identity"
	"github.com/trushilpatel/jobradar/service/resume/dto"
	resumemodel "github.com/trushilpatel/jobradar/service/resume/models"
)

type RestService struct {
	next Service
}

func NewRestService(next Service) *RestService {
	if next == nil {
		panic("resume.NewRestService: next must not be nil")
	}
	return &RestService{next: next}
}

// list serves GET /api/resumes.
func (h *RestService) list(ctx context.Context, _ *struct{}) (*apihuma.SingleOutput[[]resumemodel.Resume], error) {
	userID := identity.MustFromContext(ctx).UserID
	resumes, err := h.next.List(ctx, userID)
	if err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[[]resumemodel.Resume]{}
	out.Body.Data = resumes
	return out, nil
}

// uploadFormData is the multipart form shape for POST /api/resumes.
type uploadFormData struct {
	File  huma.FormFile `form:"file" required:"true" doc:"The resume file (PDF or DOCX)"`
	Label string        `form:"label" required:"false" doc:"Optional display label; defaults to the filename"`
}

// uploadInput is the Huma input for POST /api/resumes. Huma's native
// multipart handling (huma.MultipartFormFiles) is used rather than a raw
// RawBody/manual-parse escape hatch, so the multipart schema (file +
// label fields) is correctly documented in the generated OpenAPI — a real,
// disclosed trade-off: a missing file or malformed multipart body now
// surfaces as Huma's field-list validation shape
// ({"errors":[{"field":"file",...}]}), not the legacy handler's exact
// single-error {"error":{"code":"BAD_REQUEST","message":"file is required"}}.
// Both are 400s; no existing test asserted the exact legacy message for
// these two cases, and correct OpenAPI docs for the upload form is worth
// more than byte-parity here (see docs/superpowers/plans/2026-06-13-huma-migration-plan.md,
// Slice 5 notes).
type uploadInput struct {
	RawBody huma.MultipartFormFiles[uploadFormData]
}

// upload serves POST /api/resumes.
func (h *RestService) upload(ctx context.Context, in *uploadInput) (*apihuma.SingleOutput[*resumemodel.Resume], error) {
	userID := identity.MustFromContext(ctx).UserID
	data := in.RawBody.Data()

	file := data.File
	defer file.Close()

	raw, err := io.ReadAll(file)
	if err != nil {
		return nil, apihuma.MapError(apperr.ErrBadRequest.WithExternal("failed to read uploaded file"))
	}

	input := dto.UploadInput{
		Label:            data.Label,
		OriginalFilename: file.Filename,
		RawBytes:         raw,
		DeclaredSize:     int64(len(raw)),
	}

	resume, err := h.next.Upload(ctx, userID, input)
	if err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[*resumemodel.Resume]{}
	out.Body.Data = resume
	return out, nil
}

// downloadInput is the Huma input for GET /api/resumes/{id}/download.
type downloadInput struct {
	ID string `path:"id"`
}

// downloadOutput uses a Body func(huma.Context) — the same escape hatch
// used by the apply-queue slice — because this response's Content-Type,
// Content-Disposition, and Content-Length headers are all dynamic (derived
// per-resume) and the body is streamed directly from object storage rather
// than buffered into a typed field.
type downloadOutput struct {
	Body func(huma.Context)
}

// download serves GET /api/resumes/{id}/download.
func (h *RestService) download(ctx context.Context, in *downloadInput) (*downloadOutput, error) {
	userID := identity.MustFromContext(ctx).UserID
	reader, resume, err := h.next.Download(ctx, userID, in.ID)
	if err != nil {
		return nil, apihuma.MapError(err)
	}
	return &downloadOutput{Body: func(hctx huma.Context) {
		defer reader.Close()
		hctx.SetHeader("Content-Type", resume.ContentType)
		hctx.SetHeader("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": resume.OriginalFilename}))
		hctx.SetHeader("Content-Length", strconv.FormatInt(resume.SizeBytes, 10))
		hctx.SetStatus(200)
		// Headers are already committed at this point — an io.Copy failure
		// here (client disconnect, storage read timeout) cannot change the
		// response status, so this deliberately ignores the copy error.
		// Nothing else to do but let the partial transfer end.
		_, _ = io.Copy(hctx.BodyWriter(), reader)
	}}, nil
}

// renameInput is the Huma input for PATCH /api/resumes/{id}.
type renameInput struct {
	ID   string `path:"id"`
	Body struct {
		// required:"false" matches the pre-Huma handler's plain
		// json.Decode, which leaves an absent key at its Go zero value.
		Label string `json:"label" required:"false"`
	}
}

// rename serves PATCH /api/resumes/{id}.
func (h *RestService) rename(ctx context.Context, in *renameInput) (*apihuma.SingleOutput[*resumemodel.Resume], error) {
	userID := identity.MustFromContext(ctx).UserID
	resume, err := h.next.Rename(ctx, userID, in.ID, in.Body.Label)
	if err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[*resumemodel.Resume]{}
	out.Body.Data = resume
	return out, nil
}

// removeInput is the Huma input for DELETE /api/resumes/{id}.
type removeInput struct {
	ID string `path:"id"`
}

// removeOutput has no Body field, so Huma defaults its status to 204.
type removeOutput struct{}

// remove serves DELETE /api/resumes/{id}.
func (h *RestService) remove(ctx context.Context, in *removeInput) (*removeOutput, error) {
	userID := identity.MustFromContext(ctx).UserID
	if err := h.next.Delete(ctx, userID, in.ID); err != nil {
		return nil, apihuma.MapError(err)
	}
	return &removeOutput{}, nil
}
