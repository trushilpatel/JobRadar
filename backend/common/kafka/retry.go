package kafka

import (
	"strconv"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// RetrySchedule is the fixed tier sequence: retry_count N → (topic, delay).
// After the final tier fails, the message is sent to the DLQ.
type RetryTier struct {
	Topic string
	Delay time.Duration
}

// Name returns a short human identifier for the tier ("1m"/"10m"/"1h")
// based on Delay. Empty string for unknown delays.
func (t RetryTier) Name() string {
	switch t.Delay {
	case time.Minute:
		return "1m"
	case 10 * time.Minute:
		return "10m"
	case time.Hour:
		return "1h"
	default:
		return ""
	}
}

// FilterRetryTiers is the retry ladder for the filter worker (.raw).
// Index = retry_count value that will be written into the outgoing header.
// 0 → retry.1m (1st retry), 1 → retry.10m, 2 → retry.1h. After 2 → DLQ.
func FilterRetryTiers() []RetryTier {
	return []RetryTier{
		{Topic: TopicJobListingsRawRetry1m, Delay: 1 * time.Minute},
		{Topic: TopicJobListingsRawRetry10m, Delay: 10 * time.Minute},
		{Topic: TopicJobListingsRawRetry1h, Delay: 1 * time.Hour},
	}
}

// AnalyzerRetryTiers is the retry ladder for the analyzer worker (.filtered).
func AnalyzerRetryTiers() []RetryTier {
	return []RetryTier{
		{Topic: TopicJobListingsFilteredRetry1m, Delay: 1 * time.Minute},
		{Topic: TopicJobListingsFilteredRetry10m, Delay: 10 * time.Minute},
		{Topic: TopicJobListingsFilteredRetry1h, Delay: 1 * time.Hour},
	}
}

// RetryCountFromHeaders reads HeaderRetryCount from h; returns 0 if missing or unparseable.
func RetryCountFromHeaders(h []kgo.RecordHeader) int {
	for _, rh := range h {
		if rh.Key == HeaderRetryCount {
			if n, err := strconv.Atoi(string(rh.Value)); err == nil {
				return n
			}
			return 0
		}
	}
	return 0
}

// NextAttemptAtFromHeaders reads HeaderNextAttemptAt; returns zero time if missing or unparseable.
func NextAttemptAtFromHeaders(h []kgo.RecordHeader) time.Time {
	for _, rh := range h {
		if rh.Key == HeaderNextAttemptAt {
			if ms, err := strconv.ParseInt(string(rh.Value), 10, 64); err == nil {
				return time.UnixMilli(ms)
			}
			return time.Time{}
		}
	}
	return time.Time{}
}

// BuildRetryHeaders builds the set of headers for a retry message.
// retryCount is the new count (i.e. how many times this message has been retried, including this one).
func BuildRetryHeaders(retryCount int, nextAttemptAt time.Time) []kgo.RecordHeader {
	return []kgo.RecordHeader{
		{Key: HeaderRetryCount, Value: []byte(strconv.Itoa(retryCount))},
		{Key: HeaderNextAttemptAt, Value: []byte(strconv.FormatInt(nextAttemptAt.UnixMilli(), 10))},
	}
}

// BuildDLQHeaders builds the set of headers for a DLQ message.
// retryCount is the retry count at the time of exhaustion (used for observability).
func BuildDLQHeaders(retryCount int, lastError string) []kgo.RecordHeader {
	return []kgo.RecordHeader{
		{Key: HeaderRetryCount, Value: []byte(strconv.Itoa(retryCount))},
		{Key: HeaderLastError, Value: []byte(lastError)},
	}
}
