package source

import (
	"testing"

	configsv1alpha1 "github.com/joe-bresee/gitsync/api/v1alpha1"
)

func TestResolveEnvironmentRollbackOverridesSourceRevision(t *testing.T) {
	configSync := &configsv1alpha1.ConfigSync{
		Spec: configsv1alpha1.ConfigSyncSpec{
			Source: configsv1alpha1.SourceSpec{Git: &configsv1alpha1.GitSource{
				Revision: "main",
				Branch:   "release",
			}},
			Rollback: &configsv1alpha1.RollbackSpec{Revision: "abc123"},
		},
	}

	resolved, err := ResolveEnvironment(configSync)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Revision != "abc123" {
		t.Fatalf("expected rollback revision, got %q", resolved.Revision)
	}
	if resolved.Branch != "" {
		t.Fatalf("expected branch to be cleared for pinned revision, got %q", resolved.Branch)
	}
}

func TestResolveEnvironmentRejectsEmptyRollbackRevision(t *testing.T) {
	configSync := &configsv1alpha1.ConfigSync{
		Spec: configsv1alpha1.ConfigSyncSpec{
			Source:   configsv1alpha1.SourceSpec{Git: &configsv1alpha1.GitSource{}},
			Rollback: &configsv1alpha1.RollbackSpec{},
		},
	}
	if _, err := ResolveEnvironment(configSync); err == nil {
		t.Fatal("expected empty rollback revision to be rejected")
	}
}
