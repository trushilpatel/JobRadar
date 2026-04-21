package config

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

// llmSlotsYAML is the on-disk shape of the LLM slots config file.
type llmSlotsYAML struct {
	Filter struct {
		Slots []LLMSlotConfig `yaml:"slots"`
	} `yaml:"filter"`
	Enricher struct {
		Slots []LLMSlotConfig `yaml:"slots"`
	} `yaml:"enricher"`
}

var envRefPattern = regexp.MustCompile(`\$\{([A-Z0-9_]+)\}`)

// LoadLLMSlotsYAML reads filter + enricher slot config from a YAML file.
// String fields containing ${VAR} are expanded against the current environment
// so API keys stay in .env rather than checked into the YAML file.
// Returns (nil, nil, nil) when path is empty — callers fall back to env JSON.
func LoadLLMSlotsYAML(path string) (filter, enricher []LLMSlotConfig, err error) {
	if path == "" {
		return nil, nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("config: read llm slots yaml %s: %w", path, err)
	}
	raw, err = expandEnvRefs(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("config: unresolved env refs in %s: %w", path, err)
	}

	var doc llmSlotsYAML
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, nil, fmt.Errorf("config: parse llm slots yaml %s: %w", path, err)
	}
	return doc.Filter.Slots, doc.Enricher.Slots, nil
}

// expandEnvRefs replaces ${VAR} occurrences in the YAML bytes with the process
// env value for VAR. If any ${VAR} is unset, it accumulates the missing names
// and returns an aggregated error — never a silent literal passthrough.
//
// Edge case: a var that is set to the empty string is treated as resolved. Go's
// os.LookupEnv distinguishes "set" from "unset"; if the operator explicitly set
// VAR="" we trust that and expand to empty. Only truly-unset vars are reported.
// Duplicate missing names are de-duplicated in the error message.
func expandEnvRefs(in []byte) ([]byte, error) {
	var missingOrder []string
	missingSeen := make(map[string]struct{})
	out := envRefPattern.ReplaceAllFunc(in, func(match []byte) []byte {
		name := string(match[2 : len(match)-1])
		if v, ok := os.LookupEnv(name); ok {
			return []byte(v)
		}
		if _, dup := missingSeen[name]; !dup {
			missingSeen[name] = struct{}{}
			missingOrder = append(missingOrder, name)
		}
		return match
	})
	if len(missingOrder) > 0 {
		return nil, fmt.Errorf("%v", missingOrder)
	}
	return out, nil
}
