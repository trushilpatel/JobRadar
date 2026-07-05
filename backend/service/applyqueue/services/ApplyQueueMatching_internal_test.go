package services

import "testing"

func TestJaccardSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a, b []string
		want float64
	}{
		{"both empty", nil, nil, 0},
		{"a empty", nil, []string{"Go"}, 0},
		{"b empty", []string{"Go"}, nil, 0},
		{"identical sets", []string{"Go", "PostgreSQL"}, []string{"Go", "PostgreSQL"}, 1},
		{"disjoint sets", []string{"Go"}, []string{"React"}, 0},
		{"partial overlap", []string{"Go", "PostgreSQL", "Docker"}, []string{"Go", "PostgreSQL", "Redis"}, 0.5}, // 2/4
		{"duplicate entries within a set collapse via set semantics",
			[]string{"Go", "Go", "PostgreSQL"}, []string{"Go", "PostgreSQL"}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := jaccardSimilarity(tc.a, tc.b)
			if got != tc.want {
				t.Fatalf("jaccardSimilarity(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestBestMatchingResume(t *testing.T) {
	resumes := []parsedResumeKeywords{
		{ID: "r1", Label: "Weak", Keywords: []string{"Go", "PostgreSQL", "Python", "Redis"}},
		{ID: "r2", Label: "Strong", Keywords: []string{"Go", "PostgreSQL", "Kubernetes"}},
	}
	jobTags := []string{"Go", "PostgreSQL", "Kubernetes", "Docker"}

	id, label := bestMatchingResume(jobTags, resumes)
	if id == nil || *id != "r2" {
		t.Fatalf("expected r2 (higher Jaccard score), got %v", id)
	}
	if label == nil || *label != "Strong" {
		t.Fatalf("expected label 'Strong', got %v", label)
	}
}

func TestBestMatchingResume_EmptyJobTagsReturnsNil(t *testing.T) {
	resumes := []parsedResumeKeywords{{ID: "r1", Label: "L", Keywords: []string{"Go"}}}
	id, label := bestMatchingResume(nil, resumes)
	if id != nil || label != nil {
		t.Fatalf("expected nil, nil for empty job tags, got %v, %v", id, label)
	}
}

func TestBestMatchingResume_EmptyResumesReturnsNil(t *testing.T) {
	id, label := bestMatchingResume([]string{"Go"}, nil)
	if id != nil || label != nil {
		t.Fatalf("expected nil, nil for no resumes, got %v, %v", id, label)
	}
}

func TestBestMatchingResume_AllZeroScoresReturnsNil(t *testing.T) {
	resumes := []parsedResumeKeywords{
		{ID: "r1", Label: "L1", Keywords: []string{"React"}},
		{ID: "r2", Label: "L2", Keywords: []string{"TypeScript"}},
	}
	id, label := bestMatchingResume([]string{"Go", "PostgreSQL"}, resumes)
	if id != nil || label != nil {
		t.Fatalf("expected nil, nil when every resume scores 0, got %v, %v", id, label)
	}
}

func TestBestMatchingResume_TieKeepsFirstEncountered(t *testing.T) {
	resumes := []parsedResumeKeywords{
		{ID: "r1", Label: "First", Keywords: []string{"Go", "PostgreSQL"}},
		{ID: "r2", Label: "Second", Keywords: []string{"Go", "PostgreSQL"}},
	}
	id, label := bestMatchingResume([]string{"Go", "PostgreSQL"}, resumes)
	if id == nil || *id != "r1" {
		t.Fatalf("expected the first-encountered resume to win a tie, got %v", id)
	}
	if label == nil || *label != "First" {
		t.Fatalf("expected label 'First', got %v", label)
	}
}

func TestParseResumeKeywordRows_DropsEmptyAndMalformed(t *testing.T) {
	rows := []resumeKeywordRow{
		{ID: "r1", Label: "Good", Keywords: []byte(`["Go","PostgreSQL"]`)},
		{ID: "r2", Label: "Empty", Keywords: []byte(`[]`)},
		{ID: "r3", Label: "Malformed", Keywords: []byte(`not json`)},
		{ID: "r4", Label: "Nil", Keywords: nil},
	}
	got := parseResumeKeywordRows(rows)
	if len(got) != 1 || got[0].ID != "r1" {
		t.Fatalf("expected only r1 to survive parsing, got %+v", got)
	}
	if len(got[0].Keywords) != 2 || got[0].Keywords[0] != "Go" || got[0].Keywords[1] != "PostgreSQL" {
		t.Fatalf("unexpected parsed keywords: %v", got[0].Keywords)
	}
}
