package encrypted

import (
	"strings"
	"testing"

	configsv1alpha1 "github.com/joe-bresee/gitsync/api/v1alpha1"
)

func TestMissingSOPSIsActionable(t *testing.T) {
	_, _, err := DecryptSource(".", &configsv1alpha1.EncryptedConfigSpec{Provider: "sops", Binary: "gitsync-sops-that-does-not-exist"})
	if err == nil {
		t.Fatal("expected missing binary error")
	}
	if !strings.Contains(err.Error(), "install sops") {
		t.Fatalf("expected actionable error, got %v", err)
	}
}
