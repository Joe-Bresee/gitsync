# gitsync

A Kubernetes operator that synchronizes configuration from a Git source into target resources on a cluster.

[![Watch a quick demo](https://youtu.be/75jLiJl56Ro/maxresdefault.jpg)](https://youtu.be/75jLiJl56Ro)


## Motivation

During my internship, I worked extensively with Kubernetes: deploying Helm charts, routing Ingress, debugging Pods, and creating cloud-native workflows in Argo Workflows. I found it fascinating and really wanted to explore Kubernetes internals further.

Creating an operator seemed like a perfect way to deepen my understanding while building something practical. Inspired by ArgoCD's GitOps strategy, this project provides a lightweight alternative for smaller clusters, useful for local development on `kind` or testing environments.

This project allows me to:

- Work with Git repositories and practice Go (which I've been learning in my other repo gophercises) (via [Gophercises](https://gophercises.com))  
- Learn Kubernetes concepts like CRDs, controllers, reconcilers, and status conditions  
 
MVP notes
 - This operator applies raw manifests or Go-template-rendered manifests from the configured source to the cluster.
 - Runtime validation: the operator performs a server-side dry-run before applying manifests to catch admission/validation errors. Tests can disable this behavior by setting `apply.DryRunEnabled = false`.
 - Template syntax validation occurs before the rendered output is applied.

## Features

### ✅ **Currently Implemented:**
- **Git Source Integration**: Clone and fetch from Git repositories with SSH/HTTPS authentication
- **Manifest Application**: Parse and apply YAML manifests to Kubernetes resources
- **Status Management**: Track sync status with proper Kubernetes conditions (`Degraded`)
- **Reconciliation Loop**: Configurable refresh intervals with change detection via Git SHA comparison
- **Multi-Target Support**: Apply configuration to multiple Kubernetes resources from a single source
- **RBAC**: Proper role-based access controls for cluster operations

### 🚧 **Planned/In-Progress:**
- **Enhanced Validation**: Comprehensive YAML/manifest validation before application  
- **Testing Suite**: Unit tests and integration tests with envtest
- **Rollback Support**: Revert to previous Git commits
- **Pruning & Garbage Collection**: Clean up orphaned resources
- **Multi-Environment Support**: Branch/environment-specific configurations

 ## Technologies

 - Language: Go (module-based project)
 - Operator framework: Kubebuilder / controller-runtime
 - Git library: `go-git` for cloning and fetching repositories
 - YAML parsing: `sigs.k8s.io/yaml`
 - Build & tooling: `Makefile`, `go` toolchain, `kustomize` for manifests
 - Local testing: `kind` for local Kubernetes clusters, Docker for images


## Project Status

**Current State**: MVP is functional with core GitOps capabilities. The operator can successfully:
- Clone Git repositories and detect changes
- Apply Kubernetes manifests to cluster resources
- Track synchronization status and handle failures

**Development Phase**: Testing and enhancement phase - core functionality works but needs comprehensive testing and additional features.

## Quickstart

### Prerequisites
Ensure you have the following tools installed:
- Go 1.21+
- Docker
- kubectl 
- kind (for local testing)
- kubebuilder (for development)

### Quick Setup

1. **Create a Kind cluster:**
```bash
kind create cluster --name gitsync
```

2. **Install CRDs and deploy the operator:**
```bash
# Install the gitsync CRD
make install

# Run the controller locally (recommended for development)
make run
```

**OR** build and deploy as container:

```bash
# Create/load/build/install/deploy in one idempotent command
make deploy-kind IMG=controller:latest KIND_LOAD_CLUSTER=gitsync
```

Remove the operator and CRDs while keeping the Kind cluster:

```bash
make undeploy-kind KIND_LOAD_CLUSTER=gitsync
```

Remove the operator and then delete the entire Kind cluster:

```bash
make undeploy-kind KIND_LOAD_CLUSTER=gitsync KIND_DELETE_CLUSTER=true
```

3. **Test with a sample ConfigSync:**
```bash
kubectl apply -f config/samples/configs_v1alpha1_configsync.yaml
```

### Development Commands
```bash
# Generate CRDs and code after API changes
make manifests generate

# Build the project
make build

# Run tests (requires envtest binaries - see Known Issues)
make test

# View available targets
make help
```

## Go-template rendering

Set `spec.template` to render YAML source files with Go's `text/template`
package before server-side apply:

```yaml
spec:
  template:
    data:
      environment: staging
  source:
    git:
      repoURL: https://github.com/example/configs.git
      path: config/app.yaml
  targets:
    - namespace: staging
      name: app-config
      type: ConfigMap
```

The source may be a YAML file or a directory containing `.yaml` and `.yml`
files. Before rendering, every source YAML template is parsed for syntax
errors. Rendering and YAML parsing of the result happen before dry-run apply.

Template input is:

| Field | Value |
| --- | --- |
| `.ConfigSync.Name` | ConfigSync resource name |
| `.ConfigSync.Namespace` | ConfigSync resource namespace |
| `.Source.Path` | configured Git source path |
| `.Source.Revision` | resolved Git revision SHA |
| `.Revision` | resolved Git revision SHA (shorthand) |
| `.Target.Name` | current target name |
| `.Target.Namespace` | current target namespace |
| `.Target.Type` | current target type |
| `.Data` / `.Values` | user values from `spec.template.data` |

For example:

```yaml
metadata:
  name: {{ .Target.Name }}
  namespace: {{ .Target.Namespace }}
data:
  environment: {{ printf "%q" .Data.environment }}
  revision: {{ printf "%q" .Revision }}
```

Escaping: Go-template rendering uses `text/template`, not `html/template`.
It does not automatically escape HTML, JSON, or YAML. Quote string values
with `printf "%q"` and use appropriate template functions or explicit encoding
before inserting values into YAML. Missing `.Data`/`.Values` keys fail rendering
rather than silently producing empty values.

## Known Issues & Limitations

1. **Testing Infrastructure**: Tests require envtest binaries that aren't currently installed. Run `make envtest` to install them.
2. **Templating**: Go template support is planned but not yet implemented.
3. **Rollback**: Rollback to previous Git commits is not yet implemented.
4. **Multi-branch**: Environment-specific branch support is planned.
