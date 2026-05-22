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

// TestMsgState records which disposition was called on a test message.
type TestMsgState struct {
	Acked  bool
	Naked  bool
	Termed bool
}

type testMsg struct {
	data          []byte
	deliveryCount uint64
	state         *TestMsgState
}

func (m *testMsg) Data() []byte                      { return m.data }
func (m *testMsg) Subject() string                   { return "test.subject" }
func (m *testMsg) Headers() nats.Header              { return nil }
func (m *testMsg) Reply() string                     { return "" }
func (m *testMsg) Ack() error                        { m.state.Acked = true; return nil }
func (m *testMsg) DoubleAck(_ context.Context) error { m.state.Acked = true; return nil }
func (m *testMsg) Nak() error                        { m.state.Naked = true; return nil }
func (m *testMsg) NakWithDelay(_ time.Duration) error {
	m.state.Naked = true
	return nil
}
func (m *testMsg) InProgress() error             { return nil }
func (m *testMsg) Term() error                   { m.state.Termed = true; return nil }
func (m *testMsg) TermWithReason(_ string) error { m.state.Termed = true; return nil }
func (m *testMsg) Metadata() (*natsjs.MsgMetadata, error) {
	return &natsjs.MsgMetadata{NumDelivered: m.deliveryCount}, nil
}
