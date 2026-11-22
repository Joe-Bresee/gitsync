package source

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveGitSourcePathUsesRepositoryRelativeFile(t *testing.T) {
	repoPath := t.TempDir()
	nestedDir := filepath.Join(repoPath, "config")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(nestedDir, "app.yaml")
	if err := os.WriteFile(manifest, []byte("manifest"), 0o600); err != nil {
		t.Fatal(err)
	}

	resolved, err := resolveGitSourcePath(repoPath, "config/app.yaml")
	if err != nil {
		t.Fatal(err)
	}
	resolvedManifest, err := filepath.EvalSymlinks(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != resolvedManifest {
		t.Fatalf("expected %q, got %q", resolvedManifest, resolved)
	}
}

func TestResolveGitSourcePathRejectsTraversal(t *testing.T) {
	repoPath := t.TempDir()

	if _, err := resolveGitSourcePath(repoPath, "../outside.yaml"); err == nil {
		t.Fatal("expected traversal path to be rejected")
	}
}
