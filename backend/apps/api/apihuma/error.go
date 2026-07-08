package apihuma

import (
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
)

// pkgLogger mirrors apps/api/handler's SetLogger convention: install once at
// boot; defaults to noop so test binaries that never call SetLogger stay
// silent.
var pkgLogger logger.Logger = logger.NewNoop()

// SetLogger installs the logger MapError uses when logging a 5xx
// ServiceError or an unmapped error. Call once during process init from
// main.go before any request is served.
func SetLogger(l logger.Logger) {
	if l == nil {
		pkgLogger = logger.NewNoop()
		return
	}
	pkgLogger = l
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// serviceError is a huma.StatusError for the legacy
// {"error":{"code","message"}} envelope.
type serviceError struct {
	status    int
	ErrorBody errorDetail `json:"error"`
}

func (e *serviceError) Error() string  { return e.ErrorBody.Message }
func (e *serviceError) GetStatus() int { return e.status }

type validationFieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// validationErrorBody is a huma.StatusError for the legacy
// {"errors":[{"field","code","message"}]} envelope — always 400, matching
// handler.RespondValidationErrors.
type validationErrorBody struct {
	ErrorsBody []validationFieldError `json:"errors"`
}

func (e *validationErrorBody) Error() string {
	if len(e.ErrorsBody) == 0 {
		return "validation error"
	}
	return e.ErrorsBody[0].Message
}
func (e *validationErrorBody) GetStatus() int { return http.StatusBadRequest }

// MapError translates a domain error into a huma.StatusError whose JSON body
// matches the pre-Huma legacy envelope exactly, so the frontend needs zero
// changes during the migration. Mirrors apps/api/handler.HandleError's
// dispatch order precisely: apperr.ValidationErrors -> *apperr.ValidationError
// (normalized to a one-element list) -> *apperr.ServiceError -> unmapped 500.
func MapError(err error) error {
	var vErrs apperr.ValidationErrors
	if errors.As(err, &vErrs) {
		return newValidationError(vErrs)
	}
	var vErr *apperr.ValidationError
	if errors.As(err, &vErr) {
		return newValidationError(apperr.ValidationErrors{*vErr})
	}
	var sErr *apperr.ServiceError
	if errors.As(err, &sErr) {
		// 5xx ServiceErrors carry a deliberately generic PublicMessage() — the
		// InternalMessage set via .WithMessage() is the only record of what
		// actually failed, so it must be logged here or it's gone forever.
		// 4xx (NOT_FOUND, CONFLICT, BAD_REQUEST, ...) are routine and already
		// visible in the access log; logging those at Error level would just
		// be noise. Mirrors handler.HandleError's identical rationale.
		if sErr.Status >= http.StatusInternalServerError {
			pkgLogger.Error("apihuma: service error returned 5xx", sErr)
		}
		return &serviceError{status: sErr.Status, ErrorBody: errorDetail{Code: sErr.Code, Message: sErr.PublicMessage()}}
	}
	pkgLogger.Error("apihuma: unmapped error returned 500", err)
	return &serviceError{status: http.StatusInternalServerError, ErrorBody: errorDetail{Code: "INTERNAL", Message: "internal server error"}}
}

func newValidationError(errs apperr.ValidationErrors) *validationErrorBody {
	fields := make([]validationFieldError, len(errs))
	for i, e := range errs {
		fields[i] = validationFieldError{Field: e.Field, Code: e.Code, Message: e.Message}
	}
	return &validationErrorBody{ErrorsBody: fields}
}

// codeForStatus derives a machine-readable code from an HTTP status for
// errors huma generates itself (no domain ServiceError.Code exists for
// these), e.g. "NOT_FOUND" for 404, "BAD_REQUEST" for 400.
func codeForStatus(status int) string {
	text := http.StatusText(status)
	if text == "" {
		return "ERROR"
	}
	return strings.ToUpper(strings.ReplaceAll(text, " ", "_"))
}

// InstallLegacyErrorModel overrides huma's global default error model (RFC
// 9457 problem+json) so errors huma generates itself — a request body that
// fails schema validation before any handler runs, an unroutable request,
// method-not-allowed — also come out in the legacy envelope shape instead of
// Huma's default. Call once during process init from main.go, before serving
// any request. Domain errors from RestService handlers should return
// MapError(err) directly rather than relying on this override; this only
// covers errors the framework itself raises.
func InstallLegacyErrorModel() {
	huma.NewError = func(status int, msg string, errs ...error) huma.StatusError {
		if len(errs) == 1 && isBodySyntaxError(errs[0]) {
			// A malformed-JSON-syntax body has no specific field to blame —
			// Huma reports it with the bare "body" location. The pre-Huma
			// handler.DecodeBody always reported exactly this failure mode as
			// a single BAD_REQUEST error, never the field-list validation
			// shape (reserved for genuine per-field failures like
			// "body.active"). Preserve that distinction.
			return &serviceError{status: http.StatusBadRequest, ErrorBody: errorDetail{Code: "BAD_REQUEST", Message: "invalid request body"}}
		}
		if len(errs) > 0 {
			fields := make([]validationFieldError, 0, len(errs))
			for _, e := range errs {
				if detailer, ok := e.(huma.ErrorDetailer); ok {
					d := detailer.ErrorDetail()
					fields = append(fields, validationFieldError{Field: d.Location, Code: "VALIDATION", Message: d.Message})
					continue
				}
				fields = append(fields, validationFieldError{Code: "VALIDATION", Message: e.Error()})
			}
			return &validationErrorBody{ErrorsBody: fields}
		}
		return &serviceError{status: status, ErrorBody: errorDetail{Code: codeForStatus(status), Message: msg}}
	}
}

// isBodySyntaxError reports whether e is a Huma error detail whose location
// is the bare "body" placeholder — i.e. a whole-body parse failure (invalid
// JSON syntax) rather than a specific field's validation failure (which
// would carry a location like "body.active").
func isBodySyntaxError(e error) bool {
	detailer, ok := e.(huma.ErrorDetailer)
	if !ok {
		return false
	}
	return detailer.ErrorDetail().Location == "body"
}
