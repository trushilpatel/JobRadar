package nodeidentity_test

import (
	"context"
	"testing"

	"github.com/trushilpatel/jobradar/apps/orchestrator/nodeidentity"
)

func TestNewContext_FromContext_RoundTrip(t *testing.T) {
	want := nodeidentity.NodeIdentity{ID: "node-1", Name: "prod-us-east"}
	ctx := nodeidentity.NewContext(context.Background(), want)
	got, ok := nodeidentity.FromContext(ctx)
	if !ok {
		t.Fatal("FromContext returned ok=false after NewContext")
	}
	if got != want {
		t.Errorf("FromContext = %+v, want %+v", got, want)
	}
}

func TestFromContext_ReturnsFalseOnEmptyContext(t *testing.T) {
	_, ok := nodeidentity.FromContext(context.Background())
	if ok {
		t.Error("FromContext returned ok=true on empty context")
	}
}

func TestIDFrom_ReturnsIDFromContext(t *testing.T) {
	ctx := nodeidentity.NewContext(context.Background(), nodeidentity.NodeIdentity{ID: "node-42"})
	if got := nodeidentity.IDFrom(ctx); got != "node-42" {
		t.Errorf("IDFrom = %q, want node-42", got)
	}
}

func TestIDFrom_ReturnsEmptyOnMissingContext(t *testing.T) {
	if got := nodeidentity.IDFrom(context.Background()); got != "" {
		t.Errorf("IDFrom(empty) = %q, want ''", got)
	}
}
