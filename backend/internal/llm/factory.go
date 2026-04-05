package llm

import "fmt"

// NewScorer returns the Scorer implementation for the given provider name.
// Add new providers here — zero changes needed in workers or main.
func NewScorer(provider, apiKey string, targetRoles []string, yearsExp int) (Scorer, error) {
	switch provider {
	case "anthropic":
		return NewAnthropicScorer(apiKey, targetRoles, yearsExp), nil
	default:
		return nil, fmt.Errorf("llm: unknown provider %q (supported: anthropic)", provider)
	}
}
