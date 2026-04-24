package common

import "fmt"

// Outcome labels used by metrics and logging.
const (
	OutcomeOK              = "ok"
	OutcomeLocalQuotaRPM   = "local_quota_rpm"
	OutcomeLocalQuotaRPD   = "local_quota_rpd"
	OutcomeLocalQuotaTPM   = "local_quota_tpm"
	OutcomeProvider429     = "provider_429"
	OutcomeOtherError      = "other_error"
	OutcomeCooldownSkipped = "cooldown_skipped"
)

// SlotID formats a stable ID used across Redis keys, logs, metrics.
func SlotID(provider, keyID, model string) string {
	return fmt.Sprintf("%s:%s:%s", provider, keyID, model)
}
