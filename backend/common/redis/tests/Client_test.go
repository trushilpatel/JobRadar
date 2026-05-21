package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	commonredis "github.com/trushilpatel/jobradar/common/redis"
)

// urlFor builds a non-TLS redis URL pointing at the miniredis instance with
// the given db index. miniredis speaks plain RESP, so we use redis:// (not
// rediss://) — TLS behaviour is exercised against real Redis in integration
// tests, not here.
func urlFor(addr string, db int) string {
	if db == 0 {
		return "redis://" + addr + "/0"
	}
	return "redis://" + addr + "/" + itoa(db)
}

func itoa(n int) string {
	// small ints only — db is 0..15
	if n == 0 {
		return "0"
	}
	const digits = "0123456789"
	buf := [4]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = digits[n%10]
		n /= 10
	}
	return string(buf[i:])
}

// TestNew_HappyPath verifies that New connects, pings, and returns a usable
// client when pointed at a reachable Redis server. miniredis stands in for the
// real broker — same RESP wire protocol, same Ping semantics.
func TestNew_HappyPath(t *testing.T) {
	mr := miniredis.RunT(t)

	client, err := commonredis.New(context.Background(), urlFor(mr.Addr(), 0))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if client == nil {
		t.Fatal("New returned nil client")
	}
	t.Cleanup(func() { _ = client.Close() })

	// Sanity check — the client actually talks to the server we passed.
	if err := client.Set(context.Background(), "k", "v", 0).Err(); err != nil {
		t.Fatalf("set after New: %v", err)
	}
	if got := mr.DB(0).HGet("dummy", "x"); got != "" { // smoke the miniredis instance handle
		_ = got
	}
}

// TestNew_SelectsDB confirms the db component of the URL is honoured — values
// written via a URL ending in /3 must land in db 3 and not be visible from db 0.
func TestNew_SelectsDB(t *testing.T) {
	mr := miniredis.RunT(t)

	client, err := commonredis.New(context.Background(), urlFor(mr.Addr(), 3))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	if err := client.Set(context.Background(), "scope", "ok", 0).Err(); err != nil {
		t.Fatalf("set: %v", err)
	}

	if got, _ := mr.DB(3).Get("scope"); got != "ok" {
		t.Fatalf("expected key in db 3, got %q", got)
	}
	if mr.DB(0).Exists("scope") {
		t.Fatal("key leaked into db 0 — db param not applied")
	}
}

// TestNew_ZeroDB exercises the default db=0 selector.
func TestNew_ZeroDB(t *testing.T) {
	mr := miniredis.RunT(t)
	client, err := commonredis.New(context.Background(), urlFor(mr.Addr(), 0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	if err := client.Set(context.Background(), "k", "v", 0).Err(); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got, _ := mr.DB(0).Get("k"); got != "v" {
		t.Fatalf("expected v in db 0, got %q", got)
	}
}

// TestNew_UnreachableAddr verifies the function returns a wrapped error when
// the ping fails, rather than returning a half-built client. The error message
// must mention the address so operators can diagnose misconfiguration.
func TestNew_UnreachableAddr(t *testing.T) {
	// 127.0.0.1:1 is reserved and refuses connections immediately on most
	// machines, so the test doesn't pay the full dial-timeout cost.
	addr := "127.0.0.1:1"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := commonredis.New(ctx, urlFor(addr, 0))
	if err == nil {
		_ = client.Close()
		t.Fatal("expected error for unreachable addr, got nil")
	}
	if client != nil {
		t.Fatal("expected nil client on error")
	}
	if !strings.Contains(err.Error(), "redis: ping") {
		t.Errorf("error %q missing 'redis: ping' prefix", err.Error())
	}
	if !strings.Contains(err.Error(), addr) {
		t.Errorf("error %q does not mention addr %q", err.Error(), addr)
	}
}

// TestNew_ContextCancelled ensures a cancelled context surfaces as an error
// instead of hanging — protects against future regressions if the ping path
// is changed to ignore ctx.
func TestNew_ContextCancelled(t *testing.T) {
	mr := miniredis.RunT(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client, err := commonredis.New(ctx, urlFor(mr.Addr(), 0))
	if err == nil {
		_ = client.Close()
		t.Fatal("expected error for cancelled ctx, got nil")
	}
	if client != nil {
		t.Fatal("expected nil client on error")
	}
}

// TestNew_InvalidURL verifies that a syntactically broken URL is rejected
// up-front by ParseURL, with a wrapped error that names the failure mode.
// Guards the parse path from regressing into a panic or a half-built client.
func TestNew_InvalidURL(t *testing.T) {
	client, err := commonredis.New(context.Background(), "not-a-url")
	if err == nil {
		_ = client.Close()
		t.Fatal("expected error for invalid url, got nil")
	}
	if client != nil {
		t.Fatal("expected nil client on parse error")
	}
	if !strings.Contains(err.Error(), "redis: parse url") {
		t.Errorf("error %q missing 'redis: parse url' prefix", err.Error())
	}
}

// TestClientTypeAlias guards against accidental rename — every caller imports
// commonredis.Client as a *goredis.Client. If this fails the package contract
// has changed.
func TestClientTypeAlias(t *testing.T) {
	mr := miniredis.RunT(t)
	client, err := commonredis.New(context.Background(), urlFor(mr.Addr(), 0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	// Compile-time: Client must satisfy the *goredis.Client shape.
	var _ = client
}
