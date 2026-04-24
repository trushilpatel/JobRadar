package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	llmerr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/llm/services"
	"github.com/trushilpatel/jobradar/service/llm/store"
)

func TestRouter_FallsBackOnLocalQuota(t *testing.T) {
	quota := &fakeScorer{scoreErr: llmerr.ErrLLMLocalQuotaExceeded}
	ok := &fakeScorer{scoreOut: []string{"id-1"}}
	r := services.NewLLMRouterService([]services.RouterSlot{
		{SlotID: "s1", Model: "m1", Scorer: quota},
		{SlotID: "s2", Model: "m1", Scorer: ok},
	}, store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: time.Second, Max: time.Minute}))

	out, err := r.ScoreBatch(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0] != "id-1" {
		t.Fatalf("unexpected %v", out)
	}
}

func TestRouter_RecordsCooldownOnProvider429(t *testing.T) {
	limited := &fakeScorer{scoreErr: llmerr.ErrLLMProviderRateLimited}
	ok := &fakeScorer{scoreOut: []string{"id-1"}}
	cd := store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: 10 * time.Second, Max: time.Minute})
	r := services.NewLLMRouterService([]services.RouterSlot{
		{SlotID: "s1", Model: "m1", Scorer: limited},
		{SlotID: "s2", Model: "m1", Scorer: ok},
	}, cd)

	if _, err := r.ScoreBatch(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	st, err := cd.Status(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if st.FailureCount != 1 {
		t.Fatalf("want cooldown recorded, got %+v", st)
	}
}

func TestRouter_BubblesOtherErrors(t *testing.T) {
	boom := &fakeScorer{scoreErr: errors.New("network boom")}
	ok := &fakeScorer{scoreOut: []string{"x"}}
	r := services.NewLLMRouterService([]services.RouterSlot{
		{SlotID: "s1", Model: "m1", Scorer: boom},
		{SlotID: "s2", Model: "m1", Scorer: ok},
	}, store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: time.Second, Max: time.Minute}))

	_, err := r.ScoreBatch(context.Background(), nil)
	if err == nil || err.Error() != "network boom" {
		t.Fatalf("expected network boom to bubble, got %v", err)
	}
}

func TestRouter_ReturnsAllExhaustedError(t *testing.T) {
	q1 := &fakeScorer{scoreErr: llmerr.ErrLLMLocalQuotaExceeded}
	q2 := &fakeScorer{scoreErr: llmerr.ErrLLMLocalQuotaExceeded}
	r := services.NewLLMRouterService([]services.RouterSlot{
		{SlotID: "s1", Model: "m1", Scorer: q1},
		{SlotID: "s2", Model: "m1", Scorer: q2},
	}, store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: time.Second, Max: time.Minute}))

	_, err := r.ScoreBatch(context.Background(), nil)
	var ae *llmerr.LLMAllExhaustedError
	if !errors.As(err, &ae) {
		t.Fatalf("want LLMAllExhaustedError, got %v", err)
	}
	if len(ae.Slots) != 2 {
		t.Fatalf("want 2 slot statuses, got %d", len(ae.Slots))
	}
}
