package common

// Config holds runtime configuration for the company service.
type Config struct {
	ScanIntervalMinutes int
}

// AdapterType values for companies.adapter_type
const (
	AdapterGreenhouse = "greenhouse"
	AdapterLever      = "lever"
	AdapterAshby      = "ashby"
	AdapterGoogle     = "google"
	AdapterMeta       = "meta"
	AdapterNetflix    = "netflix"
	AdapterUber       = "uber"
	AdapterRippling   = "rippling"
	AdapterMicrosoft  = "microsoft"
	AdapterApple      = "apple"
	AdapterGem        = "gem"
	AdapterDeel       = "deel"
	AdapterWorkday    = "workday"
	AdapterEightfold  = "eightfold"
)

// AdapterTypes is the canonical list of every adapter the company domain
// knows about, built from the Adapter* consts above. It is the single source
// of truth for "which adapter strings exist" — derive any per-context
// allow-list from this slice rather than hand-maintaining a parallel set.
var AdapterTypes = []string{
	AdapterGreenhouse,
	AdapterLever,
	AdapterAshby,
	AdapterGoogle,
	AdapterMeta,
	AdapterNetflix,
	AdapterUber,
	AdapterRippling,
	AdapterMicrosoft,
	AdapterApple,
	AdapterGem,
	AdapterDeel,
	AdapterWorkday,
	AdapterEightfold,
}

// AdminUnsupportedAdapters lists adapters that are registered + wired into
// the crawler but are intentionally NOT exposed via the admin create/validate
// surface. Subtract this set from AdapterTypes to get the admin-creatable set.
//
// eightfold: the AddCompany modal cannot yet collect the host+domain
// adapter_config the eightfold adapter requires. Remove it here once the modal
// gains an eightfold config form. Keeping the omission a single, explicit,
// co-located entry means re-enabling an adapter is one deliberate edit.
var AdminUnsupportedAdapters = map[string]bool{
	AdapterEightfold: true,
}
