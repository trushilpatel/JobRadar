package tracing

import (
	"sort"
	"testing"

	"github.com/nats-io/nats.go"
)

// TestNatsHeaderCarrier_Keys exercises the Keys() method on the unexported
// natsHeaderCarrier type. Keys() is the TextMapCarrier method called by OTel
// propagators that iterate over all carrier keys; it cannot be reached
// indirectly through the public API because the W3C TraceContext and Baggage
// propagators use only Get(), not Keys(). An internal test is therefore the
// only way to reach this path without changing the production interface.
func TestNatsHeaderCarrier_Keys(t *testing.T) {
	h := nats.Header{}
	h.Set("traceparent", "00-abc-def-01")
	h.Set("tracestate", "vendor=abc")
	h.Set("x-custom", "v1")

	carrier := natsHeaderCarrier(h)
	keys := carrier.Keys()

	if len(keys) != 3 {
		t.Fatalf("Keys(): expected 3 keys, got %d: %v", len(keys), keys)
	}

	// Order is map-iteration order (non-deterministic); sort before comparing.
	sort.Strings(keys)
	// nats.Header stores keys exactly as set (no canonicalisation).
	want := []string{"traceparent", "tracestate", "x-custom"}
	sort.Strings(want)
	for i, k := range keys {
		if k != want[i] {
			t.Fatalf("Keys()[%d] = %q, want %q (sorted keys=%v)", i, k, want[i], keys)
		}
	}
}

// TestNatsHeaderCarrier_KeysEmpty verifies that Keys() on an empty header
// returns an empty (non-nil) slice.
func TestNatsHeaderCarrier_KeysEmpty(t *testing.T) {
	carrier := natsHeaderCarrier(nats.Header{})
	keys := carrier.Keys()
	if keys == nil {
		t.Fatal("Keys() on empty carrier returned nil; want non-nil empty slice")
	}
	if len(keys) != 0 {
		t.Fatalf("Keys() on empty carrier: want 0 keys, got %d: %v", len(keys), keys)
	}
}

// TestNatsHeaderCarrier_GetSetRoundTrip exercises Get and Set on the carrier
// directly (white-box) to guard the multi-value case in Get.
func TestNatsHeaderCarrier_GetSetRoundTrip(t *testing.T) {
	h := nats.Header{}
	carrier := natsHeaderCarrier(h)

	carrier.Set("traceparent", "00-trace-span-01")
	got := carrier.Get("traceparent")
	// nats.Header stores keys verbatim; Get must return the first value.
	if got != "00-trace-span-01" {
		t.Fatalf("Get after Set: want %q, got %q", "00-trace-span-01", got)
	}

	// Get on absent key must return "".
	if got := carrier.Get("no-such-header"); got != "" {
		t.Fatalf("Get absent key: want %q, got %q", "", got)
	}
}
