package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/identity"
	resumemodel "github.com/trushilpatel/jobradar/service/resume/models"
	resumesvc "github.com/trushilpatel/jobradar/service/resume/services"
)

func withUser(r *http.Request, userID string) *http.Request {
	return r.WithContext(identity.WithTestIdentity(r.Context(), userID))
}

func multipartBody(t *testing.T, label, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	if label != "" {
		_ = w.WriteField("label", label)
	}
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write file part: %v", err)
	}
	_ = w.Close()
	return buf, w.FormDataContentType()
}

func TestResumeRest_UploadReturns201OnSuccess(t *testing.T) {
	fake := &fakeService{uploadResult: &resumemodel.Resume{ID: "r1", Label: "My resume"}}
	h := resumesvc.NewRestService(fake)
	router := resumesvc.NewRestRouter(h)
	mux := chi.NewRouter()
	router.Register(mux)

	body, contentType := multipartBody(t, "My resume", "resume.pdf", []byte("%PDF-1.4"))
	req := httptest.NewRequest(http.MethodPost, "/resumes", body)
	req.Header.Set("Content-Type", contentType)
	req = withUser(req, "u1")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestResumeRest_UploadReturns400OnValidationFailure(t *testing.T) {
	fake := &fakeService{uploadErr: domainerrs.ValidationErrors{{
		Field: "file", Code: "UNSUPPORTED_TYPE", Message: "only PDF and DOCX files are supported",
	}}}
	h := resumesvc.NewRestService(fake)
	router := resumesvc.NewRestRouter(h)
	mux := chi.NewRouter()
	router.Register(mux)

	body, contentType := multipartBody(t, "", "resume.pdf", []byte(""))
	req := httptest.NewRequest(http.MethodPost, "/resumes", body)
	req.Header.Set("Content-Type", contentType)
	req = withUser(req, "u1")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestResumeRest_DownloadStreamsFileWithHeaders(t *testing.T) {
	fake := &fakeService{
		downloadReader: io.NopCloser(bytes.NewReader([]byte("%PDF-1.4 test content"))),
		downloadResume: &resumemodel.Resume{
			ID: "r1", OriginalFilename: "My Resume.pdf", ContentType: "application/pdf",
			SizeBytes: 22,
		},
	}
	h := resumesvc.NewRestService(fake)
	router := resumesvc.NewRestRouter(h)
	mux := chi.NewRouter()
	router.Register(mux)

	req := withUser(httptest.NewRequest(http.MethodGet, "/resumes/r1/download", nil), "u1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("expected Content-Type application/pdf, got %s", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename="My Resume.pdf"` {
		t.Fatalf("expected attachment Content-Disposition, got %s", cd)
	}
	if cl := rec.Header().Get("Content-Length"); cl != "22" {
		t.Fatalf("expected Content-Length 22, got %s", cl)
	}
	if rec.Body.String() != "%PDF-1.4 test content" {
		t.Fatalf("expected streamed file bytes, got %q", rec.Body.String())
	}
}

func TestResumeRest_DownloadReturns404ForMissingResume(t *testing.T) {
	fake := &fakeService{downloadErr: domainerrs.ErrNotFound.WithMessage("resume not found")}
	h := resumesvc.NewRestService(fake)
	router := resumesvc.NewRestRouter(h)
	mux := chi.NewRouter()
	router.Register(mux)

	req := withUser(httptest.NewRequest(http.MethodGet, "/resumes/missing/download", nil), "u1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestResumeRest_RenameReturns200OnSuccess(t *testing.T) {
	fake := &fakeService{}
	h := resumesvc.NewRestService(fake)
	router := resumesvc.NewRestRouter(h)
	mux := chi.NewRouter()
	router.Register(mux)

	body, _ := json.Marshal(map[string]string{"label": "New label"})
	req := withUser(httptest.NewRequest(http.MethodPatch, "/resumes/r1", bytes.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.lastRenameLabel != "New label" {
		t.Fatalf("expected the handler to pass through the new label, got %q", fake.lastRenameLabel)
	}
}

func TestResumeRest_RenameReturns400OnInvalidBody(t *testing.T) {
	fake := &fakeService{}
	h := resumesvc.NewRestService(fake)
	router := resumesvc.NewRestRouter(h)
	mux := chi.NewRouter()
	router.Register(mux)

	req := withUser(httptest.NewRequest(http.MethodPatch, "/resumes/r1", bytes.NewReader([]byte("not json"))), "u1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestResumeRest_DeleteReturns204OnSuccess(t *testing.T) {
	fake := &fakeService{}
	h := resumesvc.NewRestService(fake)
	router := resumesvc.NewRestRouter(h)
	mux := chi.NewRouter()
	router.Register(mux)

	req := withUser(httptest.NewRequest(http.MethodDelete, "/resumes/r1", nil), "u1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

func TestResumeRest_DeleteReturns404ForMissingResume(t *testing.T) {
	fake := &fakeService{deleteErr: domainerrs.ErrNotFound.WithMessage("resume not found")}
	h := resumesvc.NewRestService(fake)
	router := resumesvc.NewRestRouter(h)
	mux := chi.NewRouter()
	router.Register(mux)

	req := withUser(httptest.NewRequest(http.MethodDelete, "/resumes/missing", nil), "u1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestResumeRest_ListReturns200(t *testing.T) {
	fake := &fakeService{}
	h := resumesvc.NewRestService(fake)
	router := resumesvc.NewRestRouter(h)
	mux := chi.NewRouter()
	router.Register(mux)

	req := withUser(httptest.NewRequest(http.MethodGet, "/resumes", nil), "u1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestResumeRest_ListReturns500OnServiceError(t *testing.T) {
	fake := &fakeService{listErr: domainerrs.ErrInternal.WithMessage("db down")}
	h := resumesvc.NewRestService(fake)
	router := resumesvc.NewRestRouter(h)
	mux := chi.NewRouter()
	router.Register(mux)

	req := withUser(httptest.NewRequest(http.MethodGet, "/resumes", nil), "u1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestResumeRest_RenameReturns404ForMissingResume(t *testing.T) {
	fake := &fakeService{renameErr: domainerrs.ErrNotFound.WithMessage("resume not found")}
	h := resumesvc.NewRestService(fake)
	router := resumesvc.NewRestRouter(h)
	mux := chi.NewRouter()
	router.Register(mux)

	body, _ := json.Marshal(map[string]string{"label": "New label"})
	req := withUser(httptest.NewRequest(http.MethodPatch, "/resumes/missing", bytes.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestResumeRest_UploadReturns400OnMalformedMultipartBody(t *testing.T) {
	fake := &fakeService{}
	h := resumesvc.NewRestService(fake)
	router := resumesvc.NewRestRouter(h)
	mux := chi.NewRouter()
	router.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/resumes", bytes.NewReader([]byte("not multipart")))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	req = withUser(req, "u1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestResumeRest_UploadReturns400WhenFileFieldMissing(t *testing.T) {
	fake := &fakeService{}
	h := resumesvc.NewRestService(fake)
	router := resumesvc.NewRestRouter(h)
	mux := chi.NewRouter()
	router.Register(mux)

	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	_ = w.WriteField("label", "no file here")
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/resumes", buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req = withUser(req, "u1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestResumeRest_ExtractKeywordsReturns200OnSuccess(t *testing.T) {
	fake := &fakeService{extractResult: &resumemodel.Resume{ID: "r1", Label: "My resume"}}
	h := resumesvc.NewRestService(fake)
	router := resumesvc.NewRestRouter(h)
	mux := chi.NewRouter()
	router.Register(mux)

	req := withUser(httptest.NewRequest(http.MethodPost, "/resumes/r1/keywords", nil), "u1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestResumeRest_ExtractKeywordsReturns404ForMissingResume(t *testing.T) {
	fake := &fakeService{extractErr: domainerrs.ErrNotFound.WithMessage("resume not found")}
	h := resumesvc.NewRestService(fake)
	router := resumesvc.NewRestRouter(h)
	mux := chi.NewRouter()
	router.Register(mux)

	req := withUser(httptest.NewRequest(http.MethodPost, "/resumes/missing/keywords", nil), "u1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestResumeRest_ExtractKeywordsReturns500OnExtractionFailure(t *testing.T) {
	fake := &fakeService{extractErr: domainerrs.ErrInternal.WithMessage("llm exhausted")}
	h := resumesvc.NewRestService(fake)
	router := resumesvc.NewRestRouter(h)
	mux := chi.NewRouter()
	router.Register(mux)

	req := withUser(httptest.NewRequest(http.MethodPost, "/resumes/r1/keywords", nil), "u1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestNewRestService_PanicsOnNilNext(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected NewRestService(nil) to panic")
		}
	}()
	resumesvc.NewRestService(nil)
}
