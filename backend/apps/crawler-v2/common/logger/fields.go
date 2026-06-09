package logger

import "time"

// Field is a typed key-value pair attached to a log event.
// Use the constructor functions below — never construct Field directly.
type Field struct {
	Key   string
	Value any
}

// Standardized field key constants.
// Always use these — never raw strings.
// Consistent keys are required for Loki queries to work across services.
const (
	FieldService    = "service"
	FieldAdapter    = "adapter"
	FieldCompany    = "company_id"
	FieldJobID      = "job_id"
	FieldTitle      = "title"
	FieldURL        = "url"
	FieldLocation   = "location"
	FieldDuration   = "duration_ms"
	FieldJobCount   = "job_count"
	FieldStatusCode = "status_code"
	FieldTraceID    = "trace_id"
	FieldSpanID     = "span_id"
	FieldError      = "error"
	FieldPage       = "page"
	FieldTotalPages = "total_pages"
	FieldTotal      = "total"
	FieldDepartment = "department"
	FieldOffset     = "offset"
)

func Str(key, val string) Field             { return Field{Key: key, Value: val} }
func Int(key string, val int) Field         { return Field{Key: key, Value: val} }
func Dur(key string, d time.Duration) Field { return Field{Key: key, Value: d} }
func Bool(key string, val bool) Field       { return Field{Key: key, Value: val} }
func Err(err error) Field                   { return Field{Key: FieldError, Value: err} }
