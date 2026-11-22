# GitSync TODO

**Updated:** 2026-11-22

This backlog is ordered by priority. Items are prioritized by production
correctness and safety first, then operability, then optional features.

## Current status

The MVP currently includes:

- Git source fetching with branch and revision fields
- YAML manifest parsing and target application
- Server-side apply support
- Reconciliation with source revision tracking
- ConfigSync status conditions
- Generated CRDs, RBAC, and deployment manifests
- Initial unit tests for source-path handling and target application
- CI workflows for linting, unit tests, and end-to-end tests

## P0 - correctness and safety blockers

These items should be completed before treating the operator as production
ready.

- [ ] **Fix target RBAC.** Grant only the resource and verb combinations the
  operator needs for every supported target type, including ConfigMaps and
  Secrets. Remove permissions that are broader than the implementation
  requires. Verify the generated manifests and an end-to-end sync.
- [x] **Make reconciliation repair drift.** Do not skip application solely
  because the Git SHA is unchanged. Re-apply when a managed target is changed
  or deleted, and when the ConfigSync target specification changes.
- [x] **Correct revision resolution.** Support the documented branch, tag, and
  commit-SHA forms instead of treating every revision as a full SHA. Add tests
  for each supported form and for invalid revisions.
- [x] **Isolate concurrent source checkouts.** Prevent ConfigSync objects that
  use the same repository with different branches or revisions from sharing
  and mutating one working tree.
- [ ] **Secure SSH host verification.** Replace
  `InsecureIgnoreHostKey()` with known-hosts or an equivalent configurable
  verification mechanism. Fail clearly when host verification cannot be
  established.
- [ ] **Make validation behavior explicit and consistent.** Decide whether
  server-side dry-run is enabled by default, wire that behavior through the
  controller, and document the setting. Add tests for both enabled and
  disabled modes.

## P1 - reliability and observability

- [x] **Make status conditions stable.** Update `LastTransitionTime` only when
  a condition's status or meaningful reason changes. Add focused tests.
- [x] **Preserve useful status data.** Store a repository-relative source path
  rather than a temporary container path, or remove the field if it cannot be
  useful to users.
- [x] **Harden temporary-directory lifecycle.** Guarantee cleanup on successful
  fetches, failed fetches, and reconciliation cancellation.
- [ ] **Improve failure handling.** Add typed, actionable errors; emit
  Kubernetes events for sync failures and recoveries; and retry transient Git
  and Kubernetes errors with bounded exponential backoff.
- [ ] **Complete test coverage.** Add unit tests for Git fetch and revision
  handling, manifest parsing, apply/update/delete behavior, status conditions,
  cleanup, and retry behavior. Add an envtest reconciliation test that
  verifies a real target resource is created and updated.
- [ ] **Make CI deterministic.** Ensure envtest assets are installed by the
  workflow, run generation checks, and keep unit, integration, and e2e tests
  clearly separated with useful failure output.

## P2 - planned user-facing capabilities

- [ ] **Add templating.** Implement a narrowly defined Go-template rendering
  stage with syntax validation, documented input data, escaping rules, and
  unit tests.
- [ ] **Add manifest validation.** Validate supported Kubernetes objects before
  apply, report the object and validation error in status, and test malformed
  and admission-rejected manifests.
- [ ] **Add rollback.** Allow an operator to select a previous or explicit
  Git revision, record deployment history, and expose rollback progress and
  failure in status.
- [ ] **Add environment support.** Define how branches, overlays, or
  environment values are selected without ambiguity, then add multi-environment
  tests.

## P3 - optional extensions

- [ ] **Pruning and garbage collection.** Track managed resources, add safe
  ownership/finalizer behavior, and provide an explicit opt-in before deleting
  resources no longer present in source.
- [ ] **Webhook-triggered reconciliation.** Add authenticated Git push
  triggers while retaining periodic reconciliation as a fallback.
- [ ] **Additional source transforms.** Evaluate Helm, Kustomize, and Jsonnet
  support separately; do not expand the MVP apply contract implicitly.
- [ ] **Encrypted configuration.** Evaluate SOPS/KMS integration with a clear
  key-management and Secret-handling design.
- [ ] **Multi-cluster synchronization.** Define cluster credential boundaries,
  failure isolation, and RBAC before implementation.
- [ ] **Release and documentation improvements.** Standardize the project name
  and API identity, add architecture and user documentation, publish versioned
  images, and add release automation.

## Definition of done for the next milestone

The next milestone is complete when:

1. RBAC matches supported target types and passes an end-to-end sync.
2. Drift and target-spec changes are repaired without requiring a new commit.
3. Branch, tag, and SHA revisions are tested.
4. SSH host verification is enabled.
5. `make test`, lint, generation checks, and the applicable CI workflows pass
   from a clean checkout.
