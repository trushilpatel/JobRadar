package response_test

import (
	"encoding/json"
	"testing"

	"github.com/trushilpatel/jobradar/apps/api/response"
)

// ---------------------------------------------------------------------------
// Response[T] — single-resource envelope
// ---------------------------------------------------------------------------

func TestResponse_JSONRoundTrip_String(t *testing.T) {
	original := response.Response[string]{Data: "hello"}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got response.Response[string]
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Data != original.Data {
		t.Errorf("Data: want %q, got %q", original.Data, got.Data)
	}
}

func TestResponse_JSONRoundTrip_Struct(t *testing.T) {
	type inner struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	original := response.Response[inner]{Data: inner{ID: 42, Name: "acme"}}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got response.Response[inner]
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Data.ID != 42 || got.Data.Name != "acme" {
		t.Errorf("unexpected Data: %+v", got.Data)
	}
}

func TestResponse_JSONRoundTrip_ZeroValue(t *testing.T) {
	var original response.Response[int]
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got response.Response[int]
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Data != 0 {
		t.Errorf("want zero, got %d", got.Data)
	}
}

func TestResponse_JSONShape(t *testing.T) {
	// Verify the envelope key is "data"
	r := response.Response[int]{Data: 7}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	if _, ok := raw["data"]; !ok {
		t.Errorf("expected top-level key 'data', got keys %v", keysOf(raw))
	}
	if len(raw) != 1 {
		t.Errorf("expected exactly 1 top-level key, got %d: %v", len(raw), keysOf(raw))
	}
}

// ---------------------------------------------------------------------------
// PagedResponse[T]
// ---------------------------------------------------------------------------

func TestPagedResponse_JSONRoundTrip(t *testing.T) {
	original := response.PagedResponse[string]{
		Data:  []string{"a", "b", "c"},
		Total: 100,
		Page:  2,
		Limit: 3,
	}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got response.PagedResponse[string]
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Total != 100 || got.Page != 2 || got.Limit != 3 {
		t.Errorf("pagination fields wrong: %+v", got)
	}
	if len(got.Data) != 3 || got.Data[0] != "a" || got.Data[1] != "b" || got.Data[2] != "c" {
		t.Errorf("Data wrong: %v", got.Data)
	}
}

func TestPagedResponse_JSONShape(t *testing.T) {
	pr := response.PagedResponse[int]{Data: []int{1}, Total: 1, Page: 1, Limit: 10}
	b, err := json.Marshal(pr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	for _, key := range []string{"data", "total", "page", "limit"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("expected key %q in JSON", key)
		}
	}
}

func TestPagedResponse_EmptySlice(t *testing.T) {
	// Explicit empty slice — Data should serialise to [] not null
	original := response.PagedResponse[string]{Data: []string{}, Total: 0, Page: 1, Limit: 10}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// []string{} marshals to "[]"
	if string(raw["data"]) != "[]" {
		t.Errorf("expected Data to marshal to '[]', got %s", raw["data"])
	}
}

func TestPagedResponse_NilSlice(t *testing.T) {
	// Nil slice — Data marshals to null
	original := response.PagedResponse[string]{Total: 0, Page: 1, Limit: 10}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(raw["data"]) != "null" {
		t.Errorf("expected nil Data to marshal to 'null', got %s", raw["data"])
	}
}

func TestPagedResponse_ZeroValues(t *testing.T) {
	var original response.PagedResponse[int]
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got response.PagedResponse[int]
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Total != 0 || got.Page != 0 || got.Limit != 0 || got.Data != nil {
		t.Errorf("unexpected non-zero: %+v", got)
	}
}

func TestPagedResponse_LargeDataSet(t *testing.T) {
	data := make([]int, 1000)
	for i := range data {
		data[i] = i
	}
	original := response.PagedResponse[int]{Data: data, Total: 1000, Page: 1, Limit: 1000}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got response.PagedResponse[int]
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Data) != 1000 {
		t.Errorf("expected 1000 items, got %d", len(got.Data))
	}
	if got.Data[999] != 999 {
		t.Errorf("last element wrong: %d", got.Data[999])
	}
}

// ---------------------------------------------------------------------------
// ErrorDetail
// ---------------------------------------------------------------------------

func TestErrorDetail_JSONRoundTrip(t *testing.T) {
	original := response.ErrorDetail{Code: "NOT_FOUND", Message: "resource not found"}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got response.ErrorDetail
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Code != "NOT_FOUND" {
		t.Errorf("Code: want %q, got %q", "NOT_FOUND", got.Code)
	}
	if got.Message != "resource not found" {
		t.Errorf("Message: want %q, got %q", "resource not found", got.Message)
	}
}

func TestErrorDetail_ZeroValue(t *testing.T) {
	var original response.ErrorDetail
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got response.ErrorDetail
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Code != "" || got.Message != "" {
		t.Errorf("expected zero value, got %+v", got)
	}
}

func TestErrorDetail_JSONShape(t *testing.T) {
	d := response.ErrorDetail{Code: "E", Message: "m"}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"code", "message"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("expected key %q", key)
		}
	}
}

// ---------------------------------------------------------------------------
// ErrorResponse
// ---------------------------------------------------------------------------

func TestErrorResponse_JSONRoundTrip(t *testing.T) {
	original := response.ErrorResponse{
		Error: response.ErrorDetail{Code: "INTERNAL", Message: "something went wrong"},
	}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got response.ErrorResponse
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Error.Code != "INTERNAL" {
		t.Errorf("Code: want %q, got %q", "INTERNAL", got.Error.Code)
	}
	if got.Error.Message != "something went wrong" {
		t.Errorf("Message: want %q, got %q", "something went wrong", got.Error.Message)
	}
}

func TestErrorResponse_ZeroValue(t *testing.T) {
	var original response.ErrorResponse
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got response.ErrorResponse
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Error.Code != "" || got.Error.Message != "" {
		t.Errorf("expected zero value, got %+v", got)
	}
}

func TestErrorResponse_JSONShape(t *testing.T) {
	// Verify the envelope key is "error"
	er := response.ErrorResponse{Error: response.ErrorDetail{Code: "X", Message: "y"}}
	b, err := json.Marshal(er)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := raw["error"]; !ok {
		t.Errorf("expected top-level key 'error', got %v", keysOf(raw))
	}
	if len(raw) != 1 {
		t.Errorf("expected exactly 1 key, got %d: %v", len(raw), keysOf(raw))
	}
	// Nested keys
	var nested map[string]json.RawMessage
	if err := json.Unmarshal(raw["error"], &nested); err != nil {
		t.Fatalf("unmarshal nested: %v", err)
	}
	for _, key := range []string{"code", "message"} {
		if _, ok := nested[key]; !ok {
			t.Errorf("expected nested key %q", key)
		}
	}
}

// ---------------------------------------------------------------------------
// ValidationFieldError
// ---------------------------------------------------------------------------

func TestValidationFieldError_JSONRoundTrip(t *testing.T) {
	original := response.ValidationFieldError{
		Field:   "email",
		Code:    "INVALID_FORMAT",
		Message: "email is not valid",
	}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got response.ValidationFieldError
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Field != "email" {
		t.Errorf("Field: want %q, got %q", "email", got.Field)
	}
	if got.Code != "INVALID_FORMAT" {
		t.Errorf("Code: want %q, got %q", "INVALID_FORMAT", got.Code)
	}
	if got.Message != "email is not valid" {
		t.Errorf("Message: want %q, got %q", "email is not valid", got.Message)
	}
}

func TestValidationFieldError_ZeroValue(t *testing.T) {
	var original response.ValidationFieldError
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got response.ValidationFieldError
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Field != "" || got.Code != "" || got.Message != "" {
		t.Errorf("expected zero value, got %+v", got)
	}
}

func TestValidationFieldError_JSONShape(t *testing.T) {
	vfe := response.ValidationFieldError{Field: "f", Code: "c", Message: "m"}
	b, err := json.Marshal(vfe)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"field", "code", "message"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("expected key %q", key)
		}
	}
	if len(raw) != 3 {
		t.Errorf("expected 3 keys, got %d: %v", len(raw), keysOf(raw))
	}
}

// ---------------------------------------------------------------------------
// ValidationErrorResponse
// ---------------------------------------------------------------------------

func TestValidationErrorResponse_JSONRoundTrip(t *testing.T) {
	original := response.ValidationErrorResponse{
		Errors: []response.ValidationFieldError{
			{Field: "name", Code: "REQUIRED", Message: "name is required"},
			{Field: "age", Code: "OUT_OF_RANGE", Message: "age must be positive"},
		},
	}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got response.ValidationErrorResponse
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Errors) != 2 {
		t.Fatalf("want 2 errors, got %d", len(got.Errors))
	}
	if got.Errors[0].Field != "name" || got.Errors[0].Code != "REQUIRED" {
		t.Errorf("first error wrong: %+v", got.Errors[0])
	}
	if got.Errors[1].Field != "age" || got.Errors[1].Code != "OUT_OF_RANGE" {
		t.Errorf("second error wrong: %+v", got.Errors[1])
	}
}

func TestValidationErrorResponse_SingleError(t *testing.T) {
	original := response.ValidationErrorResponse{
		Errors: []response.ValidationFieldError{
			{Field: "email", Code: "INVALID", Message: "bad email"},
		},
	}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got response.ValidationErrorResponse
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Errors) != 1 {
		t.Fatalf("want 1 error, got %d", len(got.Errors))
	}
	if got.Errors[0].Field != "email" {
		t.Errorf("Field: want 'email', got %q", got.Errors[0].Field)
	}
}

func TestValidationErrorResponse_EmptySlice(t *testing.T) {
	// Explicit empty errors slice — should serialise to []
	original := response.ValidationErrorResponse{Errors: []response.ValidationFieldError{}}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(raw["errors"]) != "[]" {
		t.Errorf("expected 'errors' to marshal to '[]', got %s", raw["errors"])
	}
}

func TestValidationErrorResponse_NilSlice(t *testing.T) {
	// Nil Errors slice — marshals to null
	var original response.ValidationErrorResponse
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(raw["errors"]) != "null" {
		t.Errorf("expected 'errors' to marshal to 'null', got %s", raw["errors"])
	}
}

func TestValidationErrorResponse_JSONShape(t *testing.T) {
	ver := response.ValidationErrorResponse{
		Errors: []response.ValidationFieldError{{Field: "f", Code: "c", Message: "m"}},
	}
	b, err := json.Marshal(ver)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := raw["errors"]; !ok {
		t.Errorf("expected top-level key 'errors', got %v", keysOf(raw))
	}
	if len(raw) != 1 {
		t.Errorf("expected exactly 1 key, got %d: %v", len(raw), keysOf(raw))
	}
}

func TestValidationErrorResponse_PreservesFieldOrder(t *testing.T) {
	original := response.ValidationErrorResponse{
		Errors: []response.ValidationFieldError{
			{Field: "z", Code: "Z", Message: "last"},
			{Field: "a", Code: "A", Message: "first"},
		},
	}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got response.ValidationErrorResponse
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Errors[0].Field != "z" || got.Errors[1].Field != "a" {
		t.Errorf("order not preserved: %+v", got.Errors)
	}
}

// ---------------------------------------------------------------------------
// Response[T] with pointer type parameter
// ---------------------------------------------------------------------------

func TestResponse_PointerType_Nil(t *testing.T) {
	// T is a pointer type; Data is nil
	var original response.Response[*string]
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got response.Response[*string]
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Data != nil {
		t.Errorf("expected nil Data pointer, got %v", got.Data)
	}
}

func TestResponse_PointerType_Set(t *testing.T) {
	s := "value"
	original := response.Response[*string]{Data: &s}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got response.Response[*string]
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Data == nil {
		t.Fatal("expected non-nil Data pointer")
	}
	if *got.Data != "value" {
		t.Errorf("want %q, got %q", "value", *got.Data)
	}
}

// ---------------------------------------------------------------------------
// PagedResponse[T] with struct elements
// ---------------------------------------------------------------------------

func TestPagedResponse_StructElements(t *testing.T) {
	type item struct {
		ID    int    `json:"id"`
		Label string `json:"label"`
	}
	original := response.PagedResponse[item]{
		Data:  []item{{ID: 1, Label: "one"}, {ID: 2, Label: "two"}},
		Total: 2,
		Page:  1,
		Limit: 10,
	}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got response.PagedResponse[item]
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Data) != 2 {
		t.Fatalf("want 2 items, got %d", len(got.Data))
	}
	if got.Data[0].ID != 1 || got.Data[0].Label != "one" {
		t.Errorf("first item wrong: %+v", got.Data[0])
	}
	if got.Data[1].ID != 2 || got.Data[1].Label != "two" {
		t.Errorf("second item wrong: %+v", got.Data[1])
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func keysOf(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
