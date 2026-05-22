package types

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestStripHTML_RemovesTagsAndCollapses verifies StripHTML removes HTML tags,
// decodes entities, and collapses whitespace.
func TestStripHTML_RemovesTagsAndCollapses(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"<p>Hello <b>World</b></p>", "Hello World"},
		{"<br/>line1<br/>line2", "line1 line2"},
		{"&lt;em&gt;text&lt;/em&gt;", "<em>text</em>"},
		{"  extra   spaces  ", "extra spaces"},
		{"", ""},
	}
	for _, tc := range cases {
		got := StripHTML(tc.in)
		if got != tc.want {
			t.Errorf("StripHTML(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestRawJob_FullContentOmitEmpty verifies that a RawJob with nil FullContent
// does NOT include "full_content" in its JSON serialization.
// This is critical for backward compatibility: existing Kafka consumers that
// do not know about FullContent must not see a new field in the message.
func TestRawJob_FullContentOmitEmpty(t *testing.T) {
	job := RawJob{
		ID:          "greenhouse-1",
		CompanyID:   "company-abc",
		Adapter:     "greenhouse",
		Title:       "Software Engineer",
		Location:    "San Francisco, CA",
		URL:         "https://example.com/jobs/1",
		EmployType:  "full_time",
		FullContent: nil,
	}

	data, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}

	jsonStr := string(data)
	if strings.Contains(jsonStr, "full_content") {
		t.Errorf("JSON should not contain 'full_content' when FullContent is nil, got: %s", jsonStr)
	}
}

// TestRawJob_FullContentRoundTrip verifies that a RawJob with a fully-populated
// FullContent marshals to JSON containing "full_content" and round-trips back
// to an identical value.
func TestRawJob_FullContentRoundTrip(t *testing.T) {
	original := RawJob{
		ID:         "lever-42",
		CompanyID:  "company-xyz",
		Adapter:    "lever",
		Title:      "Staff Engineer",
		Location:   "Remote",
		URL:        "https://jobs.lever.co/co/42",
		EmployType: "full_time",
		FullContent: &FullJobContent{
			Description:   "Build distributed systems.",
			Requirements:  "5+ years Go experience.",
			Compensation:  "$150,000 - $200,000",
			WorkplaceType: "remote",
			Department:    "Engineering",
			PublishedDate: "2026-04-01",
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, "full_content") {
		t.Errorf("JSON should contain 'full_content' when FullContent is set, got: %s", jsonStr)
	}

	var roundTripped RawJob
	if err := json.Unmarshal(data, &roundTripped); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}

	// Assert base fields
	if roundTripped.ID != original.ID {
		t.Errorf("ID: want %q got %q", original.ID, roundTripped.ID)
	}
	if roundTripped.CompanyID != original.CompanyID {
		t.Errorf("CompanyID: want %q got %q", original.CompanyID, roundTripped.CompanyID)
	}
	if roundTripped.Adapter != original.Adapter {
		t.Errorf("Adapter: want %q got %q", original.Adapter, roundTripped.Adapter)
	}
	if roundTripped.Title != original.Title {
		t.Errorf("Title: want %q got %q", original.Title, roundTripped.Title)
	}
	if roundTripped.Location != original.Location {
		t.Errorf("Location: want %q got %q", original.Location, roundTripped.Location)
	}
	if roundTripped.URL != original.URL {
		t.Errorf("URL: want %q got %q", original.URL, roundTripped.URL)
	}
	if roundTripped.EmployType != original.EmployType {
		t.Errorf("EmployType: want %q got %q", original.EmployType, roundTripped.EmployType)
	}

	// Assert FullContent fields
	if roundTripped.FullContent == nil {
		t.Fatal("FullContent should not be nil after round-trip")
	}
	fc := roundTripped.FullContent
	orig := original.FullContent
	if fc.Description != orig.Description {
		t.Errorf("FullContent.Description: want %q got %q", orig.Description, fc.Description)
	}
	if fc.Requirements != orig.Requirements {
		t.Errorf("FullContent.Requirements: want %q got %q", orig.Requirements, fc.Requirements)
	}
	if fc.Compensation != orig.Compensation {
		t.Errorf("FullContent.Compensation: want %q got %q", orig.Compensation, fc.Compensation)
	}
	if fc.WorkplaceType != orig.WorkplaceType {
		t.Errorf("FullContent.WorkplaceType: want %q got %q", orig.WorkplaceType, fc.WorkplaceType)
	}
	if fc.Department != orig.Department {
		t.Errorf("FullContent.Department: want %q got %q", orig.Department, fc.Department)
	}
	if fc.PublishedDate != orig.PublishedDate {
		t.Errorf("FullContent.PublishedDate: want %q got %q", orig.PublishedDate, fc.PublishedDate)
	}
}
