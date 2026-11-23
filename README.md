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
 - Before apply, manifests are validated locally for required Kubernetes object fields and supported kinds (`ConfigMap`, `Secret`, and `Deployment`). Invalid objects are reported in status with the source file, kind/name, and corrective reason.
 - Runtime validation: the operator performs a server-side dry-run before applying manifests to catch admission/validation errors. Tests can disable this behavior by setting `apply.DryRunEnabled = false`.
 - Template syntax validation occurs before the rendered output is applied.
 - SOPS can decrypt YAML source files using credentials from a referenced Kubernetes Secret or from credentials mounted in the operator environment.
 - Transient Git and Kubernetes API failures use bounded exponential retries and
   emit Kubernetes warning/normal events for failures and recovery.
 - `spec.rollback.revision` pins synchronization to a Git branch, tag, or commit
   SHA. The previous successful revision is retained in status.

## Features

### ✅ **Currently Implemented:**
- **Git Source Integration**: Clone and fetch from Git repositories with SSH/HTTPS authentication
- **Manifest Application**: Parse, validate, dry-run, and apply YAML manifests to Kubernetes resources
- **Status Management**: Track sync status, source revisions, environments, managed resources, and Kubernetes conditions
- **Reconciliation Loop**: Configurable refresh intervals with change detection via Git SHA comparison
- **Go Templates**: Render source manifests with environment, source, target, and custom values
- **Environment Selection**: Select Git branches and repository overlays and expose environment data to templates
- **Rollback**: Pin synchronization to an explicit Git revision and retain the previous successful revision
- **SOPS Decryption**: Decrypt YAML sources with explicit Secret-to-environment/file mappings and secure temporary credentials
- **Pruning**: Opt-in deletion of previously managed source resources
- **Manifest Transforms**: Optional Kustomize, Helm, and Jsonnet transform binaries
- **Webhook Reconciliation**: Authenticated Git push triggers alongside periodic reconciliation
- **Remote Clusters**: Named credential boundaries with kubeconfig Secrets and namespace restrictions
- **Failure Handling**: Actionable errors, Kubernetes events, and bounded retry backoff
- **Multi-Target Support**: Apply configuration to multiple Kubernetes resources from a single source
- **RBAC**: Proper role-based access controls for cluster operations

### 🚧 **Planned/In-Progress:**
- **Release & Distribution**: Standardized project identity, versioned images, and automated releases
- **Project Documentation**: Architecture guidance and a public documentation site
- **Operational Examples**: Live traffic, rollback, monitoring, and Grafana demonstrations
- **Expanded Cloud Validation**: AWS EKS and Google Kubernetes Engine testing

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
- Render templates and select environment-specific branches and overlays
- Decrypt SOPS YAML sources using Kubernetes Secret credentials
- Transform manifests, prune opt-in orphaned resources, and reconcile webhook changes
- Track synchronization status and handle transient failures

**Development Phase**: Reliability and distribution phase. The core reconciliation pipeline is implemented; remaining work is focused on broader integration testing, release automation, documentation, and cloud validation.

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
| `.Environment.Name` | resolved environment name |
| `.Environment.Branch` | resolved environment branch |
| `.Environment.Overlay` | resolved repository-relative overlay |
| `.Environment.Data` | environment-specific values |
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

## Environment selection

Use `spec.environment` to select an environment branch and repository overlay
explicitly:

```yaml
spec:
  environment:
    name: production
    branch: release
    overlay: overlays/production
    data:
      region: us-east
  source:
    git:
      repoURL: https://github.com/example/configs.git
      path: environments
  targets:
    - namespace: production
      name: app-config
      type: ConfigMap
```

The effective source path is `spec.source.git.path` plus `environment.overlay`.
Environment branch settings take precedence over `spec.source.git.branch`.
An explicit `spec.source.git.revision` (branch, tag, or commit SHA) always
takes precedence over a branch and is recorded in status.

Environment template input is available as `.Environment.Name`,
`.Environment.Branch`, `.Environment.Overlay`, and `.Environment.Data`.
Environment data is merged into `.Data` and `.Values`; environment values
override same-named template values.

## SOPS decryption

Set `spec.encryptedConfig.provider` to `sops` to decrypt YAML source files
before transforms, template rendering, or manifest apply:

```yaml
spec:
  encryptedConfig:
    provider: sops
    binary: sops # optional
    keySecretRef:
      namespace: default
      name: sops-keys
    env:
      SOPS_AGE_KEY: age-key
    files:
      GOOGLE_APPLICATION_CREDENTIALS: google-credentials.json
```

The operator reads the referenced Secret for each reconciliation. Every Secret
data key is only copied when selected by `env` or `files`; `env` maps an
environment variable name to a Secret key, and `files` maps an environment
variable to a Secret key whose value is written as a temporary `0600` file.
For existing resources with no explicit mappings, common provider conventions
are mapped conservatively:

- `GOOGLE_APPLICATION_CREDENTIALS`, `google_application_credentials`,
  `service-account.json`, or `credentials.json`
- `AWS_SHARED_CREDENTIALS_FILE` or `aws_shared_credentials`
- `AWS_CONFIG_FILE` or `aws_config`
- `SOPS_AGE_KEY_FILE`, `age_key_file`, `age-key.txt`, `age.key`, or `age_key.txt`

Secret values are held only for the reconciliation process and temporary
credential files are deleted after SOPS exits. Changes to the key Secret trigger
reconciliation.

## Known Issues & Limitations

1. **Testing Infrastructure**: The full envtest suite requires setup assets. Run `make setup-envtest` before `make test`.
2. **SOPS Binary**: SOPS must be installed in the operator image or configured with an explicit executable path.
3. **External Providers**: PGP keyrings and provider-specific credentials must be mounted or configured in the operator environment unless supplied explicitly through a Secret.
