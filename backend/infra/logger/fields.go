package logger

import "time"

// Field is a typed key-value pair attached to a log event.
// Use the constructor functions below — never construct Field directly.
type Field struct {
	Key   string
	Value any
}

// Standardized field key constants. Always use these — never raw strings.
// Consistent keys are required for Loki queries to work across services.
const (
	FieldService  = "service"
	FieldCompany  = "company_id"
	FieldJobID    = "job_id"
	FieldDuration = "duration"
	FieldFetched  = "fetched"
	FieldNew      = "new"
	FieldError    = "error"
	FieldBatch    = "batch"
	FieldRelevant = "relevant"
	FieldMethod   = "method"
	FieldPath     = "path"
	FieldStatus   = "status"
	FieldQueueID  = "queue_id"
)

// Str creates a string field.
func Str(key, val string) Field { return Field{Key: key, Value: val} }

// Int creates an integer field.
func Int(key string, val int) Field { return Field{Key: key, Value: val} }

// Dur creates a duration field.
func Dur(key string, d time.Duration) Field { return Field{Key: key, Value: d} }

// Bool creates a boolean field.
func Bool(key string, val bool) Field { return Field{Key: key, Value: val} }

// Err creates an error field using FieldError as the key.
// Use this to attach an error at a non-Error log level (e.g. Warn).
// At Error level, pass err directly to Logger.Error() instead.
func Err(err error) Field { return Field{Key: FieldError, Value: err} }
