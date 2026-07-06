// Command genspec regenerates the combined OpenAPI document served by the
// API (see apps/api/apihuma, apps/api/apispec) and writes it to
// apps/api/openapi.yaml. CI runs this and fails the build if the committed
// file is stale (git diff --exit-code) — the "generated/enforced contract
// over convention" guarantee from CLAUDE.md: the spec can never silently
// drift from what the API actually serves. See
// docs/superpowers/plans/2026-06-13-huma-migration-plan.md, Section 3.5.
package main

import (
	"fmt"
	"os"

	"github.com/trushilpatel/jobradar/apps/api/apispec"
)

// outputPath is relative to the backend/ module root, matching how `go run
// ./apps/genspec` is invoked from CI and Makefile targets.
const outputPath = "apps/api/openapi.yaml"

func main() {
	spec, err := apispec.Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "genspec: build spec: %v\n", err)
		os.Exit(1)
	}

	yamlBytes, err := spec.YAML()
	if err != nil {
		fmt.Fprintf(os.Stderr, "genspec: marshal spec: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(outputPath, yamlBytes, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "genspec: write %s: %v\n", outputPath, err)
		os.Exit(1)
	}
}
