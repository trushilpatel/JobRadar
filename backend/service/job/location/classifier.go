// Package location classifies a free-text job location string as USA, NonUSA,
// or Unknown. It is a pure-function library with no external dependencies so
// it can be called from any pipeline worker without coupling.
//
// The pipeline uses Classify (via IsNonUSA) as a pre-LLM filter in the
// relevance filter worker. Rejecting clearly non-USA listings saves LLM
// spend and keeps the downstream `jobs` table USA-only.
//
// Design rules
//
//   - "Any USA wins" — a multi-part location ("New York / London") that names
//     at least one USA region is kept. The user can still apply finer filters
//     downstream.
//   - "Unknown stays" — empty strings, plain "Remote", "Worldwide" etc. are
//     left for the LLM scorer to decide. We never reject on missing signal.
//   - Whole-word matching — tokens are matched with word boundaries so
//     "Russian River, CA" does not match the country "Russia".
package location

import (
	"regexp"
	"strings"
)

// Classification is the result of classifying a location string.
type Classification int

const (
	// ClassificationUnknown — the input carries no clear USA / non-USA signal.
	// Callers should treat this as "keep" (the LLM filter decides downstream).
	ClassificationUnknown Classification = iota

	// ClassificationUSA — at least one part of the input names a USA region.
	ClassificationUSA

	// ClassificationNonUSA — every recognised part of the input names a
	// non-USA region and no part is USA.
	ClassificationNonUSA
)

func (c Classification) String() string {
	switch c {
	case ClassificationUSA:
		return "usa"
	case ClassificationNonUSA:
		return "non_usa"
	default:
		return "unknown"
	}
}

// Classify classifies a free-text location string.
//
// Examples:
//
//	Classify("San Francisco, CA")       → ClassificationUSA
//	Classify("Remote - US")             → ClassificationUSA
//	Classify("London, United Kingdom")  → ClassificationNonUSA
//	Classify("Bangalore, India")        → ClassificationNonUSA
//	Classify("New York / London")       → ClassificationUSA   (any-USA-wins)
//	Classify("Remote")                  → ClassificationUnknown
//	Classify("")                        → ClassificationUnknown
func Classify(raw string) Classification {
	if strings.TrimSpace(raw) == "" {
		return ClassificationUnknown
	}

	parts := splitParts(raw)
	if len(parts) == 0 {
		return ClassificationUnknown
	}

	var sawUSA, sawNonUSA bool
	for _, p := range parts {
		switch classifyPart(p) {
		case ClassificationUSA:
			sawUSA = true
		case ClassificationNonUSA:
			sawNonUSA = true
		}
	}

	switch {
	case sawUSA:
		return ClassificationUSA
	case sawNonUSA:
		return ClassificationNonUSA
	default:
		return ClassificationUnknown
	}
}

// IsNonUSA returns true only when the location is unambiguously non-USA.
// Empty / Unknown / Remote-without-region returns false so we never reject
// listings on missing signal.
func IsNonUSA(raw string) bool {
	return Classify(raw) == ClassificationNonUSA
}

// IsUSA returns true when at least one part of the location names a USA region.
func IsUSA(raw string) bool {
	return Classify(raw) == ClassificationUSA
}

// AnyNonUSA returns true when at least one string in the slice classifies as
// NonUSA and no string classifies as USA ("any USA wins" rule).
//
// Returns false for a nil or empty slice so callers can safely pass AllLocations
// without a nil guard — the gate is effectively disabled when no locations are
// known.
func AnyNonUSA(locations []string) bool {
	if len(locations) == 0 {
		return false
	}
	var sawNonUSA bool
	for _, loc := range locations {
		switch Classify(loc) {
		case ClassificationUSA:
			return false // any USA wins — keep the job
		case ClassificationNonUSA:
			sawNonUSA = true
		}
	}
	return sawNonUSA
}

// ── internals ────────────────────────────────────────────────────────────────

// partSplitter matches the separators commonly used by ATSs to join multiple
// locations into one string: "/", ";", "|", "&", newlines.
// Commas are NOT a separator — "San Francisco, CA" is one part, not two.
// "and"/"or" are intentionally NOT separators because they collide with US
// state codes "AND" (none) / "OR" (Oregon) under the case-insensitive flag.
var partSplitter = regexp.MustCompile(`\s*(?:/|;|\||&|\n|\r)\s*`)

// stateSuffix matches a US state code immediately after a comma, preserving
// original case. Examples: ", CA", ", NY", ", D.C.", ", Tx".
// The trailing boundary is `,`, end-of-string, whitespace, or paren.
var stateSuffix = regexp.MustCompile(`(?i),\s*([A-Z]{2})(?:[\s,)]|$)`)

// wordRe builds a whole-word regex for an alternation of tokens. Tokens are
// regex-escaped, sorted longest-first so multi-word tokens (e.g. "united
// states") win over their substrings ("united").
func wordRe(tokens []string) *regexp.Regexp {
	if len(tokens) == 0 {
		return nil
	}
	escaped := make([]string, 0, len(tokens))
	seen := make(map[string]bool, len(tokens))
	for _, t := range tokens {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		escaped = append(escaped, regexp.QuoteMeta(t))
	}
	// Sort longest-first so the alternation prefers longer matches.
	sortLongestFirst(escaped)
	return regexp.MustCompile(`(?i)(?:^|[\W_])(?:` + strings.Join(escaped, "|") + `)(?:[\W_]|$)`)
}

func sortLongestFirst(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && len(s[j]) > len(s[j-1]); j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

var (
	usaTokensRe    = wordRe(usaTokens())
	nonUSATokensRe = wordRe(nonUSATokens())

	// US state two-letter codes (case-insensitive set for membership checks).
	usStateCodes = map[string]struct{}{
		"AL": {}, "AK": {}, "AZ": {}, "AR": {}, "CA": {}, "CO": {}, "CT": {},
		"DE": {}, "FL": {}, "GA": {}, "HI": {}, "ID": {}, "IL": {}, "IN": {},
		"IA": {}, "KS": {}, "KY": {}, "LA": {}, "ME": {}, "MD": {}, "MA": {},
		"MI": {}, "MN": {}, "MS": {}, "MO": {}, "MT": {}, "NE": {}, "NV": {},
		"NH": {}, "NJ": {}, "NM": {}, "NY": {}, "NC": {}, "ND": {}, "OH": {},
		"OK": {}, "OR": {}, "PA": {}, "RI": {}, "SC": {}, "SD": {}, "TN": {},
		"TX": {}, "UT": {}, "VT": {}, "VA": {}, "WA": {}, "WV": {}, "WI": {},
		"WY": {}, "DC": {}, "PR": {},
	}
)

func splitParts(raw string) []string {
	pieces := partSplitter.Split(raw, -1)
	out := make([]string, 0, len(pieces))
	for _, p := range pieces {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// classifyPart returns the classification of one location segment.
//
// Precedence (NonUSA wins over USA inside a single segment because non-USA
// signals are more specific — e.g. "Toronto, USA office" is mis-typed in
// practice but a "Canada" mention overrides a stray "USA" mention; this is
// a defensive call we re-evaluate from real data over time):
//
//  1. NonUSA token present → NonUSA
//  2. USA token present, or trailing US state code, or US state full name → USA
//  3. Otherwise → Unknown
func classifyPart(part string) Classification {
	lower := strings.ToLower(part)
	if nonUSATokensRe != nil && nonUSATokensRe.MatchString(" "+lower+" ") {
		return ClassificationNonUSA
	}
	if usaTokensRe != nil && usaTokensRe.MatchString(" "+lower+" ") {
		return ClassificationUSA
	}
	// Trailing 2-letter state code with original case preserved.
	if m := stateSuffix.FindStringSubmatch(part); len(m) == 2 {
		if _, ok := usStateCodes[strings.ToUpper(m[1])]; ok {
			return ClassificationUSA
		}
	}
	return ClassificationUnknown
}

// usaTokens enumerates the lowercase whole-word tokens that mark a segment
// as USA. Includes country names, US state full names, US territories, and
// common "Remote - US" phrasings.
func usaTokens() []string {
	return []string{
		// Country
		"usa", "u.s.a.", "u.s.a", "u.s.", "u.s", "us",
		"united states", "united states of america", "america", "americas",
		"north america", // listings using this regional banner are typically US-led
		// Remote phrasings (whole-word matched after split — these only match
		// when the segment is exactly the phrase or starts/ends with it).
		"remote us", "remote usa", "remote u.s.", "remote united states",
		"remote - us", "remote - usa", "remote - united states",
		"remote (us)", "remote (usa)", "remote (united states)",
		"us remote", "usa remote", "us-remote", "usa-remote",
		"nationwide", "anywhere in us", "anywhere in usa", "anywhere in the us",
		"anywhere in the united states",
		// US states (full names)
		"alabama", "alaska", "arizona", "arkansas", "california", "colorado",
		"connecticut", "delaware", "florida", "georgia", "hawaii", "idaho",
		"illinois", "indiana", "iowa", "kansas", "kentucky", "louisiana",
		"maine", "maryland", "massachusetts", "michigan", "minnesota",
		"mississippi", "missouri", "montana", "nebraska", "nevada",
		"new hampshire", "new jersey", "new mexico", "new york state",
		"north carolina", "north dakota", "ohio", "oklahoma", "oregon",
		"pennsylvania", "rhode island", "south carolina", "south dakota",
		"tennessee", "texas", "utah", "vermont", "virginia", "washington state",
		"west virginia", "wisconsin", "wyoming",
		"district of columbia", "washington, d.c.", "washington d.c.",
		// US territories
		"puerto rico", "guam", "u.s. virgin islands", "us virgin islands",
		"american samoa", "northern mariana islands",
		// Major US metros that ATSs commonly emit as bare city names without
		// a state code. Limited to globally-unambiguous US cities — names
		// like "Cambridge" or "Springfield" are intentionally omitted because
		// they collide with non-US places.
		"new york", "new york city", "nyc", "brooklyn", "manhattan",
		"san francisco", "sf bay area", "bay area", "silicon valley",
		"palo alto", "mountain view", "menlo park", "redwood city",
		"sunnyvale", "san jose", "oakland", "berkeley",
		"los angeles", "san diego", "sacramento", "fresno",
		"seattle", "bellevue", "redmond",
		"portland, or",
		"chicago", "evanston",
		"boston", "cambridge, ma", "somerville", "waltham",
		"atlanta",
		"miami", "tampa", "orlando", "jacksonville",
		"dallas", "fort worth", "houston", "austin", "san antonio", "el paso",
		"denver", "boulder",
		"phoenix", "tempe", "scottsdale",
		"minneapolis", "st. paul", "saint paul",
		"detroit", "ann arbor",
		"philadelphia", "pittsburgh",
		"baltimore", "annapolis",
		"raleigh", "durham", "charlotte", "chapel hill",
		"nashville", "memphis", "knoxville",
		"new orleans", "baton rouge",
		"kansas city", "st. louis", "saint louis",
		"cincinnati", "cleveland", "columbus", "indianapolis",
		"salt lake city", "provo",
		"las vegas", "reno",
		"honolulu",
		"anchorage",
	}
}

// nonUSATokens enumerates the lowercase whole-word tokens that mark a
// segment as Non-USA. Includes:
//   - countries (and common short forms / demonyms)
//   - regional banners (EMEA, APAC, LATAM, etc.)
//   - the largest non-US metros that ATSs commonly emit as bare city names
//
// Adding a city/country here is the supported extension path — no code
// changes outside this list are required.
func nonUSATokens() []string {
	return []string{
		// Regional banners
		"emea", "apac", "apj", "anz", "latam", "latin america",
		"europe", "european union", "eu", "asia", "asia pacific",
		"middle east", "africa", "oceania", "caribbean",
		"global ex-us", "outside us", "outside the us", "non-us", "non us",
		"international", "worldwide ex-us",

		// Canada
		"canada", "canadian", "toronto", "montreal", "vancouver", "calgary",
		"ottawa", "edmonton", "winnipeg", "quebec", "ontario", "alberta",
		"british columbia", "nova scotia", "saskatchewan", "manitoba",
		"halifax", "victoria, bc",

		// UK / Ireland
		"united kingdom", "uk", "u.k.", "great britain", "britain",
		"england", "scotland", "wales", "northern ireland", "ireland",
		"london", "manchester", "edinburgh", "glasgow", "birmingham",
		"leeds", "liverpool", "bristol", "dublin", "cork", "belfast",
		"oxford", "cambridge, uk",

		// Western Europe
		"france", "paris", "lyon", "marseille", "toulouse", "bordeaux",
		"germany", "berlin", "munich", "hamburg", "frankfurt", "cologne",
		"stuttgart", "düsseldorf", "dusseldorf", "kaiserslautern", "mannheim",
		"netherlands", "holland", "amsterdam", "rotterdam", "the hague",
		"belgium", "brussels", "antwerp",
		"spain", "madrid", "barcelona", "valencia", "seville",
		"portugal", "lisbon", "porto",
		"italy", "rome", "milan", "milano", "turin", "naples", "florence",
		"switzerland", "zurich", "geneva", "bern", "basel",
		"austria", "vienna", "salzburg",
		"luxembourg",

		// Nordics
		"sweden", "stockholm", "gothenburg", "malmö", "malmo",
		"norway", "oslo", "bergen",
		"denmark", "copenhagen", "aarhus",
		"finland", "helsinki",
		"iceland", "reykjavik",

		// Eastern / Central Europe
		"poland", "warsaw", "krakow", "kraków", "gdańsk", "gdansk", "wrocław", "wroclaw",
		"czech republic", "czechia", "prague",
		"slovakia", "bratislava",
		"hungary", "budapest",
		"romania", "bucharest", "cluj",
		"bulgaria", "sofia",
		"greece", "athens", "thessaloniki",
		"turkey", "istanbul", "ankara",
		"ukraine", "kyiv", "kiev", "lviv",
		"russia", "moscow", "saint petersburg", "st petersburg",
		"serbia", "belgrade", "croatia", "zagreb", "slovenia", "ljubljana",
		"estonia", "tallinn", "latvia", "riga", "lithuania", "vilnius",

		// Asia
		"india", "indian", "bangalore", "bengaluru", "mumbai", "delhi",
		"new delhi", "hyderabad", "chennai", "pune", "kolkata", "gurgaon",
		"gurugram", "noida", "ahmedabad", "indore", "jaipur", "bhubaneswar",
		"coimbatore", "nagpur", "kochi", "lucknow", "chandigarh", "surat",
		"visakhapatnam", "vizag", "thiruvananthapuram", "trivandrum",
		"china", "chinese", "beijing", "shanghai", "shenzhen", "guangzhou",
		"hangzhou", "chengdu",
		"hong kong",
		"taiwan", "taipei",
		"japan", "tokyo", "osaka", "yokohama", "kyoto",
		"south korea", "korea", "seoul", "busan",
		"singapore",
		"malaysia", "kuala lumpur",
		"indonesia", "jakarta",
		"thailand", "bangkok",
		"vietnam", "ho chi minh", "hanoi", "ho chi minh city",
		"philippines", "manila", "cebu",
		"pakistan", "karachi", "lahore", "islamabad",
		"bangladesh", "dhaka",
		"sri lanka", "colombo",

		// Middle East
		"united arab emirates", "uae", "dubai", "abu dhabi",
		"saudi arabia", "riyadh", "jeddah",
		"israel", "tel aviv", "jerusalem", "haifa",
		"qatar", "doha",
		"bahrain", "manama",
		"kuwait", "kuwait city",
		"oman", "muscat",
		"jordan", "amman",
		"egypt", "cairo", "alexandria",
		"lebanon", "beirut",

		// Africa
		"south africa", "johannesburg", "cape town", "durban",
		"nigeria", "lagos", "abuja",
		"kenya", "nairobi",
		"ghana", "accra",
		"morocco", "casablanca", "rabat",
		"tunisia", "tunis",
		"ethiopia", "addis ababa",
		"rwanda", "kigali",
		"uganda", "kampala",
		"tanzania", "dar es salaam",

		// Latin America
		"mexico", "mexican", "mexico city", "guadalajara", "monterrey",
		"brazil", "brazilian", "são paulo", "sao paulo", "rio de janeiro",
		"belo horizonte", "brasilia", "brasília",
		"argentina", "buenos aires", "córdoba", "cordoba",
		"chile", "santiago",
		"colombia", "bogotá", "bogota", "medellín", "medellin",
		"peru", "lima",
		"uruguay", "montevideo",
		"venezuela", "caracas",
		"costa rica", "san josé", "san jose, costa rica",
		"panama", "panama city",
		"dominican republic", "santo domingo",
		"ecuador", "quito", "guayaquil",
		"bolivia", "la paz",
		"paraguay", "asunción",
		"guatemala", "guatemala city",
		"el salvador", "san salvador",
		"honduras", "tegucigalpa",
		"nicaragua", "managua",
		"jamaica", "kingston",
		"trinidad and tobago",
		"barbados",
		"bahamas",
		"cuba", "havana",
		"haiti", "port-au-prince",

		// Oceania
		"australia", "australian", "sydney", "melbourne", "brisbane",
		"perth", "adelaide", "canberra",
		"new zealand", "auckland", "wellington", "christchurch",
		"fiji",
	}
}
