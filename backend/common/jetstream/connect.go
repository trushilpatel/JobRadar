package jetstream

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	natsjs "github.com/nats-io/nats.go/jetstream"
)

// Connect opens a NATS connection and returns a JetStream context.
// Reconnects automatically forever — callers do not need retry logic.
func Connect(cfg Config) (*nats.Conn, natsjs.JetStream, error) {
	opts := []nats.Option{
		nats.Name("jobradar"),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2 * time.Second),
	}
	if cfg.CredsFile != "" {
		opts = append(opts, nats.UserCredentials(cfg.CredsFile))
	}

	nc, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("nats connect: %w", err)
	}

	js, err := natsjs.New(nc)
	if err != nil {
		_ = nc.Drain()
		return nil, nil, fmt.Errorf("jetstream init: %w", err)
	}

	return nc, js, nil
}
