package source

import (
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

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
