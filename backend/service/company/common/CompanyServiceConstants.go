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
)
