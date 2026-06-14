// Package deel implements the ATS adapter for jobs.deel.com — Deel's hosted
// careers-page product (Klarna, Dott, and other HR customers).
//
// Deel renders every job board with Next.js Server Components. The public API
// on api-prod.letsdeel.com is auth-gated, but the SSR page can be requested
// with the `RSC: 1` header to return the React Server Components stream
// directly — no JS execution required. The stream contains JSON job records
// embedded in numbered "T" (text) chunks; we extract them with balanced-brace
// parsing and resolve the `$<hex>` description references against the chunk map.
//
// FetchJobListing issues a single HTTP request and sets JobContent on every
// job, so no PostFetcher is needed (fat adapter).
package deel

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	deelapi "github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/deel/api"
	"github.com/trushilpatel/jobradar/common/ats/core"
	"github.com/trushilpatel/jobradar/common/logger"

	"encoding/json"
)

const (
	defaultBaseURL = "https://jobs.deel.com"

	// rscHeader tells Next.js to return the raw React Server Components stream
	// instead of the wrapping HTML document.
	rscHeader = "RSC"

	// userAgent — Cloudfront in front of jobs.deel.com sometimes 403s the
	// default Go user agent; a real browser UA is safer.
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

// Deel implements core.ListingFetcher against jobs.deel.com.
type Deel struct {
	// baseURL overrides the host — used in tests to point at httptest.
	baseURL    string
	log        logger.Logger
	httpClient *http.Client
}

// New constructs a Deel adapter with production defaults.
func New(log logger.Logger, httpClient *http.Client) *Deel {
	return &Deel{
		log:        log.With(logger.Str(logger.FieldAdapter, "deel")),
		httpClient: httpClient,
	}
}

// ── ListingFetcher ────────────────────────────────────────────────────────────

// FetchJobListing fetches and parses the RSC stream for the given Deel vanity
// slug, returning every open posting with JobContent fully populated.
func (d *Deel) FetchJobListing(ctx context.Context, companyID string) (core.JobListing, error) {
	log := d.log.WithContext(ctx).With(logger.Str(logger.FieldCompany, companyID))
	start := time.Now()

	url := d.resolveBaseURL() + "/" + companyID
	log.Debug("fetching RSC stream", logger.Str(logger.FieldURL, url))

	body, err := d.fetchRSC(ctx, companyID)
	if err != nil {
		log.Error("RSC fetch failed", err, logger.Str(logger.FieldURL, url))
		return core.JobListing{}, fmt.Errorf("deel: %w", err)
	}

	chunks := parseChunks(body)
	records := extractJobRecords(body)

	jobs := make([]core.JobSummary, 0, len(records))
	for _, rec := range records {
		content := buildJobContent(rec, chunks)
		jobs = append(jobs, mapJobSummary(rec, companyID, content))
	}

	log.Info("listing fetched",
		logger.Int(logger.FieldJobCount, len(jobs)),
		logger.Dur(logger.FieldDurationMs, time.Since(start)),
	)
	return core.JobListing{Jobs: jobs}, nil
}

// ── HTTP ──────────────────────────────────────────────────────────────────────

// fetchRSC issues a GET with the RSC header so Next.js returns the raw Server
// Components stream instead of the wrapping HTML document.
func (d *Deel) fetchRSC(ctx context.Context, slug string) (string, error) {
	url := d.resolveBaseURL() + "/" + slug
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set(rscHeader, "1")
	req.Header.Set("Accept", "text/x-component")
	req.Header.Set("User-Agent", userAgent)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	return string(b), nil
}

// ── RSC stream parsing ────────────────────────────────────────────────────────

// chunkHeader matches the RSC text-chunk header `<hexId>:T<lenHex>,` that
// prefixes every streamed text payload. We scan by header position rather than
// by `\n` because T-chunks can be concatenated inside one physical line.
var chunkHeader = regexp.MustCompile(`([0-9a-f]+):T([0-9a-f]+),`)

// parseChunks walks the RSC stream and returns a map from chunk ID (lowercase
// hex) to the chunk's text payload.
func parseChunks(body string) map[string]string {
	out := make(map[string]string)
	headers := chunkHeader.FindAllStringSubmatchIndex(body, -1)
	for _, h := range headers {
		// h: [0]=match start, [1]=match end,
		//    [2..3]=hexId, [4..5]=lenHex
		id := body[h[2]:h[3]]
		lenStr := body[h[4]:h[5]]
		var n int
		if _, err := fmt.Sscanf(lenStr, "%x", &n); err != nil || n < 0 {
			continue
		}
		start := h[1]
		end := start + n
		if end > len(body) {
			end = len(body)
		}
		out[id] = body[start:end]
	}
	return out
}

// jobRecordStart matches the prefix of each job posting record in the stream.
// `id` is always followed by `jobId` — both UUIDs — and that pair is unique to
// JobPosting records (career-page-settings records use `id` + `organizationId`
// instead, so the prefix discriminates cleanly).
var jobRecordStart = regexp.MustCompile(`\{"id":"[a-f0-9-]{36}","jobId":"[a-f0-9-]{36}",`)

// extractJobRecords scans the entire RSC body for job-posting objects and
// returns each one decoded. Records are concatenated inside text chunks, so we
// locate each start position via regex then balance braces to find the end.
func extractJobRecords(body string) []deelapi.RawJobPosting {
	starts := jobRecordStart.FindAllStringIndex(body, -1)
	out := make([]deelapi.RawJobPosting, 0, len(starts))
	for _, s := range starts {
		end := matchBalanced(body, s[0])
		if end < 0 {
			continue
		}
		var rec deelapi.RawJobPosting
		if err := json.Unmarshal([]byte(body[s[0]:end]), &rec); err != nil {
			continue
		}
		out = append(out, rec)
	}
	return out
}

// matchBalanced returns the index past the closing `}` that pairs with the
// opening `{` at start, or -1 if the braces are unbalanced. String contents
// (including escape sequences) are skipped so curly braces inside JSON strings
// do not throw the counter off.
func matchBalanced(s string, start int) int {
	if start >= len(s) || s[start] != '{' {
		return -1
	}
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if esc {
			esc = false
			continue
		}
		if c == '\\' {
			esc = true
			continue
		}
		if c == '"' {
			inStr = !inStr
			continue
		}
		if inStr {
			continue
		}
		switch c {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return -1
}

func (d *Deel) resolveBaseURL() string {
	if d.baseURL != "" {
		return d.baseURL
	}
	return defaultBaseURL
}

// ── Interface checks ──────────────────────────────────────────────────────────

var _ core.ListingFetcher = (*Deel)(nil)
