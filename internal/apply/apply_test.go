package apply

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	configsv1alpha1 "github.com/joe-bresee/gitsync/api/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestManifestFilesUsesSpecificFile(t *testing.T) {
	tempDir := t.TempDir()
	nestedDir := filepath.Join(tempDir, "config")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}

	rootManifest := filepath.Join(tempDir, "root.yaml")
	nestedManifest := filepath.Join(nestedDir, "app.yaml")
	if err := os.WriteFile(rootManifest, []byte("root"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nestedManifest, []byte("nested"), 0o600); err != nil {
		t.Fatal(err)
	}

	files, err := manifestFiles(nestedManifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != nestedManifest {
		t.Fatalf("expected only %q, got %v", nestedManifest, files)
	}
}

func TestManifestFilesListsYAMLFilesInDirectory(t *testing.T) {
	tempDir := t.TempDir()
	yamlFile := filepath.Join(tempDir, "app.yaml")
	ymlFile := filepath.Join(tempDir, "other.yml")
	hiddenFile := filepath.Join(tempDir, ".golangci.yml")
	if err := os.WriteFile(yamlFile, []byte("yaml"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ymlFile, []byte("yml"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hiddenFile, []byte("lint"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "ignored.txt"), []byte("text"), 0o600); err != nil {
		t.Fatal(err)
	}

	files, err := manifestFiles(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 YAML manifests, got %v", files)
	}
}

type recordingClient struct {
	client.Client
	patchErr error
	patches  int
}

func (c *recordingClient) Patch(_ context.Context, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
	c.patches++
	return c.patchErr
}

func TestApplyTargetRejectsMalformedManifest(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "invalid.yaml")
	if err := os.WriteFile(sourcePath, []byte("apiVersion: v1\nmetadata: ["), 0o600); err != nil {
		t.Fatal(err)
	}

	err := ApplyTarget(context.Background(), &recordingClient{}, sourcePath, configsv1alpha1.TargetRef{})
	if err == nil {
		t.Fatal("expected malformed manifest to be rejected")
	}
}

func TestApplyTargetStopsWhenDryRunFails(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "app.yaml")
	if err := os.WriteFile(sourcePath, []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: app
data:
  key: value
`), 0o600); err != nil {
		t.Fatal(err)
	}

	previous := DryRunEnabled
	DryRunEnabled = true
	t.Cleanup(func() { DryRunEnabled = previous })

	c := &recordingClient{patchErr: errors.New("rejected by server")}
	err := ApplyTarget(context.Background(), c, sourcePath, configsv1alpha1.TargetRef{})
	if err == nil {
		t.Fatal("expected dry-run rejection")
	}
	if c.patches != 1 {
		t.Fatalf("expected one dry-run patch and no real apply, got %d patches", c.patches)
	}
}

func TestApplyTargetValidatesSupportedObjectsBeforePatches(t *testing.T) {
	tests := []struct {
		name       string
		manifest   string
		wantReason string
	}{
		{
			name:       "missing metadata name",
			manifest:   "apiVersion: v1\nkind: ConfigMap\nmetadata: {}\n",
			wantReason: "metadata.name is required",
		},
		{
			name:       "unsupported kind",
			manifest:   "apiVersion: v1\nkind: Service\nmetadata:\n  name: app\n",
			wantReason: "unsupported kind \"Service\"",
		},
		{
			name: "deployment missing selector",
			manifest: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
spec:
  template:
    spec:
      containers:
        - name: app
          image: nginx
`,
			wantReason: "spec.selector is required",
		},
		{
			name: "deployment container missing image",
			manifest: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
spec:
  selector:
    matchLabels:
      app: app
  template:
    spec:
      containers:
        - name: app
`,
			wantReason: "requires",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourcePath := filepath.Join(t.TempDir(), "invalid.yaml")
			if err := os.WriteFile(sourcePath, []byte(tt.manifest), 0o600); err != nil {
				t.Fatal(err)
			}
			c := &recordingClient{}
			err := ApplyTarget(context.Background(), c, sourcePath, configsv1alpha1.TargetRef{})
			if err == nil {
				t.Fatal("expected manifest validation error")
			}
			var validationErr *ManifestValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("expected ManifestValidationError, got %T: %v", err, err)
			}
			if !strings.Contains(validationErr.Reason, tt.wantReason) {
				t.Fatalf("expected reason containing %q, got %q", tt.wantReason, validationErr.Reason)
			}
			if c.patches != 0 {
				t.Fatalf("validation should happen before patching; got %d patches", c.patches)
			}
		})
	}
}

func TestCleanObjectForApplyRemovesServerMetadata(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":              "app",
			"namespace":         "default",
			"uid":               "server-value",
			"resourceVersion":   "server-value",
			"managedFields":     []interface{}{"server-value"},
			"creationTimestamp": "server-value",
		},
		"status": map[string]interface{}{"ready": true},
	}}

	cleanObjectForApply(obj)

	metadata := obj.Object["metadata"].(map[string]interface{})
	if _, ok := metadata["uid"]; ok {
		t.Error("expected uid to be removed")
	}
	if _, ok := metadata["resourceVersion"]; ok {
		t.Error("expected resourceVersion to be removed")
	}
	if _, ok := obj.Object["status"]; ok {
		t.Error("expected status to be removed")
	}
}
