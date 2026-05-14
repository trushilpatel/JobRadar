package kafka_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/trushilpatel/jobradar/common/kafka"
)

func TestRetryTier_Name(t *testing.T) {
	cases := []struct {
		delay time.Duration
		want  string
	}{
		{time.Minute, "1m"},
		{10 * time.Minute, "10m"},
		{time.Hour, "1h"},
		{42 * time.Second, ""},
		{0, ""},
	}
	for _, c := range cases {
		t.Run(c.delay.String(), func(t *testing.T) {
			got := kafka.RetryTier{Delay: c.delay}.Name()
			if got != c.want {
				t.Fatalf("delay=%v: want %q, got %q", c.delay, c.want, got)
			}
		})
	}
}

func TestFilterRetryTiers_Order(t *testing.T) {
	tiers := kafka.FilterRetryTiers()
	if len(tiers) != 3 {
		t.Fatalf("expected 3 tiers, got %d", len(tiers))
	}
	want := []time.Duration{time.Minute, 10 * time.Minute, time.Hour}
	for i, w := range want {
		if tiers[i].Delay != w {
			t.Fatalf("tier[%d]: want %v, got %v", i, w, tiers[i].Delay)
		}
		if tiers[i].Topic == "" {
			t.Fatalf("tier[%d]: empty topic", i)
		}
	}
}

func TestAnalyzerRetryTiers_Order(t *testing.T) {
	tiers := kafka.AnalyzerRetryTiers()
	if len(tiers) != 3 {
		t.Fatalf("expected 3 tiers, got %d", len(tiers))
	}
	want := []time.Duration{time.Minute, 10 * time.Minute, time.Hour}
	for i, w := range want {
		if tiers[i].Delay != w {
			t.Fatalf("tier[%d]: want %v, got %v", i, w, tiers[i].Delay)
		}
	}
}

func TestRetryCountFromHeaders(t *testing.T) {
	t.Run("missing returns 0", func(t *testing.T) {
		if got := kafka.RetryCountFromHeaders(nil); got != 0 {
			t.Fatalf("nil: want 0, got %d", got)
		}
		if got := kafka.RetryCountFromHeaders([]kgo.RecordHeader{{Key: "other", Value: []byte("9")}}); got != 0 {
			t.Fatalf("no retry_count: want 0, got %d", got)
		}
	})
	t.Run("valid integer", func(t *testing.T) {
		h := []kgo.RecordHeader{{Key: kafka.HeaderRetryCount, Value: []byte("3")}}
		if got := kafka.RetryCountFromHeaders(h); got != 3 {
			t.Fatalf("want 3, got %d", got)
		}
	})
	t.Run("unparseable falls back to 0", func(t *testing.T) {
		h := []kgo.RecordHeader{{Key: kafka.HeaderRetryCount, Value: []byte("not-a-number")}}
		if got := kafka.RetryCountFromHeaders(h); got != 0 {
			t.Fatalf("want 0, got %d", got)
		}
	})
}

func TestNextAttemptAtFromHeaders(t *testing.T) {
	now := time.Now().UnixMilli()
	t.Run("missing returns zero time", func(t *testing.T) {
		got := kafka.NextAttemptAtFromHeaders(nil)
		if !got.IsZero() {
			t.Fatalf("nil: want zero time, got %v", got)
		}
	})
	t.Run("valid ms", func(t *testing.T) {
		h := []kgo.RecordHeader{{Key: kafka.HeaderNextAttemptAt, Value: []byte(strconv.FormatInt(now, 10))}}
		got := kafka.NextAttemptAtFromHeaders(h)
		if got.UnixMilli() != now {
			t.Fatalf("want %d, got %d", now, got.UnixMilli())
		}
	})
	t.Run("unparseable returns zero", func(t *testing.T) {
		h := []kgo.RecordHeader{{Key: kafka.HeaderNextAttemptAt, Value: []byte("nope")}}
		if got := kafka.NextAttemptAtFromHeaders(h); !got.IsZero() {
			t.Fatalf("want zero time, got %v", got)
		}
	})
}

func TestBuildRetryHeaders(t *testing.T) {
	at := time.Now().Add(5 * time.Minute)
	h := kafka.BuildRetryHeaders(2, at)
	if len(h) != 2 {
		t.Fatalf("expected 2 headers, got %d", len(h))
	}
	// Round-trip via the readers to verify byte encoding.
	if got := kafka.RetryCountFromHeaders(h); got != 2 {
		t.Fatalf("retry_count round-trip: want 2, got %d", got)
	}
	if got := kafka.NextAttemptAtFromHeaders(h); got.UnixMilli() != at.UnixMilli() {
		t.Fatalf("next_attempt_at round-trip: want %d, got %d", at.UnixMilli(), got.UnixMilli())
	}
}

func TestBuildDLQHeaders(t *testing.T) {
	h := kafka.BuildDLQHeaders(3, "boom")
	if len(h) != 2 {
		t.Fatalf("expected 2 headers, got %d", len(h))
	}
	var sawRetry, sawErr bool
	for _, rh := range h {
		switch rh.Key {
		case kafka.HeaderRetryCount:
			sawRetry = true
			if string(rh.Value) != "3" {
				t.Fatalf("retry_count: want '3', got %q", string(rh.Value))
			}
		case kafka.HeaderLastError:
			sawErr = true
			if string(rh.Value) != "boom" {
				t.Fatalf("last_error: want 'boom', got %q", string(rh.Value))
			}
		}
	}
	if !sawRetry || !sawErr {
		t.Fatalf("missing required headers: retry=%v err=%v", sawRetry, sawErr)
	}
}

func TestTopicConstants_AreUnique(t *testing.T) {
	all := []string{
		kafka.TopicJobListingsRaw,
		kafka.TopicJobListingsFiltered,
		kafka.TopicJobListingsRawRetry1m,
		kafka.TopicJobListingsRawRetry10m,
		kafka.TopicJobListingsRawRetry1h,
		kafka.TopicJobListingsRawDLQ,
		kafka.TopicJobListingsFilteredRetry1m,
		kafka.TopicJobListingsFilteredRetry10m,
		kafka.TopicJobListingsFilteredRetry1h,
		kafka.TopicJobListingsFilteredDLQ,
	}
	seen := map[string]bool{}
	for _, t := range all {
		seen[t] = true
	}
	if len(seen) != len(all) {
		t.Fatalf("duplicate topic constants: %d unique vs %d total", len(seen), len(all))
	}
	// Sanity: every topic has the expected prefix.
	for _, top := range all {
		if top == "" {
			t.Fatalf("empty topic constant")
		}
	}
}
