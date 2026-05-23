package services

import "github.com/trushilpatel/jobradar/common/config"

// SlotsFromConfig converts config slot definitions to LLM service slot options.
func SlotsFromConfig(slots []config.LLMSlotConfig) []SlotOptions {
	result := make([]SlotOptions, 0, len(slots))
	for _, s := range slots {
		result = append(result, SlotOptions{
			KeyID:  s.KeyID,
			APIKey: s.APIKey,
			Model:  s.Model,
			RPM:    s.RPM,
			RPD:    s.RPD,
			TPMIn:  s.TPMIn,
		})
	}
	return result
}
