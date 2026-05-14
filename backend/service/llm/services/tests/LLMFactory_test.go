package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/common/logger"
	llmcommon "github.com/trushilpatel/jobradar/service/llm/common"
	"github.com/trushilpatel/jobradar/service/llm/services"
	"github.com/trushilpatel/jobradar/service/llm/store"
)

func newStores() (store.LLMRateLimitStore, store.LLMCooldownStore) {
	return store.NewLLMRateLimitMemoryStore(),
		store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: time.Second, Max: time.Minute})
}

func TestNewScorer_RejectsUnsupportedProvider(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")

	rls, cds := newStores()
	_, err := services.NewScorer(context.Background(), services.Options{
		Provider:       "openai",
		RateLimitStore: rls,
		CooldownStore:  cds,
		Slots:          []services.SlotOptions{{KeyID: "k", APIKey: "v", Model: "m"}},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported provider") {
		t.Fatalf("expected unsupported provider error, got %v", err)
	}
}

func TestNewScorer_RequiresRateLimitStore(t *testing.T) {
	_, cds := newStores()
	_, err := services.NewScorer(context.Background(), services.Options{
		Provider:      llmcommon.ProviderGemini,
		CooldownStore: cds,
		Slots:         []services.SlotOptions{{KeyID: "k", APIKey: "v", Model: "m"}},
	})
	if err == nil || !strings.Contains(err.Error(), "RateLimitStore is required") {
		t.Fatalf("expected RateLimitStore required error, got %v", err)
	}
}

func TestNewScorer_RequiresCooldownStore(t *testing.T) {
	rls, _ := newStores()
	_, err := services.NewScorer(context.Background(), services.Options{
		Provider:       llmcommon.ProviderGemini,
		RateLimitStore: rls,
		Slots:          []services.SlotOptions{{KeyID: "k", APIKey: "v", Model: "m"}},
	})
	if err == nil || !strings.Contains(err.Error(), "CooldownStore is required") {
		t.Fatalf("expected CooldownStore required error, got %v", err)
	}
}

func TestNewScorer_RejectsEmptySlots(t *testing.T) {
	rls, cds := newStores()
	_, err := services.NewScorer(context.Background(), services.Options{
		Provider:       llmcommon.ProviderGemini,
		RateLimitStore: rls,
		CooldownStore:  cds,
	})
	if err == nil || !strings.Contains(err.Error(), "no slots configured") {
		t.Fatalf("expected no-slots error, got %v", err)
	}
}

func TestNewScorer_RejectsSlotMissingKeyID(t *testing.T) {
	rls, cds := newStores()
	_, err := services.NewScorer(context.Background(), services.Options{
		Provider:       llmcommon.ProviderGemini,
		RateLimitStore: rls,
		CooldownStore:  cds,
		Slots:          []services.SlotOptions{{APIKey: "v", Model: "m"}},
	})
	if err == nil || !strings.Contains(err.Error(), "missing KeyID") {
		t.Fatalf("expected missing KeyID error, got %v", err)
	}
}

func TestNewScorer_RejectsSlotMissingAPIKey(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")

	rls, cds := newStores()
	_, err := services.NewScorer(context.Background(), services.Options{
		Provider:       llmcommon.ProviderGemini,
		RateLimitStore: rls,
		CooldownStore:  cds,
		Slots:          []services.SlotOptions{{KeyID: "primary", Model: "m"}},
	})
	if err == nil || !strings.Contains(err.Error(), "missing APIKey") {
		t.Fatalf("expected missing APIKey error, got %v", err)
	}
}

func TestNewScorer_RejectsSlotMissingModel(t *testing.T) {
	rls, cds := newStores()
	_, err := services.NewScorer(context.Background(), services.Options{
		Provider:       llmcommon.ProviderGemini,
		RateLimitStore: rls,
		CooldownStore:  cds,
		Slots:          []services.SlotOptions{{KeyID: "primary", APIKey: "v"}},
	})
	if err == nil || !strings.Contains(err.Error(), "missing Model") {
		t.Fatalf("expected missing Model error, got %v", err)
	}
}

func TestNewScorer_RejectsDuplicateSlotID(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")

	rls, cds := newStores()
	_, err := services.NewScorer(context.Background(), services.Options{
		Provider:       llmcommon.ProviderGemini,
		RateLimitStore: rls,
		CooldownStore:  cds,
		Slots: []services.SlotOptions{
			{KeyID: "k", APIKey: "v", Model: "m"},
			{KeyID: "k", APIKey: "v", Model: "m"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate slot id") {
		t.Fatalf("expected duplicate slot id error, got %v", err)
	}
}

func TestNewScorer_BuildsRouterForValidSlots(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")

	rls, cds := newStores()
	scorer, err := services.NewScorer(context.Background(), services.Options{
		Provider:       llmcommon.ProviderGemini,
		TargetRoles:    []string{"SWE"},
		YearsExp:       3,
		RateLimitStore: rls,
		CooldownStore:  cds,
		Log:            logger.NewNoop(),
		Slots: []services.SlotOptions{
			{KeyID: "k1", APIKey: "v1", Model: "gemini-3.1-flash-lite-preview", RPM: 60, RPD: 1000, TPMIn: 10000},
			{KeyID: "k2", APIKey: "v2", Model: "gemma-4-31b-it"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scorer == nil {
		t.Fatal("expected non-nil scorer")
	}
}

func TestNewScorer_BuildsRouterWithoutLogger(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")

	rls, cds := newStores()
	scorer, err := services.NewScorer(context.Background(), services.Options{
		Provider:       llmcommon.ProviderGemini,
		RateLimitStore: rls,
		CooldownStore:  cds,
		Slots:          []services.SlotOptions{{KeyID: "k1", APIKey: "v1", Model: "m"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scorer == nil {
		t.Fatal("expected non-nil scorer")
	}
}
