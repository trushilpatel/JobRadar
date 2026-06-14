package jetstream

import (
	"context"
	"time"

	nats "github.com/nats-io/nats.go"
	natsjs "github.com/nats-io/nats.go/jetstream"
)

// NewTestMsg creates a Msg backed by a fake for unit tests outside this package.
// The returned TestMsgState can be inspected after the test to verify Ack/Nak/Term.
func NewTestMsg(data []byte, deliveryCount int) (*Msg, *TestMsgState) {
	state := &TestMsgState{}
	inner := &testMsg{data: data, deliveryCount: uint64(deliveryCount), state: state}
	return &Msg{inner: inner}, state
}

// NewTestMsgWithHeaders creates a Msg with the given NATS headers set.
// Use this to test code that reads message headers (e.g. X-Retry-Count).
// The returned TestMsgState can be inspected after the test to verify Ack/Nak/Term.
func NewTestMsgWithHeaders(data []byte, deliveryCount int, headers map[string]string) (*Msg, *TestMsgState) {
	state := &TestMsgState{}
	h := make(nats.Header, len(headers))
	for k, v := range headers {
		h.Set(k, v)
	}
	inner := &testMsg{data: data, deliveryCount: uint64(deliveryCount), headers: h, state: state}
	return &Msg{inner: inner}, state
}

// TestMsgState records which disposition was called on a test message.
// NakErr, when set, is returned by Nak() so tests can exercise the Nak-failure
// path; the Naked flag is still set so the call is observable.
// TermErr, when set, is returned by Term() so tests can exercise the Term-failure
// path; the Termed flag is still set so the call is observable.
// AckErr, when set, is returned by Ack() so tests can exercise the Ack-failure
// path; the Acked flag is still set so the call is observable.
type TestMsgState struct {
	Acked   bool
	Naked   bool
	Termed  bool
	AckErr  error
	NakErr  error
	TermErr error
}

type testMsg struct {
	data          []byte
	deliveryCount uint64
	headers       nats.Header
	state         *TestMsgState
}

func (m *testMsg) Data() []byte                      { return m.data }
func (m *testMsg) Subject() string                   { return "test.subject" }
func (m *testMsg) Headers() nats.Header              { return m.headers }
func (m *testMsg) Reply() string                     { return "" }
func (m *testMsg) Ack() error                        { m.state.Acked = true; return m.state.AckErr }
func (m *testMsg) DoubleAck(_ context.Context) error { m.state.Acked = true; return m.state.AckErr }
func (m *testMsg) Nak() error                        { m.state.Naked = true; return m.state.NakErr }
func (m *testMsg) NakWithDelay(_ time.Duration) error {
	m.state.Naked = true
	return m.state.NakErr
}
func (m *testMsg) InProgress() error             { return nil }
func (m *testMsg) Term() error                   { m.state.Termed = true; return m.state.TermErr }
func (m *testMsg) TermWithReason(_ string) error { m.state.Termed = true; return m.state.TermErr }
func (m *testMsg) Metadata() (*natsjs.MsgMetadata, error) {
	return &natsjs.MsgMetadata{NumDelivered: m.deliveryCount}, nil
}
