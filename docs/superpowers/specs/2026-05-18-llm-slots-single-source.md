# LLM Slots — Single Source via Helm `.Files.Get` — 2026-05-18

Trigger: User wanted the new Gemma model `gemma-4-26b-a4b-it` added as an extra enricher slot on both `GEMINI_KEY` and `GEMINI_KEY_JP`, then asked whether the slots YAML could be consumed directly by Helm instead of being duplicated between the local file and the chart template.

## What changed (file-level only)

### Slot definitions

- [k8s/helm/jobradar/files/llm-slots.yaml](../../../k8s/helm/jobradar/files/llm-slots.yaml) — new canonical location for the slots config. `git mv`d from `backend/common/config/llm-slots.yaml`. Filter has 2 slots (`gemini-3.1-flash-lite-preview` × `GEMINI_KEY`, × `GEMINI_KEY_JP`). Enricher has 4 slots: `gemma-4-31b-it` × both keys and `gemma-4-26b-a4b-it` × both keys.

### Helm chart

- [k8s/helm/jobradar/templates/llm-slots-configmap.yaml](../../../k8s/helm/jobradar/templates/llm-slots-configmap.yaml) — replaced the per-slot `Values`-driven template with a single `{{ .Files.Get "files/llm-slots.yaml" | indent 4 }}` directive. Helm now embeds the YAML byte-for-byte into the ConfigMap; numbers stay as plain integers (no more `1e+06` float serialization round-trip).
- [k8s/helm/jobradar/values.yaml](../../../k8s/helm/jobradar/values.yaml) — removed the entire `llmSlots:` block (16 dead keys: `filterKeyId`, `filterModel`, `filterRpm`, `enricherKeyId`, `enricherKeyId2`, `enricherModel2`, etc.). The slot file is now the truth; values.yaml no longer participates.

### Local dev / docker-compose

- [docker-compose.yml](../../../docker-compose.yml) — `filter` and `enricher` service volume mounts re-pointed from `./backend/common/config:/app/config:ro` to `./k8s/helm/jobradar/files:/app/config:ro`. The in-container `LLM_SLOTS_CONFIG=/app/config/llm-slots.yaml` env var is unchanged.
- [backend/.env.example](../../../backend/.env.example) — `LLM_SLOTS_CONFIG` example value updated to `../k8s/helm/jobradar/files/llm-slots.yaml` (relative path from `backend/` for local `go run`).

### Documentation

- [README.md](../../../README.md) — section heading + body now point at the new path; removed the stale inline example block (had outdated `gemini-2.0-flash` models). Replaced with a one-line description of the `.Files.Get` pipeline.
- [docs/REFERENCE.md](../../../docs/REFERENCE.md) — config-file link updated; rate-limits table gained a third row for `gemma-4-26b-a4b-it`; also corrected the env-ref list to include `${GEMINI_KEY_JP}`.
- [backend/common/config/CLAUDE.md](../../../backend/common/config/CLAUDE.md) — "Current slots file" link rewritten to point at the chart location and explain the new flow.

## Architecture rules — enforce in code review

1. **Slot config is single-source.** Edit `k8s/helm/jobradar/files/llm-slots.yaml`. Do not reintroduce a parallel file in `backend/common/config/`. Both docker-compose and Helm read from the same path.
2. **Helm reads the file via `.Files.Get`, not via `Values`.** Don't migrate slot definitions back into `values.yaml` — the byte-faithful render is intentional (it preserves integer literals and prevents drift).
3. **API keys remain env-references (`${VAR}`).** The runtime `LoadLLMSlotsYAML` expander resolves them at process startup, never at Helm template time. Never bake secrets into the file.
4. **Adding a slot is a single-file edit.** No `values.yaml` change, no chart-template change. Validate locally with `helm template jobradar k8s/helm/jobradar` before deploying.
5. **`LLM_SLOTS_CONFIG` must point at a real file on the running container.** For pods, mounted at `/etc/jobradar/llm-slots.yaml`. For docker-compose, mounted at `/app/config/llm-slots.yaml`. For local `go run`, set explicitly via `.env`.

## What was _not_ changed

- The Go loader (`LoadLLMSlotsYAML` in `backend/common/config/config.go`) — same code paths, same env-var expansion semantics, same fallback to `FILTER_LLM_SLOTS` / `ENRICHER_LLM_SLOTS` JSON env vars.
- Filter slots — only the enricher gained the new `gemma-4-26b-a4b-it` model. Filter relies on structured JSON output, which Gemma models don't handle as cleanly as the `gemini-*` family.
- Cluster secrets — both `jobradar-filter-secret` and `jobradar-enricher-secret` already referenced `GEMINI_KEY_JP` in earlier work; this change does not touch secret material.

## Verification

- `helm lint k8s/helm/jobradar` clean.
- `helm template` renders the ConfigMap byte-faithful to the source file; `1000000` stays as `1000000`.
- `go test ./common/config/... -run LLMSlots` passes.
- Grep confirms no stale `backend/common/config/llm-slots.yaml` references remain in tracked sources.
