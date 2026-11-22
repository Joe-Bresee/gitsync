package apply

import (
	"os"
	"path/filepath"
	"testing"
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
