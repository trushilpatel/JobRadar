package jetstream

import (
	"testing"
	"time"
)

// connect must retry on a failing dial until the deadline elapses, then return
// the last error — rather than giving up after the first attempt. This is what
// lets a worker wait for a NATS connection slot to free during a redeploy
// instead of exiting and crash-looping.
func TestConnect_RetriesUntilDeadline(t *testing.T) {
	maxWait := 120 * time.Millisecond
	retryWait := 20 * time.Millisecond

	start := time.Now()
	// Port 1 is reserved/unbindable, so every dial fails fast with "connection
	// refused" — exercising the retry loop without needing a live server.
	nc, js, err := connect(Config{URL: "nats://127.0.0.1:1"}, maxWait, retryWait)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error connecting to a dead address")
	}
	if nc != nil || js != nil {
		t.Fatalf("expected nil handles on failure, got nc=%v js=%v", nc, js)
	}
	if elapsed < maxWait {
		t.Fatalf("returned after %s, before the %s deadline — did not retry", elapsed, maxWait)
	}
}
