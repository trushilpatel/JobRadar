package meta

import (
	"testing"
)

func TestParseSitemap(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantIDs []string
	}{
		{
			name: "three urls two jobs one non-job filtered",
			input: `<?xml version="1.0"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://www.metacareers.com/profile/job_details/1234567890/</loc></url>
  <url><loc>https://www.metacareers.com/profile/job_details/9876543210/</loc></url>
  <url><loc>https://www.metacareers.com/about/careers</loc></url>
</urlset>`,
			wantIDs: []string{"1234567890", "9876543210"},
		},
		{
			name: "duplicate job IDs deduplicated",
			input: `<urlset>
  <url><loc>https://www.metacareers.com/profile/job_details/111/</loc></url>
  <url><loc>https://www.metacareers.com/profile/job_details/111/</loc></url>
  <url><loc>https://www.metacareers.com/profile/job_details/222/</loc></url>
</urlset>`,
			wantIDs: []string{"111", "222"},
		},
		{
			name: "mixed case loc tags",
			input: `<urlset>
  <URL><LOC>https://www.metacareers.com/profile/job_details/555/</LOC></URL>
</urlset>`,
			wantIDs: []string{"555"},
		},
		{
			name:    "no loc tags returns empty slice",
			input:   `<?xml version="1.0"?><urlset></urlset>`,
			wantIDs: []string{},
		},
		{
			name:    "non-job urls only returns empty slice",
			input:   `<urlset><url><loc>https://www.metacareers.com/about</loc></url></urlset>`,
			wantIDs: []string{},
		},
		{
			name:    "empty input returns empty slice",
			input:   ``,
			wantIDs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSitemap([]byte(tt.input))
			if len(got) != len(tt.wantIDs) {
				t.Fatalf("parseSitemap: got %d IDs %v, want %d %v",
					len(got), got, len(tt.wantIDs), tt.wantIDs)
			}
			for i, id := range tt.wantIDs {
				if got[i] != id {
					t.Errorf("id[%d] = %q, want %q", i, got[i], id)
				}
			}
		})
	}
}

func TestParseSitemap_Empty(t *testing.T) {
	ids := parseSitemap([]byte(""))
	if len(ids) != 0 {
		t.Errorf("expected empty slice for empty input, got %v", ids)
	}
}

func TestParseSitemap_OrderPreserved(t *testing.T) {
	input := `<urlset>
  <url><loc>https://www.metacareers.com/profile/job_details/300/</loc></url>
  <url><loc>https://www.metacareers.com/profile/job_details/100/</loc></url>
  <url><loc>https://www.metacareers.com/profile/job_details/200/</loc></url>
</urlset>`
	got := parseSitemap([]byte(input))
	want := []string{"300", "100", "200"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
