package tests

import (
	"bytes"
	"encoding/json"
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

func TestResumeRest_DownloadReturnsSignedURL(t *testing.T) {
	fake := &fakeService{presignResult: "https://example.test/signed"}
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
	var envelope struct {
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if envelope.Data.URL != "https://example.test/signed" {
		t.Fatalf("expected the fake's presigned URL, got %s", envelope.Data.URL)
	}
}

func TestResumeRest_DownloadReturns404ForMissingResume(t *testing.T) {
	fake := &fakeService{presignErr: domainerrs.ErrNotFound.WithMessage("resume not found")}
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
