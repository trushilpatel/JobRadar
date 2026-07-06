package apihuma

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
)

// spyLogger mirrors apps/api/handler's spyLogger test double so MapError's
// 5xx-only logging behavior can be asserted the same way HandleError's is.
type spyLogger struct {
	mu     sync.Mutex
	errors []error
}

func (s *spyLogger) Debug(string, ...logger.Field) {}
func (s *spyLogger) Info(string, ...logger.Field)  {}
func (s *spyLogger) Warn(string, ...logger.Field)  {}
func (s *spyLogger) Error(_ string, err error, _ ...logger.Field) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errors = append(s.errors, err)
}
func (s *spyLogger) With(...logger.Field) logger.Logger        { return s }
func (s *spyLogger) WithContext(context.Context) logger.Logger { return s }

func (s *spyLogger) errorCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.errors)
}

func TestMapError_ServiceError_MatchesLegacyEnvelope(t *testing.T) {
	err := MapError(&apperr.ServiceError{Code: "NOT_FOUND", Status: http.StatusNotFound, ExternalMessage: "job not found"})

	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected a huma.StatusError, got %T", err)
	}
	if statusErr.GetStatus() != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", statusErr.GetStatus())
	}

	got, marshalErr := json.Marshal(statusErr)
	if marshalErr != nil {
		t.Fatalf("marshal failed: %v", marshalErr)
	}
	want := `{"error":{"code":"NOT_FOUND","message":"job not found"}}`
	if string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestMapError_ServiceError_5xx_Logs(t *testing.T) {
	original := pkgLogger
	defer func() { pkgLogger = original }()
	spy := &spyLogger{}
	SetLogger(spy)

	_ = MapError(&apperr.ServiceError{Code: "INTERNAL", Status: http.StatusInternalServerError, InternalMessage: "db exploded"})

	if spy.errorCount() != 1 {
		t.Fatalf("expected 5xx ServiceError to log once, got %d", spy.errorCount())
	}
}

func TestMapError_ServiceError_4xx_DoesNotLog(t *testing.T) {
	original := pkgLogger
	defer func() { pkgLogger = original }()
	spy := &spyLogger{}
	SetLogger(spy)

	_ = MapError(&apperr.ServiceError{Code: "CONFLICT", Status: http.StatusConflict, ExternalMessage: "already exists"})

	if spy.errorCount() != 0 {
		t.Fatalf("expected routine 4xx ServiceError not to log, got %d entries", spy.errorCount())
	}
}

func TestMapError_ValidationErrors_MatchesLegacyEnvelope(t *testing.T) {
	err := MapError(apperr.ValidationErrors{
		{Field: "email", Code: "REQUIRED", Message: "email is required"},
		{Field: "age", Code: "OUT_OF_RANGE", Message: "age must be positive"},
	})

	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected a huma.StatusError, got %T", err)
	}
	if statusErr.GetStatus() != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", statusErr.GetStatus())
	}

	got, marshalErr := json.Marshal(statusErr)
	if marshalErr != nil {
		t.Fatalf("marshal failed: %v", marshalErr)
	}
	want := `{"errors":[{"field":"email","code":"REQUIRED","message":"email is required"},{"field":"age","code":"OUT_OF_RANGE","message":"age must be positive"}]}`
	if string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestMapError_SingleValidationError_NormalizedToOneElementList(t *testing.T) {
	err := MapError(&apperr.ValidationError{Field: "email", Code: "REQUIRED", Message: "email is required"})

	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected a huma.StatusError, got %T", err)
	}
	if statusErr.GetStatus() != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", statusErr.GetStatus())
	}
	got, marshalErr := json.Marshal(statusErr)
	if marshalErr != nil {
		t.Fatalf("marshal failed: %v", marshalErr)
	}
	want := `{"errors":[{"field":"email","code":"REQUIRED","message":"email is required"}]}`
	if string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestMapError_Unmapped_Returns500AndLogs(t *testing.T) {
	original := pkgLogger
	defer func() { pkgLogger = original }()
	spy := &spyLogger{}
	SetLogger(spy)

	err := MapError(errors.New("boom"))

	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected a huma.StatusError, got %T", err)
	}
	if statusErr.GetStatus() != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", statusErr.GetStatus())
	}
	got, marshalErr := json.Marshal(statusErr)
	if marshalErr != nil {
		t.Fatalf("marshal failed: %v", marshalErr)
	}
	want := `{"error":{"code":"INTERNAL","message":"internal server error"}}`
	if string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	if spy.errorCount() != 1 {
		t.Fatalf("expected unmapped error to log once, got %d", spy.errorCount())
	}
}

func TestSetLogger_Nil_InstallsNoop(t *testing.T) {
	original := pkgLogger
	defer func() { pkgLogger = original }()
	SetLogger(nil)
	if pkgLogger == nil {
		t.Fatal("SetLogger(nil) must install a noop, not leave pkgLogger nil")
	}
}

// TestInstallLegacyErrorModel_MalformedBodySyntax_UsesSingleErrorShape
// guards a real finding from Slice 3: a malformed-JSON-syntax body failure
// (caught by Huma itself, before any handler runs) carries a single
// huma.ErrorDetail whose Location is the bare "body" placeholder — Huma has
// no specific field to point to. The pre-Huma handler (handler.DecodeBody)
// always reported this exact failure mode as a single
// {"error":{"code":"BAD_REQUEST","message":"invalid request body"}}, never
// the field-list validation shape — that shape is reserved for genuine
// per-field validation failures (Location like "body.active"). Losing this
// distinction would be a real contract break for every migrated slice with
// a JSON body.
func TestInstallLegacyErrorModel_MalformedBodySyntax_UsesSingleErrorShape(t *testing.T) {
	original := huma.NewError
	defer func() { huma.NewError = original }()
	InstallLegacyErrorModel()

	err := huma.NewError(http.StatusUnprocessableEntity, "validation failed", &huma.ErrorDetail{
		Message:  "invalid character 'n' looking for beginning of object key string",
		Location: "body",
	})

	if err.GetStatus() != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", err.GetStatus())
	}
	got, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatalf("marshal failed: %v", marshalErr)
	}
	want := `{"error":{"code":"BAD_REQUEST","message":"invalid request body"}}`
	if string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestInstallLegacyErrorModel_ValidationErrorsUseLegacyShape(t *testing.T) {
	original := huma.NewError
	defer func() { huma.NewError = original }()
	InstallLegacyErrorModel()

	err := huma.NewError(http.StatusUnprocessableEntity, "validation failed", &huma.ErrorDetail{
		Message:  "expected boolean",
		Location: "body.active",
	})

	if err.GetStatus() != http.StatusBadRequest {
		t.Fatalf("expected legacy model to normalize validation status to 400, got %d", err.GetStatus())
	}
	got, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatalf("marshal failed: %v", marshalErr)
	}
	want := `{"errors":[{"field":"body.active","code":"VALIDATION","message":"expected boolean"}]}`
	if string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestServiceError_ErrorReturnsMessage(t *testing.T) {
	e := &serviceError{status: http.StatusNotFound, ErrorBody: errorDetail{Code: "NOT_FOUND", Message: "job not found"}}
	if e.Error() != "job not found" {
		t.Fatalf("got %q, want %q", e.Error(), "job not found")
	}
}

func TestValidationErrorBody_ErrorReturnsFirstMessage(t *testing.T) {
	e := &validationErrorBody{ErrorsBody: []validationFieldError{
		{Field: "email", Code: "REQUIRED", Message: "email is required"},
	}}
	if e.Error() != "email is required" {
		t.Fatalf("got %q, want %q", e.Error(), "email is required")
	}
}

func TestValidationErrorBody_ErrorEmptyListReturnsGenericMessage(t *testing.T) {
	e := &validationErrorBody{}
	if e.Error() != "validation error" {
		t.Fatalf("got %q, want %q", e.Error(), "validation error")
	}
}

func TestCodeForStatus_UnknownStatusReturnsGenericError(t *testing.T) {
	if got := codeForStatus(999); got != "ERROR" {
		t.Fatalf("got %q, want %q", got, "ERROR")
	}
}

func TestCodeForStatus_KnownStatusDerivesCode(t *testing.T) {
	if got := codeForStatus(http.StatusNotFound); got != "NOT_FOUND" {
		t.Fatalf("got %q, want %q", got, "NOT_FOUND")
	}
}

func TestInstallLegacyErrorModel_ValidationErrorsWithPlainNonDetailerError(t *testing.T) {
	original := huma.NewError
	defer func() { huma.NewError = original }()
	InstallLegacyErrorModel()

	err := huma.NewError(http.StatusUnprocessableEntity, "validation failed", errors.New("plain failure"))

	got, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatalf("marshal failed: %v", marshalErr)
	}
	want := `{"errors":[{"field":"","code":"VALIDATION","message":"plain failure"}]}`
	if string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestInstallLegacyErrorModel_PlainErrorUsesLegacyShape(t *testing.T) {
	original := huma.NewError
	defer func() { huma.NewError = original }()
	InstallLegacyErrorModel()

	err := huma.NewError(http.StatusNotFound, "no matching route")

	if err.GetStatus() != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", err.GetStatus())
	}
	got, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatalf("marshal failed: %v", marshalErr)
	}
	want := `{"error":{"code":"NOT_FOUND","message":"no matching route"}}`
	if string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}
