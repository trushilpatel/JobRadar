# GHCR + Flux CD GitOps Design

**Date:** 2026-05-23
**Status:** Planned — not yet implemented
**Branch target:** `worktree-fix+k8s-helm-optimization`

## Goal

Replace the local `make k8s-build` + `k3d image import` workflow with:

- GitHub Container Registry (GHCR) as the image registry
- Flux CD running in-cluster for GitOps reconciliation and drift detection
- GitHub Actions CD pipeline publishing images on every merge to `main`

---

## Architecture

### Image flow

```
merge to main
  → GHA cd.yml builds jobradar-{backend,crawler,console}
  → pushes to ghcr.io/trushilpatel/jobradar-{backend,crawler,console}:<sha> + :latest
  → Flux image-reflector-controller detects new tag
  → ImageUpdateAutomation commits tag update to values.yaml on a branch
  → PR auto-merged after CI passes → Flux HelmRelease reconciles → helm upgrade runs
```

### GHCR image names

| Service                                             | Image                                   |
| --------------------------------------------------- | --------------------------------------- |
| backend (api, filter, enricher, scheduler, migrate) | `ghcr.io/trushilpatel/jobradar-backend` |
| crawler                                             | `ghcr.io/trushilpatel/jobradar-crawler` |
| console                                             | `ghcr.io/trushilpatel/jobradar-console` |

Tags: short SHA (7 chars) matching the existing `GIT_SHA` Makefile convention, plus `latest`.

### Flux components

| Component                     | Purpose                                                   |
| ----------------------------- | --------------------------------------------------------- |
| `source-controller`           | Watches git repo, detects Helm chart changes              |
| `helm-controller`             | Reconciles `HelmRelease`, applies drift corrections       |
| `image-reflector-controller`  | Polls GHCR for new image tags                             |
| `image-automation-controller` | Commits tag updates to `values.yaml` when new tags appear |

---

## Files to Create

| File                                    | Purpose                                                                       |
| --------------------------------------- | ----------------------------------------------------------------------------- |
| `.github/workflows/cd.yml`              | Build + push all 3 images to GHCR on push to `main`                           |
| `k8s/flux/gotk-sync.yaml`               | `GitRepository` CRD — watches `trushilpatel/JobRadar` `main` branch           |
| `k8s/flux/helm-release.yaml`            | `HelmRelease` CRD — reconciles jobradar chart; replaces manual `helm upgrade` |
| `k8s/flux/image-repositories.yaml`      | Three `ImageRepository` CRDs — one per image, polls GHCR every 1m             |
| `k8s/flux/image-policies.yaml`          | Three `ImagePolicy` CRDs — regex `^[a-f0-9]{7}$` selects SHA tags             |
| `k8s/flux/image-update-automation.yaml` | `ImageUpdateAutomation` — commits tag updates to a branch (not main directly) |

## Files to Modify

| File                                          | Change                                                                                              |
| --------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| `k8s/helm/jobradar/values.yaml`               | Update image repositories to GHCR URLs; tag `latest`; pullPolicy `Always`; add Flux marker comments |
| `k8s/helm/jobradar/values.local.yaml.example` | New example file — documents local dev override back to `:local` images                             |
| `Makefile`                                    | Add `k8s-push-{backend,crawler,console}`, `k8s-push`, `flux-bootstrap` targets                      |
| `infra/CLAUDE.md`                             | Add GHCR workflow and Flux bootstrap sections                                                       |

---

## Local Dev Compatibility

`make k8s-build && make k8s-up` continues to work unchanged. The `values.local.yaml` (gitignored) overrides GHCR references back to locally-imported `:local` images:

```
k8s/helm/jobradar/values.local.yaml  ← gitignored, contains local image overrides
k8s/helm/jobradar/values.local.yaml.example  ← committed, documents the pattern
```

---

## Security Design

The following security controls are specified for the production implementation. Implement in priority order.

### Critical

**Private GHCR + imagePullSecret**

- Set all three GHCR packages to **private** via GitHub Settings → Packages
- Create `ghcr-pull-secret` in `jobradar-local` namespace using a PAT with `read:packages` scope only
- Add `imagePullSecrets` to all Deployment templates in the Helm chart
- For multi-node prod: bind pull secret to a service account, rotate via External Secrets Operator

**GHA OIDC keyless auth**

- Set `permissions.id-token: write` at the job level in `cd.yml`
- Use `GITHUB_TOKEN` (OIDC-backed) for GHCR login — no PAT stored as a secret
- Token auto-expires after the workflow run; no rotation required

### High

**GHA action SHA pinning**

- Pin every `uses:` action to its full commit SHA, not a version tag
- Add human-readable version comment next to each SHA
- Prevents supply chain attacks via compromised action tags

**Sealed Secrets for cluster secrets**

- Install `sealed-secrets` controller in `kube-system`
- Seal runtime secrets (`jobradar-api-secret`, `jobradar-filter-secret`, etc.) with `kubeseal`
- Commit sealed YAML files to `k8s/flux/secrets/` — safe to store in git, decryptable only by the cluster key
- Replaces the manual `kubectl create secret` steps in bootstrap

**`ImageUpdateAutomation` → PR branch (not direct main push)**

- Configure `spec.git.push.branch: flux/image-updates` in `ImageUpdateAutomation`
- Add a GHA workflow to auto-merge `flux/image-updates` PRs after lint passes
- Honors the project's no-direct-main-push policy; creates an audit trail in PR history

**Pod NetworkPolicies**

- Add a `default-deny-all` ingress + egress policy to `jobradar-local`
- Open egress per service: port 443 (NATS TLS + Supabase HTTPS) and port 6543 (Supabase DB)
- Prevents lateral movement if any pod is compromised

**Flux `--network-policy=true` bootstrap flag**

- Adds Flux's own network policies blocking inter-controller communication from outside `flux-system`

### Medium (future iteration)

**Cosign image signing + Flux verification**

- Add `cosign sign` step in `cd.yml` after push, using keyless Sigstore/Fulcio signing
- Add `verify.provider: cosign` to each `ImagePolicy` in Flux
- Flux rejects deployment of any unsigned or tampered image
- No private key to manage — signing identity is the GHA OIDC token

**Kyverno admission policies**

- Install Kyverno controller
- Enforce: no root pods, `readOnlyRootFilesystem` required, resource limits required
- Enforce: images must come from `ghcr.io/trushilpatel/*` only — blocks any rogue image reference in `jobradar-local`

---

## Flux Bootstrap (One-Time per Cluster)

```
make flux-bootstrap  ← prints the exact command, does not run it
```

The command (with `--network-policy=true` and `--components-extra` for image controllers) commits Flux CRDs to `k8s/flux/` and installs controllers in-cluster. Requires a `GITHUB_TOKEN` with `repo` scope.

After bootstrap, create the values secret:

```
kubectl create secret generic jobradar-helm-values \
  --from-file=values.yaml=k8s/helm/jobradar/values.local.yaml \
  -n jobradar-local
```

---

## Design Decisions

| Decision                       | Choice                                              | Rationale                                                       |
| ------------------------------ | --------------------------------------------------- | --------------------------------------------------------------- |
| Image tags                     | Short SHA (7 chars)                                 | Matches existing `GIT_SHA` convention; deterministic per commit |
| `ImageUpdateAutomation` target | Feature branch → PR, not direct main push           | Honors no-direct-main policy; provides audit trail              |
| Tag selection policy           | Regex `^[a-f0-9]{7}$`                               | No semver in this repo; SHA is the only stable pattern          |
| Values updates                 | Flux marker comments in `values.yaml`               | Idiomatic Flux; avoids extra overlay files                      |
| Secret management              | Sealed Secrets                                      | No external secrets backend; cluster self-contained             |
| GHCR auth in GHA               | OIDC + `GITHUB_TOKEN`                               | No PAT to store or rotate                                       |
| Local dev compatibility        | `values.local.yaml` override                        | Existing pattern; no breaking changes                           |
| Flux RBAC                      | `--network-policy=true` + scoped RBAC per namespace | Principle of least privilege                                    |
