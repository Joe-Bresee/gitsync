# Project Guidelines

## Project Overview

This repository contains a Go Kubernetes operator built with Kubebuilder and
controller-runtime. The operator watches `ConfigSync` resources, fetches
configuration from Git, and applies Kubernetes resources.

## Code and Repository Conventions

- Use the Go version declared in `go.mod`.
- Keep API types under `api/`, controller code under `internal/controller/`,
  source and application logic under `internal/`, and end-to-end tests under
  `test/e2e/`.
- Run `gofmt` through `make fmt` after Go changes.
- Regenerate CRDs and generated Go code with `make manifests generate` after
  changing API types or Kubebuilder markers.
- Do not commit credentials, kubeconfig files, generated local binaries, or
  other environment-specific artifacts.

## Build and Test

- Run the standard validation suite with `make test`. This runs formatting,
  `go vet`, envtest setup, and non-e2e Go tests with coverage.
- Run end-to-end tests with `make test-e2e`; these require Docker and Kind.
- Run lint checks with `make lint`.
- Keep tests close to the package they exercise and use the existing
  Ginkgo/Gomega style for controller and e2e tests.

## Agent Workspace

`.agents/` is intentionally ignored by Git and is reserved for local,
task-specific requirements, plans, and task lists. Do not rely on files in
that directory as committed project documentation.

