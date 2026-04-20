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
