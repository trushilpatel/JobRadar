// Package adaptermeta answers narrow, static questions about ATS adapter
// types that both the apps tier (crawler-v2) and the service tier (company
// admin validation) need to agree on. It holds no I/O and no dependency on
// either side — apps/crawler-v2/adapters is allowed to import this package
// (apps -> service is the allowed direction per apps/CLAUDE.md); this
// package must never import anything from apps/.
package adaptermeta

// requiresConfig is the hardcoded set of adapter type names whose
// AdapterBundle registers a non-nil ConfigListing in
// apps/crawler-v2/adapters/registry.go — meaning FetchJobListing alone always
// errors and a non-empty adapter_config JSON is required for the adapter to
// actually crawl a company. Kept in sync with the registry by
// apps/crawler-v2/adapters/registry_requires_config_test.go, which derives
// the "has ConfigListing" set from the registry itself and compares it
// against this map.
var requiresConfig = map[string]bool{
	"workday":   true,
	"eightfold": true,
}

// RequiresConfig reports whether adapterType is one of the known adapters
// that require a non-empty adapter_config JSON to crawl. Matching is exact
// (case-sensitive, no normalization) — consistent with adapter_type being
// treated as an opaque key elsewhere (the `ats` table's `name` column).
//
// An adapter type that is not in the hardcoded set — whether it is a valid
// adapter with no config requirement, or an unrecognised/unknown string —
// returns false. Existence of adapterType as a real adapter is validated
// elsewhere (against the live `ats` table); this function only answers
// "does this specific known-configurable adapter need config".
func RequiresConfig(adapterType string) bool {
	return requiresConfig[adapterType]
}
