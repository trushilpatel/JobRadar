package dto

// SlotStatus is the snapshot router emits when recording an attempt outcome.
// Exposed to tests and to the exhaustion error translation layer.
type SlotStatus struct {
	SlotID        string
	Provider      string
	KeyID         string
	Model         string
	RPMUsage      int
	RPMLimit      int
	RPDUsage      int
	RPDLimit      int
	TPMUsage      int
	TPMLimit      int
	FailureCount  int
	CooldownUntil int64 // unix millis; 0 = not in cooldown
	LastDenial    string
}
