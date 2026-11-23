package encrypted

import (
	"os"
	"path/filepath"
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

func TestDecryptSourceMapsSecretDataToEnvironmentAndFiles(t *testing.T) {
	binaryDir := t.TempDir()
	binaryPath := filepath.Join(binaryDir, "fake-sops")
	script := `#!/bin/sh
cat <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: decrypted
data:
  aws: "${AWS_ACCESS_KEY_ID}"
  age-file: "$(cat "${SOPS_AGE_KEY_FILE}")"
EOF
`
	if err := os.WriteFile(binaryPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	sourcePath := filepath.Join(t.TempDir(), "encrypted.yaml")
	if err := os.WriteFile(sourcePath, []byte("encrypted: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	decryptedPath, cleanup, err := DecryptSource(sourcePath, &configsv1alpha1.EncryptedConfigSpec{
		Provider: "sops",
		Binary:   binaryPath,
	}, map[string][]byte{
		"AWS_ACCESS_KEY_ID": []byte("test-access-key"),
		"age-key.txt":       []byte("AGE-SECRET-KEY-test"),
	})
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	defer cleanup()

	decrypted, err := os.ReadFile(decryptedPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(decrypted)
	if !strings.Contains(text, `aws: "test-access-key"`) {
		t.Fatalf("AWS environment data was not passed to SOPS: %s", text)
	}
	if !strings.Contains(text, `age-file: "AGE-SECRET-KEY-test"`) {
		t.Fatalf("file credential was not passed to SOPS: %s", text)
	}
}

func TestLoadCredentialsRequiresExplicitMappings(t *testing.T) {
	spec := &configsv1alpha1.EncryptedConfigSpec{
		Provider: "sops",
		Env:      map[string]string{"SOPS_AGE_KEY": "age-key"},
		Files:    map[string]string{"GOOGLE_APPLICATION_CREDENTIALS": "service-account.json"},
	}
	_, err := LoadCredentials(spec, map[string][]byte{
		"age-key": []byte("AGE-SECRET-KEY-test"),
	})
	if err == nil {
		t.Fatal("expected missing explicit file mapping to fail")
	}
	if !strings.Contains(err.Error(), "service-account.json") {
		t.Fatalf("expected actionable missing key error, got %v", err)
	}
}

func TestLoadCredentialsCleanupAndPermissions(t *testing.T) {
	spec := &configsv1alpha1.EncryptedConfigSpec{
		Provider: "sops",
		Files:    map[string]string{"GOOGLE_APPLICATION_CREDENTIALS": "google.json"},
	}
	credentials, err := LoadCredentials(spec, map[string][]byte{
		"google.json": []byte(`{"type":"service_account"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer credentials.Cleanup()

	dirInfo, err := os.Stat(credentials.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("credential directory permissions = %o, want 700", dirInfo.Mode().Perm())
	}
	fileInfo, err := os.Stat(credentials.FilePaths["GOOGLE_APPLICATION_CREDENTIALS"])
	if err != nil {
		t.Fatal(err)
	}
	if fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("credential file permissions = %o, want 600", fileInfo.Mode().Perm())
	}

	credentials.Cleanup()
	if _, err := os.Stat(credentials.Dir); !os.IsNotExist(err) {
		t.Fatalf("credential directory still exists after cleanup: %v", err)
	}
}
