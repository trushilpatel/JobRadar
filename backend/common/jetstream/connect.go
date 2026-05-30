package jetstream

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	natsjs "github.com/nats-io/nats.go/jetstream"
)

// Initial-connection retry window. The Synadia Cloud account connection budget
// has little headroom over steady state, so during a redeploy a new pod can
// briefly find the account "full" until the old pod's slot is reaped. Retrying
// the initial connect — rather than exiting — lets the new pod wait for the
// slot instead of crash-looping (which only compounds the problem by growing
// the CrashLoopBackOff delay until it can no longer claim the freed slot in
// time). Bounded so a genuine misconfig (bad URL/creds) still surfaces as a
// crash rather than hanging forever.
const (
	connectMaxWait   = 90 * time.Second
	connectRetryWait = 2 * time.Second
)

// Connect opens a NATS connection and returns a JetStream context.
// Reconnects automatically forever once established — callers do not need retry
// logic. The initial connection is retried for up to connectMaxWait on any
// failure (e.g. the account connection limit during a redeploy).
func Connect(cfg Config) (*nats.Conn, natsjs.JetStream, error) {
	return connect(cfg, connectMaxWait, connectRetryWait)
}

func connect(cfg Config, maxWait, retryWait time.Duration) (*nats.Conn, natsjs.JetStream, error) {
	opts := []nats.Option{
		nats.Name("jobradar"),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2 * time.Second),
	}
	if cfg.CredsFile != "" {
		opts = append(opts, nats.UserCredentials(cfg.CredsFile))
	}

	deadline := time.Now().Add(maxWait)
	var lastErr error
	for {
		nc, err := nats.Connect(cfg.URL, opts...)
		if err == nil {
			js, jerr := natsjs.New(nc)
			if jerr != nil {
				_ = nc.Drain()
				return nil, nil, fmt.Errorf("jetstream init: %w", jerr)
			}
			return nc, js, nil
		}
		lastErr = err
		if time.Now().After(deadline) {
			return nil, nil, fmt.Errorf("nats connect (retried for %s): %w", maxWait, lastErr)
		}
		time.Sleep(retryWait)
	}
}

// ConnectManaged dials NATS, registers a context-cancel close goroutine, and
// returns only the JetStream handle. The *nats.Conn is managed internally.
func ConnectManaged(ctx context.Context, cfg Config) (natsjs.JetStream, error) {
	nc, js, err := Connect(cfg)
	if err != nil {
		return nil, err
	}
	go func() {
		<-ctx.Done()
		nc.Close()
	}()
	return js, nil
}
