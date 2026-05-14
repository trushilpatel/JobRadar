package tests

import (
	"context"
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/trushilpatel/jobradar/service/llm/store"
)

func TestMemoryStore_RPMBoundary(t *testing.T) {
	s := store.NewLLMRateLimitMemoryStore()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		r, err := s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 3, RPD: 100, TPMIn: 0, ReqID: id(i), Tokens: 0})
		mustOK(t, r, err)
	}
	r, err := s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 3, RPD: 100, ReqID: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if r != store.ReserveDeniedRPM {
		t.Fatalf("want ReserveDeniedRPM, got %v", r)
	}
}

func TestMemoryStore_RPDBoundary(t *testing.T) {
	s := store.NewLLMRateLimitMemoryStore()
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		r, err := s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 100, RPD: 2, ReqID: id(i)})
		mustOK(t, r, err)
	}
	r, err := s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 100, RPD: 2, ReqID: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if r != store.ReserveDeniedRPD {
		t.Fatalf("want ReserveDeniedRPD, got %v", r)
	}
}

func TestMemoryStore_TPMBoundary(t *testing.T) {
	s := store.NewLLMRateLimitMemoryStore()
	ctx := context.Background()
	r, err := s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 100, RPD: 100, TPMIn: 1000, ReqID: "a", Tokens: 600})
	mustOK(t, r, err)
	r, err = s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 100, RPD: 100, TPMIn: 1000, ReqID: "b", Tokens: 500})
	if err != nil {
		t.Fatal(err)
	}
	if r != store.ReserveDeniedTPM {
		t.Fatalf("want TPM denial, got %v", r)
	}
}

func TestMemoryStore_Release(t *testing.T) {
	s := store.NewLLMRateLimitMemoryStore()
	ctx := context.Background()
	_, _ = s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 1, RPD: 100, ReqID: "a"})
	if err := s.Release(ctx, "s1", "a"); err != nil {
		t.Fatal(err)
	}
	r, err := s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 1, RPD: 100, ReqID: "b"})
	mustOK(t, r, err)
}

func TestMemoryStore_Usage(t *testing.T) {
	s := store.NewLLMRateLimitMemoryStore()
	ctx := context.Background()
	_, _ = s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 10, RPD: 10, TPMIn: 1000, ReqID: "a", Tokens: 100})
	_, _ = s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 10, RPD: 10, TPMIn: 1000, ReqID: "b", Tokens: 200})
	u, err := s.Usage(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if u.RPM != 2 {
		t.Fatalf("want RPM=2, got %d", u.RPM)
	}
	if u.RPD != 2 {
		t.Fatalf("want RPD=2, got %d", u.RPD)
	}
	if u.TPM != 300 {
		t.Fatalf("want TPM=300, got %d", u.TPM)
	}
}

func mustOK(t *testing.T, r store.ReserveResult, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if r != store.ReserveOK {
		t.Fatalf("want ReserveOK, got %v", r)
	}
}

func id(i int) string { return []string{"a", "b", "c", "d", "e"}[i] }

func newRedisStore(t *testing.T) (store.LLMRateLimitStore, func()) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	return store.NewLLMRateLimitRedisStore(rdb), func() {
		_ = rdb.Close()
		mr.Close()
	}
}

func TestRedisStore_RPMBoundary(t *testing.T) {
	s, cleanup := newRedisStore(t)
	defer cleanup()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		r, err := s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 3, RPD: 100, ReqID: fmt.Sprintf("r%d", i)})
		mustOK(t, r, err)
	}
	r, err := s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 3, RPD: 100, ReqID: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if r != store.ReserveDeniedRPM {
		t.Fatalf("want RPM denial, got %v", r)
	}
}

func TestRedisStore_RPDBoundary(t *testing.T) {
	s, cleanup := newRedisStore(t)
	defer cleanup()
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		r, err := s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 100, RPD: 2, ReqID: fmt.Sprintf("r%d", i)})
		mustOK(t, r, err)
	}
	r, err := s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 100, RPD: 2, ReqID: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if r != store.ReserveDeniedRPD {
		t.Fatalf("want RPD denial, got %v", r)
	}
}

func TestRedisStore_TPMBoundary(t *testing.T) {
	s, cleanup := newRedisStore(t)
	defer cleanup()
	ctx := context.Background()
	r, err := s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 100, RPD: 100, TPMIn: 1000, ReqID: "a", Tokens: 600})
	mustOK(t, r, err)
	r, err = s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 100, RPD: 100, TPMIn: 1000, ReqID: "b", Tokens: 500})
	if err != nil {
		t.Fatal(err)
	}
	if r != store.ReserveDeniedTPM {
		t.Fatalf("want TPM denial, got %v", r)
	}
}

func TestRedisStore_Release(t *testing.T) {
	s, cleanup := newRedisStore(t)
	defer cleanup()
	ctx := context.Background()
	_, err := s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 1, RPD: 100, ReqID: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Release(ctx, "s1", "a"); err != nil {
		t.Fatal(err)
	}
	r, err := s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 1, RPD: 100, ReqID: "b"})
	mustOK(t, r, err)
}

func TestRedisStore_Usage(t *testing.T) {
	s, cleanup := newRedisStore(t)
	defer cleanup()
	ctx := context.Background()
	_, err := s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 10, RPD: 10, TPMIn: 1000, ReqID: "a", Tokens: 100})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 10, RPD: 10, TPMIn: 1000, ReqID: "b", Tokens: 200})
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.Usage(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if u.RPM != 2 {
		t.Fatalf("want RPM=2, got %d", u.RPM)
	}
	if u.RPD != 2 {
		t.Fatalf("want RPD=2, got %d", u.RPD)
	}
	if u.TPM != 300 {
		t.Fatalf("want TPM=300, got %d", u.TPM)
	}
}

func TestRedisStore_UsageEmptySlot(t *testing.T) {
	s, cleanup := newRedisStore(t)
	defer cleanup()
	u, err := s.Usage(context.Background(), "never-used")
	if err != nil {
		t.Fatal(err)
	}
	if u.RPM != 0 || u.RPD != 0 || u.TPM != 0 {
		t.Fatalf("expected zero usage for untouched slot, got %+v", u)
	}
}

func TestRedisStore_ReleaseAfterTokenReservation(t *testing.T) {
	s, cleanup := newRedisStore(t)
	defer cleanup()
	ctx := context.Background()
	// Reserve with non-zero Tokens so Release must trim the token hash + sum.
	r, err := s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 10, RPD: 100, TPMIn: 1000, ReqID: "a", Tokens: 250})
	mustOK(t, r, err)

	if err := s.Release(ctx, "s1", "a"); err != nil {
		t.Fatal(err)
	}

	// After Release the slot should report zero RPM/TPM and a decremented day.
	u, err := s.Usage(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if u.RPM != 0 {
		t.Fatalf("want RPM=0 after release, got %d", u.RPM)
	}
	if u.TPM != 0 {
		t.Fatalf("want TPM=0 after release, got %d", u.TPM)
	}
}

func TestRedisStore_ReleaseUnknownReqIDIsNoOp(t *testing.T) {
	s, cleanup := newRedisStore(t)
	defer cleanup()
	// Release for a slot/req we never reserved must not error.
	if err := s.Release(context.Background(), "ghost", "missing"); err != nil {
		t.Fatalf("Release on unknown slot/req must be a no-op error-wise: %v", err)
	}
}

func TestMemoryStore_ReleaseUnknownSlotIsNoOp(t *testing.T) {
	s := store.NewLLMRateLimitMemoryStore()
	if err := s.Release(context.Background(), "never", "x"); err != nil {
		t.Fatalf("unknown slot release should be no-op, got %v", err)
	}
}

func TestMemoryStore_ReleaseUnknownReqIDIsNoOp(t *testing.T) {
	s := store.NewLLMRateLimitMemoryStore()
	ctx := context.Background()
	_, _ = s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 10, RPD: 10, ReqID: "a"})
	if err := s.Release(ctx, "s1", "never-reserved"); err != nil {
		t.Fatalf("unknown reqID release should be no-op, got %v", err)
	}
	// Should not have changed any state — usage still shows 1.
	u, _ := s.Usage(ctx, "s1")
	if u.RPM != 1 {
		t.Fatalf("usage must remain 1 after no-op release, got %d", u.RPM)
	}
}

func TestMemoryStore_ReleaseRemovesTokenEntry(t *testing.T) {
	s := store.NewLLMRateLimitMemoryStore()
	ctx := context.Background()
	_, _ = s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 10, RPD: 10, TPMIn: 1000, ReqID: "a", Tokens: 200})
	_, _ = s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 10, RPD: 10, TPMIn: 1000, ReqID: "b", Tokens: 300})

	if err := s.Release(ctx, "s1", "a"); err != nil {
		t.Fatal(err)
	}
	u, _ := s.Usage(ctx, "s1")
	// After releasing "a" (200 tokens) only "b" (300) remains.
	if u.RPM != 1 {
		t.Fatalf("want RPM=1 after release, got %d", u.RPM)
	}
	if u.TPM != 300 {
		t.Fatalf("want TPM=300 after release, got %d", u.TPM)
	}
}

func TestMemoryStore_UsageOnFreshSlot(t *testing.T) {
	s := store.NewLLMRateLimitMemoryStore()
	u, err := s.Usage(context.Background(), "ghost")
	if err != nil {
		t.Fatal(err)
	}
	if u.RPM != 0 || u.RPD != 0 || u.TPM != 0 {
		t.Fatalf("fresh slot should be empty, got %+v", u)
	}
}

func TestMemoryStore_TPMUnlimitedAllowsLargeBatch(t *testing.T) {
	// TPMIn=0 -> unlimited. A large batch must succeed even if it would exceed
	// any cap. RPM still applies.
	s := store.NewLLMRateLimitMemoryStore()
	r, err := s.Reserve(context.Background(), store.ReserveReq{
		SlotID: "s1", RPM: 10, RPD: 10, TPMIn: 0, ReqID: "a", Tokens: 1_000_000,
	})
	mustOK(t, r, err)
}

func TestMemoryStore_RPDUnlimitedAllowsManyDays(t *testing.T) {
	// RPD=0 -> unlimited. Reservations must still succeed past arbitrary counts.
	s := store.NewLLMRateLimitMemoryStore()
	for i := 0; i < 50; i++ {
		r, err := s.Reserve(context.Background(), store.ReserveReq{
			SlotID: "s1", RPM: 100, RPD: 0, ReqID: fmt.Sprintf("r%d", i),
		})
		mustOK(t, r, err)
	}
}
