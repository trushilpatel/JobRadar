package jetstream

// Config holds NATS connection parameters.
// CredsFile is optional — leave empty for unauthenticated local NATS.
type Config struct {
	URL       string // e.g. "nats://nats:4222" or Synadia Cloud URL
	CredsFile string // path to .creds file; empty = no auth (local dev)
}
