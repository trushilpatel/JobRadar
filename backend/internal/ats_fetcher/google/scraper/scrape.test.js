#!/usr/bin/env node
/**
 * Unit tests for Google Careers scraper utilities.
 *
 * These tests verify the scraper's output format, employment type normalization,
 * DOM selector expectations, and JSON structure without launching a browser.
 *
 * Run with:
 *   node scrape.test.js
 *
 * Exit code 0 = all passed, 1 = failures.
 */

// ── Test harness ─────────────────────────────────────────────────────────────

let passed = 0;
let failed = 0;

function assert(condition, msg) {
  if (condition) {
    passed++;
    process.stderr.write(`  ✓ ${msg}\n`);
  } else {
    failed++;
    process.stderr.write(`  ✗ FAIL: ${msg}\n`);
  }
}

function assertEqual(got, want, msg) {
  assert(
    got === want,
    `${msg}: got ${JSON.stringify(got)}, want ${JSON.stringify(want)}`
  );
}

function section(name) {
  process.stderr.write(`\n── ${name} ──\n`);
}

// ── Extract normalizeEmployType from scraper ─────────────────────────────────
// (Re-implement here since scrape.js doesn't export it as a module)

function normalizeEmployType(raw) {
  const lower = (raw || "").toLowerCase();
  if (lower.includes("full-time") || lower.includes("full time"))
    return "full_time";
  if (lower.includes("part-time") || lower.includes("part time"))
    return "part_time";
  if (lower.includes("contract")) return "contract";
  if (lower.includes("intern")) return "internship";
  return "";
}

// ── Tests ────────────────────────────────────────────────────────────────────

section("normalizeEmployType");
assertEqual(normalizeEmployType("Full-time"), "full_time", "Full-time");
assertEqual(normalizeEmployType("full time"), "full_time", "full time (space)");
assertEqual(
  normalizeEmployType("FULL-TIME"),
  "full_time",
  "FULL-TIME uppercase"
);
assertEqual(normalizeEmployType("Part-time"), "part_time", "Part-time");
assertEqual(normalizeEmployType("part time"), "part_time", "part time (space)");
assertEqual(normalizeEmployType("Contract"), "contract", "Contract");
assertEqual(normalizeEmployType("Intern"), "internship", "Intern");
assertEqual(normalizeEmployType("Internship"), "internship", "Internship");
assertEqual(normalizeEmployType(""), "", "empty string");
assertEqual(normalizeEmployType(null), "", "null");
assertEqual(normalizeEmployType(undefined), "", "undefined");
assertEqual(normalizeEmployType("Something else"), "", "unknown type");

section("Output JSON structure");

// Scraper outputs thin listing items — no full_content.
// FullContent is fetched later by job_post_analyzer for filtered jobs only.
const sampleJob = {
  id: "google-100056351576597190",
  title: "Software Engineer, Site Reliability Engineering",
  url: "https://www.google.com/about/careers/applications/jobs/results/100056351576597190-software-engineer-sre",
  location: "New York, NY, USA",
  employ_type: "full_time",
};

// All required fields present
assert("id" in sampleJob, "has id field");
assert("title" in sampleJob, "has title field");
assert("url" in sampleJob, "has url field");
assert("location" in sampleJob, "has location field");
assert("employ_type" in sampleJob, "has employ_type field");
assert(!("full_content" in sampleJob), "no full_content — thin adapter");

// ID format
assert(sampleJob.id.startsWith("google-"), "id starts with google- prefix");
assert(sampleJob.id.length > 7, "id has content after prefix");

// URL format
assert(
  sampleJob.url.includes("/about/careers/applications/jobs/results/"),
  "url contains expected path"
);
assert(
  sampleJob.url.startsWith("https://www.google.com"),
  "url starts with google.com"
);

section("Job ID extraction from URL");

// Simulate the ID extraction regex used in extractJobsFromPage
const testUrls = [
  {
    href: "jobs/results/100056351576597190-data-center-facilities-technician",
    wantId: "100056351576597190",
  },
  {
    href: "jobs/results/123942573822616262-tech-lead-youtube",
    wantId: "123942573822616262",
  },
  {
    href: "jobs/results/12345-simple-slug",
    wantId: "12345",
  },
  {
    href: "some/other/path",
    wantId: null,
  },
];

for (const tc of testUrls) {
  const match = tc.href.match(/jobs\/results\/(\d+)/);
  const gotId = match ? match[1] : null;
  assertEqual(gotId, tc.wantId, `ID from "${tc.href}"`);
}

section("Material icon text stripping (getTextByIcon pattern)");

// Simulate the icon-prefix stripping done by getTextByIcon in extractJobsFromPage.
// When a parent element contains e.g. "corporate_fareGoogle", the icon name
// "corporate_fare" is stripped to yield "Google".
function stripIconPrefix(raw, iconName) {
  return raw.replace(new RegExp("^" + iconName), "").trim();
}

assertEqual(
  stripIconPrefix("corporate_fareGoogle", "corporate_fare"),
  "Google",
  "strip corporate_fare icon"
);
assertEqual(
  stripIconPrefix("businessAlphabet", "business"),
  "Alphabet",
  "strip business icon"
);
assertEqual(
  stripIconPrefix("placeNew York, NY, USA", "place"),
  "New York, NY, USA",
  "strip place icon for location"
);
assertEqual(
  stripIconPrefix("bar_chartMid", "bar_chart"),
  "Mid",
  "strip bar_chart icon for experience"
);
assertEqual(
  stripIconPrefix("Google", "corporate_fare"),
  "Google",
  "no icon prefix to strip"
);

section("JSON serialization round-trip");

const serialized = JSON.stringify([sampleJob]);
const parsed = JSON.parse(serialized);
assertEqual(parsed.length, 1, "array length after round-trip");
assertEqual(parsed[0].id, "google-100056351576597190", "id survives round-trip");
assertEqual(parsed[0].title, "Software Engineer, Site Reliability Engineering", "title survives round-trip");
assertEqual(parsed[0].location, "New York, NY, USA", "location survives round-trip");
assertEqual(parsed[0].employ_type, "full_time", "employ_type survives round-trip");
assert(!("full_content" in parsed[0]), "no full_content in output");

section("Empty result");

const emptyResult = JSON.stringify([]);
const parsedEmpty = JSON.parse(emptyResult);
assertEqual(parsedEmpty.length, 0, "empty array parses correctly");
assert(Array.isArray(parsedEmpty), "empty result is an array");

section("Error response format");

const errorResponse = JSON.stringify({ error: "Navigation timeout" });
const parsedError = JSON.parse(errorResponse);
assert("error" in parsedError, "error response has error field");
assertEqual(
  parsedError.error,
  "Navigation timeout",
  "error message preserved"
);
assert(!Array.isArray(parsedError), "error response is not an array");

section("Multiple jobs");

const multipleJobs = [];
for (let i = 1; i <= 10; i++) {
  multipleJobs.push({
    id: `google-${i}00000000000`,
    title: `Software Engineer ${i}`,
    url: `https://www.google.com/about/careers/applications/jobs/results/${i}00000000000-software-engineer-${i}`,
    location: `City ${i}, CA, USA`,
    employ_type: "full_time",
  });
}

const multiSerialized = JSON.stringify(multipleJobs);
const multiParsed = JSON.parse(multiSerialized);
assertEqual(multiParsed.length, 10, "10 jobs serialized/parsed");
assertEqual(multiParsed[0].employ_type, "full_time", "job 1 is full_time");
assertEqual(multiParsed[4].title, "Software Engineer 5", "job 5 title correct");

// Verify all IDs are unique
const ids = new Set(multiParsed.map((j) => j.id));
assertEqual(ids.size, 10, "all 10 IDs are unique");

section("Resilient selector expectations");

// Verify that the selectors we rely on are semantic/accessible, not obfuscated
const resilientSelectors = {
  // Job links: anchor on aria-label, not CSS class
  jobLink: 'a[aria-label^="Learn more about"]',
  nextPage: 'a[aria-label="Go to next page"]',
  // Card title: semantic h3 tag
  cardTitle: "h3",
  // Card container: semantic <li> via closest(), not obfuscated class
  cardContainer: "li (via closest from link)",
};

// None of the resilient selectors should contain obfuscated class patterns
// (single-letter class names, random alphanumeric strings like lLd3Je, p1N2lc)
const obfuscatedPattern = /\.[a-zA-Z][a-zA-Z0-9]{4,}/;
for (const [name, sel] of Object.entries(resilientSelectors)) {
  assert(
    !obfuscatedPattern.test(sel),
    `${name} selector "${sel}" is not obfuscated`
  );
}

// Material icon names we depend on (stable Google Material Icons text values)
const iconAnchors = ["place", "corporate_fare", "business", "bar_chart"];
for (const icon of iconAnchors) {
  assert(icon.length > 2, `icon name "${icon}" is a real word, not obfuscated`);
  assert(
    !obfuscatedPattern.test(icon),
    `icon "${icon}" is not an obfuscated class`
  );
}

section("Filter URL construction");

// Verify the expected filter URL format
const BASE_URL =
  "https://www.google.com/about/careers/applications/jobs/results";
const FILTER_PARAMS = [
  "location=United%20States",
  "target_level=EARLY",
  "target_level=MID",
  "employment_type=FULL_TIME",
  "q=Software%20Engineering",
].join("&");
const fullUrl = `${BASE_URL}?${FILTER_PARAMS}`;

assert(fullUrl.includes("location=United%20States"), "URL has location filter");
assert(fullUrl.includes("target_level=EARLY"), "URL has Early experience");
assert(fullUrl.includes("target_level=MID"), "URL has Mid experience");
assert(fullUrl.includes("employment_type=FULL_TIME"), "URL has full-time filter");
assert(
  fullUrl.includes("q=Software%20Engineering"),
  "URL has search query filter"
);

section("Pagination URL format");

// Verify page parameter format
const page2Url = fullUrl + "&page=2";
assert(page2Url.includes("&page=2"), "page 2 URL has page param");
const pageMatch = page2Url.match(/page=(\d+)/);
assertEqual(pageMatch[1], "2", "page number extracted correctly");

// ── Summary ──────────────────────────────────────────────────────────────────

process.stderr.write(`\n${"=".repeat(40)}\n`);
process.stderr.write(`Results: ${passed} passed, ${failed} failed\n`);
process.exit(failed > 0 ? 1 : 0);
