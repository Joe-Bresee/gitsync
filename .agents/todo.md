# GitSync TODO

**Updated:** 2026-11-22

Items are ordered by production correctness and safety, then operability, then
optional features.

## Completed

- [x] Git source fetching with branch, tag, and commit-SHA resolution
- [x] Isolated temporary checkouts with cleanup on success and failure
- [x] YAML manifest parsing and server-side apply
- [x] Reconciliation with periodic drift correction
- [x] Stable status condition transition timestamps
- [x] Repository-relative source path in status
- [x] ConfigMap and Secret target RBAC
- [x] Initial source, apply, and controller regression tests

## P0 - correctness and safety

- [x] **Secure SSH host verification.** Replace `InsecureIgnoreHostKey()`
  with known-hosts or equivalent configurable verification, and fail clearly
  when verification cannot be established.
- [x] **Make validation behavior explicit.** Server-side dry-run validation is
  enabled by default before every real apply; tests may disable it explicitly.
- [x] **Verify target RBAC end to end.** Confirm every supported target type
  and required verb works in a real cluster, while removing unnecessary
  permissions.

## P1 - reliability and observability

- [x] **Improve failure handling.** Add typed, actionable errors, Kubernetes
  events for failures and recoveries, and bounded exponential retry for
  transient Git and Kubernetes errors.
- [x] **Complete test coverage.** Add tests for malformed manifests, dry-run
  rejection, apply/update/delete behavior, cleanup, retries, and an envtest
  reconciliation that verifies a target is created and updated.
- [x] **Make CI deterministic.** Ensure envtest assets are installed by the
  workflow and generation checks run from a clean checkout.

## P2 - planned user-facing capabilities

- [x] **Add templating.** Implement a defined Go-template rendering stage with
  syntax validation, documented input data, escaping rules, and tests.
- [x] **Add manifest validation.** Validate supported Kubernetes objects before
  apply and report object-level errors in status.
- [x] **Add rollback.** Support previous or explicit Git revisions, deployment
  history, and rollback status.
- [x] **Add environment support.** Define branch, overlay, and environment
  value selection without ambiguity.

## P3 - optional extensions

- [x] **Pruning and garbage collection.** Track managed resources and require
  explicit opt-in before deleting resources absent from source.
- [x] **Webhook-triggered reconciliation.** Add authenticated Git push triggers
  while retaining periodic reconciliation.
- [x] **Additional transforms.** Evaluate Helm, Kustomize, and Jsonnet
  separately without expanding the MVP contract implicitly.
- [x] **Encrypted configuration.** Evaluate SOPS/KMS integration and key
  management.
- [x] **Multi-cluster synchronization.** Define credential boundaries and
  failure isolation before implementation.
- [ ] **Release and documentation improvements.** Standardize project identity,
  add architecture documentation, publish versioned images, and automate
  releases.

## Next milestone

complete/check completion of deterministic CI.
think about aws support eks/ google kuberenets: google $300 credit!
docs page? explaining product like a true OS proj?
live testing and analyses of project against a website w/ simulated traffic, downs, rollbacks, etc? grafana/prom?