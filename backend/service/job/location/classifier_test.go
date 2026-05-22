package location

import "testing"

func TestClassify_USA(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"country full name", "United States"},
		{"country short", "USA"},
		{"country with punctuation", "U.S.A."},
		{"country bare us", "US"},
		{"city state code", "San Francisco, CA"},
		{"city state code lowercase suffix", "san francisco, ca"},
		{"city state full name", "Houston, Texas"},
		{"city alone NYC", "New York"},
		{"city alone NYC variant", "New York City"},
		{"bay area shorthand", "Bay Area"},
		{"silicon valley", "Silicon Valley"},
		{"seattle bare", "Seattle"},
		{"chicago bare", "Chicago"},
		{"atlanta bare", "Atlanta"},
		{"austin bare", "Austin"},
		{"city with USA suffix", "San Francisco, CA, USA"},
		{"DC variant", "Washington, D.C."},
		{"district of columbia", "District of Columbia"},
		{"puerto rico", "San Juan, Puerto Rico"},
		{"remote US dash", "Remote - US"},
		{"remote US paren", "Remote (US)"},
		{"remote United States", "Remote - United States"},
		{"remote USA bare", "Remote USA"},
		{"US Remote", "US Remote"},
		{"nationwide", "Nationwide"},
		{"anywhere in US", "Anywhere in the US"},
		{"state code uppercase only", "Remote, TX"},
		{"multi-state list", "Remote, CA / Remote, NY"},
		{"multi mixed any USA wins", "London / New York"},
		{"multi mixed nonUSA + USA", "Bangalore, India / San Francisco, CA"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Classify(tc.in)
			if got != ClassificationUSA {
				t.Errorf("Classify(%q) = %s, want USA", tc.in, got)
			}
			if !IsUSA(tc.in) {
				t.Errorf("IsUSA(%q) = false, want true", tc.in)
			}
			if IsNonUSA(tc.in) {
				t.Errorf("IsNonUSA(%q) = true, want false", tc.in)
			}
		})
	}
}

func TestClassify_NonUSA(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"london bare", "London"},
		{"london uk", "London, UK"},
		{"london united kingdom", "London, United Kingdom"},
		{"paris", "Paris, France"},
		{"berlin", "Berlin, Germany"},
		{"amsterdam", "Amsterdam"},
		{"dublin", "Dublin, Ireland"},
		{"toronto canada", "Toronto, Canada"},
		{"toronto ontario", "Toronto, Ontario"},
		{"vancouver bare", "Vancouver"},
		{"montreal bare", "Montreal"},
		{"bangalore india", "Bangalore, India"},
		{"bengaluru", "Bengaluru"},
		{"mumbai", "Mumbai"},
		{"hyderabad", "Hyderabad, India"},
		{"new delhi", "New Delhi"},
		{"tokyo", "Tokyo, Japan"},
		{"singapore", "Singapore"},
		{"hong kong", "Hong Kong"},
		{"sydney australia", "Sydney, Australia"},
		{"melbourne", "Melbourne"},
		{"auckland nz", "Auckland, New Zealand"},
		{"sao paulo", "São Paulo, Brazil"},
		{"mexico city", "Mexico City"},
		{"dubai", "Dubai, UAE"},
		{"tel aviv", "Tel Aviv, Israel"},
		{"region emea", "Remote - EMEA"},
		{"region apac", "Remote - APAC"},
		{"region latam", "Remote, LATAM"},
		{"europe", "Remote - Europe"},
		{"non-us", "Remote (Non-US)"},
		{"outside us", "Remote, outside the US"},
		{"all parts nonUSA", "London / Paris / Berlin"},
		{"city with country", "Cambridge, UK"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Classify(tc.in)
			if got != ClassificationNonUSA {
				t.Errorf("Classify(%q) = %s, want NonUSA", tc.in, got)
			}
			if !IsNonUSA(tc.in) {
				t.Errorf("IsNonUSA(%q) = false, want true", tc.in)
			}
			if IsUSA(tc.in) {
				t.Errorf("IsUSA(%q) = true, want false", tc.in)
			}
		})
	}
}

func TestClassify_Unknown(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"whitespace only", "   \n\t "},
		{"just remote", "Remote"},
		{"remote lowercase", "remote"},
		{"worldwide", "Worldwide"},
		{"global", "Global"},
		{"flexible", "Flexible"},
		{"hybrid", "Hybrid"},
		{"anywhere", "Anywhere"},
		{"city with no state", "Springfield"},
		{"random gibberish", "asdf qwer"},
		{"just a comma", ","},
		{"separators only", "/ ; |"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Classify(tc.in)
			if got != ClassificationUnknown {
				t.Errorf("Classify(%q) = %s, want Unknown", tc.in, got)
			}
			if IsNonUSA(tc.in) {
				t.Errorf("IsNonUSA(%q) = true, want false (Unknown must not be rejected)", tc.in)
			}
			if IsUSA(tc.in) {
				t.Errorf("IsUSA(%q) = true, want false", tc.in)
			}
		})
	}
}

// Boundary safety: tokens must match only as whole words.
func TestClassify_NoFalsePositives(t *testing.T) {
	cases := []struct {
		in       string
		want     Classification
		whyLabel string
	}{
		// "us" must not match inside other words.
		{"Status review, remote", ClassificationUnknown, "status contains 'us' substring"},
		{"Customer Success - Anywhere", ClassificationUnknown, "customer/success contain 'us'"},
		{"bus driver, anywhere", ClassificationUnknown, "bus starts with 'us' substring"},
		// "eu" must not match inside other words.
		{"Eureka office", ClassificationUnknown, "eureka contains 'eu'"},
		{"Pharmaceutical, Anywhere", ClassificationUnknown, "pharmaceutical contains 'eu'"},
		// "russia" must not match "Russian River" (a US wine region).
		{"Russian River, CA", ClassificationUSA, "russian river is in California"},
		// "asia" must not match "Asiago" etc — and we still want clear NonUSA when actually Asia.
		{"Asia", ClassificationNonUSA, "asia region"},
		// State suffix must require a true comma boundary.
		{"NYCABC", ClassificationUnknown, "no boundary around nyc"},
		// "in" / "or" / "and" must not be country/state-code matches.
		{"Pune, in", ClassificationNonUSA, "pune is non-USA city; 'in' alone is not a state code"},
		{"Portland, OR", ClassificationUSA, "Portland, Oregon"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got := Classify(tc.in)
			if got != tc.want {
				t.Errorf("Classify(%q) = %s, want %s (%s)", tc.in, got, tc.want, tc.whyLabel)
			}
		})
	}
}

// Multi-part location splitting precedence.
func TestClassify_MultiPart(t *testing.T) {
	cases := []struct {
		in   string
		want Classification
	}{
		{"New York / London", ClassificationUSA},
		{"London / New York", ClassificationUSA},
		{"San Francisco, CA; Toronto, Canada", ClassificationUSA},
		{"Toronto, Canada; Vancouver, Canada", ClassificationNonUSA},
		{"London | Paris | Berlin", ClassificationNonUSA},
		{"Remote\nLondon", ClassificationNonUSA},
		{"Remote / New York", ClassificationUSA},
		{"Bangalore, India; Hyderabad", ClassificationNonUSA},
		{"San Francisco, CA & Remote", ClassificationUSA},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got := Classify(tc.in)
			if got != tc.want {
				t.Errorf("Classify(%q) = %s, want %s", tc.in, got, tc.want)
			}
		})
	}
}

// All 50 US state codes resolve via the trailing-suffix path.
func TestClassify_AllStateCodes(t *testing.T) {
	codes := []string{
		"AL", "AK", "AZ", "AR", "CA", "CO", "CT", "DE", "FL", "GA",
		"HI", "ID", "IL", "IN", "IA", "KS", "KY", "LA", "ME", "MD",
		"MA", "MI", "MN", "MS", "MO", "MT", "NE", "NV", "NH", "NJ",
		"NM", "NY", "NC", "ND", "OH", "OK", "OR", "PA", "RI", "SC",
		"SD", "TN", "TX", "UT", "VT", "VA", "WA", "WV", "WI", "WY",
		"DC", "PR",
	}
	for _, code := range codes {
		code := code
		t.Run(code, func(t *testing.T) {
			t.Parallel()
			in := "Somewhereville, " + code
			got := Classify(in)
			if got != ClassificationUSA {
				t.Errorf("Classify(%q) = %s, want USA", in, got)
			}
		})
	}
}

// Non-US country codes after a comma must NOT be misread as US states.
func TestClassify_NonUSCountryCodesNotMistakenForStates(t *testing.T) {
	// Country codes that are not US state codes — these should NOT trigger
	// USA classification via the state-suffix path. (Most pair with a
	// clearly non-USA city which independently triggers NonUSA.)
	cases := []struct {
		in   string
		want Classification
	}{
		{"Toronto, ON", ClassificationNonUSA},   // city → nonUSA
		{"Vancouver, BC", ClassificationNonUSA}, // city → nonUSA
		{"Sherbrooke, QC, Canada", ClassificationNonUSA},
		{"Edmonton, AB", ClassificationNonUSA},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got := Classify(tc.in)
			if got != tc.want {
				t.Errorf("Classify(%q) = %s, want %s", tc.in, got, tc.want)
			}
		})
	}
}

func TestClassification_String(t *testing.T) {
	cases := []struct {
		c    Classification
		want string
	}{
		{ClassificationUSA, "usa"},
		{ClassificationNonUSA, "non_usa"},
		{ClassificationUnknown, "unknown"},
		{Classification(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.c.String(); got != tc.want {
			t.Errorf("Classification(%d).String() = %q, want %q", tc.c, got, tc.want)
		}
	}
}

// TestAnyNonUSA_EmptySlice — false for nil/empty input.
func TestAnyNonUSA_EmptySlice(t *testing.T) {
	if AnyNonUSA(nil) {
		t.Error("AnyNonUSA(nil) = true, want false")
	}
	if AnyNonUSA([]string{}) {
		t.Error("AnyNonUSA([]) = true, want false")
	}
}

// TestAnyNonUSA_AnyUSAWins — if any string classifies as USA, result is false.
func TestAnyNonUSA_AnyUSAWins(t *testing.T) {
	cases := [][]string{
		{"San Francisco, CA"},
		{"London, UK", "New York, NY"},
		{"London, UK", "San Francisco, CA", "Berlin, Germany"},
	}
	for _, locs := range cases {
		if AnyNonUSA(locs) {
			t.Errorf("AnyNonUSA(%v) = true, want false (any-USA-wins rule)", locs)
		}
	}
}

// TestAnyNonUSA_AllNonUSA — true only when at least one is NonUSA and none is USA.
func TestAnyNonUSA_AllNonUSA(t *testing.T) {
	cases := [][]string{
		{"London, UK"},
		{"Bangalore, India"},
		{"London, UK", "Berlin, Germany"},
		{"Remote - EMEA"},
	}
	for _, locs := range cases {
		if !AnyNonUSA(locs) {
			t.Errorf("AnyNonUSA(%v) = false, want true", locs)
		}
	}
}

// TestAnyNonUSA_AllUnknown — false when every string is Unknown (never reject on missing signal).
func TestAnyNonUSA_AllUnknown(t *testing.T) {
	cases := [][]string{
		{"Remote"},
		{"Worldwide"},
		{""},
		{"Remote", "Worldwide"},
	}
	for _, locs := range cases {
		if AnyNonUSA(locs) {
			t.Errorf("AnyNonUSA(%v) = true, want false (unknown = keep)", locs)
		}
	}
}

func BenchmarkClassify(b *testing.B) {
	cases := []string{
		"San Francisco, CA",
		"London, United Kingdom",
		"Bangalore, India / San Francisco, CA",
		"Remote",
		"",
		"Remote - EMEA",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Classify(cases[i%len(cases)])
	}
}
