package apihuma

import (
	"encoding/json"
	"testing"

	"github.com/trushilpatel/jobradar/apps/api/response"
)

type testResource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func TestSingleOutputMatchesLegacyShape(t *testing.T) {
	out := SingleOutput[testResource]{}
	out.Body = response.Response[testResource]{Data: testResource{ID: "abc", Name: "Widget"}}

	got, err := json.Marshal(out.Body)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	want := `{"data":{"id":"abc","name":"Widget"}}`
	if string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestPagedOutputMatchesLegacyShape(t *testing.T) {
	out := PagedOutput[testResource]{}
	out.Body = response.PagedResponse[testResource]{
		Data:  []testResource{{ID: "abc", Name: "Widget"}},
		Total: 1,
		Page:  1,
		Limit: 20,
	}

	got, err := json.Marshal(out.Body)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	want := `{"data":[{"id":"abc","name":"Widget"}],"total":1,"page":1,"limit":20}`
	if string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestCursorPageOutputMatchesLegacyPublicListShape(t *testing.T) {
	out := CursorPageOutput[testResource]{}
	out.Body = CursorPage[testResource]{
		Data:       []testResource{{ID: "abc", Name: "Widget"}},
		Total:      1,
		Limit:      20,
		NextCursor: new("cursor123"),
	}

	got, err := json.Marshal(out.Body)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	want := `{"data":[{"id":"abc","name":"Widget"}],"total":1,"limit":20,"next_cursor":"cursor123"}`
	if string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestCursorPageOutputNextCursorNilRendersNull(t *testing.T) {
	out := CursorPageOutput[testResource]{}
	out.Body = CursorPage[testResource]{Data: []testResource{}, Total: 0, Limit: 20, NextCursor: nil}

	got, err := json.Marshal(out.Body)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	want := `{"data":[],"total":0,"limit":20,"next_cursor":null}`
	if string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestAdminCursorPageOutputMatchesLegacyAdminListShape(t *testing.T) {
	out := AdminCursorPageOutput[testResource]{}
	out.Body = AdminCursorPage[testResource]{
		Data:        []testResource{{ID: "abc", Name: "Widget"}},
		Total:       5,
		TotalActive: 3,
		Limit:       20,
		NextCursor:  nil,
	}

	got, err := json.Marshal(out.Body)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	want := `{"data":[{"id":"abc","name":"Widget"}],"total":5,"total_active":3,"limit":20,"next_cursor":null}`
	if string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}
