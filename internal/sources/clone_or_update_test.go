package source

import (
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	configsv1alpha1 "github.com/joe-bresee/gitsync/api/v1alpha1"
)

func TestResolveEnvironmentUsesExplicitSettings(t *testing.T) {
	configSync := &configsv1alpha1.ConfigSync{
		Spec: configsv1alpha1.ConfigSyncSpec{
			Source: configsv1alpha1.SourceSpec{
				Git: &configsv1alpha1.GitSource{
					RepoURL:  "https://example.invalid/config.git",
					Path:     "config",
					Branch:   "main",
					Revision: "v1.0.0",
				},
			},
			Environment: &configsv1alpha1.EnvironmentSpec{
				Name:    "production",
				Branch:  "release",
				Overlay: "overlays/production",
				Data:    map[string]string{"region": "us-east"},
			},
		},
	}

	resolved, err := ResolveEnvironment(configSync)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Name != "production" || resolved.Overlay != "overlays/production" {
		t.Fatalf("unexpected environment resolution: %+v", resolved)
	}
	if resolved.Revision != "v1.0.0" || resolved.Branch != "" {
		t.Fatalf("revision must take precedence over branch: %+v", resolved)
	}
	if resolved.Data["region"] != "us-east" {
		t.Fatalf("environment data was not retained: %+v", resolved.Data)
	}
}

func TestResolveEnvironmentRejectsUnsafeOverlay(t *testing.T) {
	configSync := &configsv1alpha1.ConfigSync{
		Spec: configsv1alpha1.ConfigSyncSpec{
			Source: configsv1alpha1.SourceSpec{
				Git: &configsv1alpha1.GitSource{RepoURL: "https://example.invalid/config.git", Path: "config"},
			},
			Environment: &configsv1alpha1.EnvironmentSpec{Name: "bad", Overlay: "../outside"},
		},
	}

	_, err := ResolveEnvironment(configSync)
	if err == nil {
		t.Fatal("expected unsafe overlay to be rejected")
	}
}

func TestResolveRevisionSupportsBranchTagAndSHA(t *testing.T) {
	repoPath := t.TempDir()
	repo, err := git.PlainInit(repoPath, false)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(repoPath, "config.yaml"), []byte("data: one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	_, err = worktree.Add("config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	commitHash, err := worktree.Commit("initial", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.CreateTag("v1.0.0", commitHash, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, revision := range []string{"master", "v1.0.0", commitHash.String()} {
		hash, err := resolveRevision(repo, revision)
		if err != nil {
			t.Fatalf("resolve %s: %v", revision, err)
		}
		if commitHash != hash {
			t.Fatalf("resolve %s = %s, want %s", revision, hash, commitHash)
		}
	}

	_, err = resolveRevision(repo, "does-not-exist")
	if err == nil {
		t.Fatal("expected invalid revision to fail")
	}
}
