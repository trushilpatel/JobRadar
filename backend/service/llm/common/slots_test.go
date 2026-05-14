package common_test

import (
	"testing"

	llmcommon "github.com/trushilpatel/jobradar/service/llm/common"
)

func TestSlotID_FormatsStableTriple(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		keyID    string
		model    string
		want     string
	}{
		{
			name:     "basic gemini slot",
			provider: llmcommon.ProviderGemini,
			keyID:    "primary",
			model:    "gemini-3.1-flash-lite-preview",
			want:     "gemini:primary:gemini-3.1-flash-lite-preview",
		},
		{
			name:     "empty parts still produce stable separators",
			provider: "",
			keyID:    "",
			model:    "",
			want:     "::",
		},
		{
			name:     "key id with special characters preserved verbatim",
			provider: "gemini",
			keyID:    "key-2_alt",
			model:    "gemma-4-31b-it",
			want:     "gemini:key-2_alt:gemma-4-31b-it",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := llmcommon.SlotID(tc.provider, tc.keyID, tc.model)
			if got != tc.want {
				t.Fatalf("SlotID(%q,%q,%q) = %q, want %q",
					tc.provider, tc.keyID, tc.model, got, tc.want)
			}
		})
	}
}

func TestSlotID_DistinctForDistinctKeysAndModels(t *testing.T) {
	// Two slots that share provider but differ on key or model must produce
	// distinct IDs — this property underpins per-slot Redis isolation.
	a := llmcommon.SlotID("gemini", "k1", "m1")
	b := llmcommon.SlotID("gemini", "k1", "m2")
	c := llmcommon.SlotID("gemini", "k2", "m1")
	if a == b || a == c || b == c {
		t.Fatalf("slot ids must be distinct; got a=%q b=%q c=%q", a, b, c)
	}
}
