package encrypted

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	configsv1alpha1 "github.com/joe-bresee/gitsync/api/v1alpha1"
	syncerrors "github.com/joe-bresee/gitsync/internal/errors"
)

// DecryptSource decrypts an individual SOPS file or all YAML files in a directory.
// Credentials are inherited from the operator environment or the key Secret is
// made available as environment variables by the caller.
func DecryptSource(sourcePath string, spec *configsv1alpha1.EncryptedConfigSpec) (string, func(), error) {
	if spec == nil {
		return sourcePath, func() {}, nil
	}
	if spec.Provider != "sops" {
		return "", func() {}, fmt.Errorf("unsupported encrypted config provider %q", spec.Provider)
	}
	binary := spec.Binary
	if binary == "" {
		binary = "sops"
	}
	path, err := exec.LookPath(binary)
	if err != nil {
		return "", func() {}, syncerrors.Wrap("decrypt", "sops", false,
			fmt.Errorf("SOPS is unavailable; install sops or set spec.encryptedConfig.binary"))
	}

	info, err := os.Stat(sourcePath)
	if err != nil {
		return "", func() {}, err
	}
	dir, err := os.MkdirTemp(".", ".gitsync-decrypted-*")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }

	files := []string{}
	if info.IsDir() {
		entries, err := os.ReadDir(sourcePath)
		if err != nil {
			cleanup()
			return "", func() {}, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if strings.HasSuffix(strings.ToLower(name), ".yaml") || strings.HasSuffix(strings.ToLower(name), ".yml") {
				files = append(files, filepath.Join(sourcePath, name))
			}
		}
	} else {
		files = append(files, sourcePath)
	}

	for _, file := range files {
		cmd := exec.Command(path, "--decrypt", file)
		output, err := cmd.Output()
		if err != nil {
			cleanup()
			if exitErr, ok := err.(*exec.ExitError); ok {
				return "", func() {}, syncerrors.Wrap("decrypt", file, false,
					fmt.Errorf("SOPS decryption failed: %s", strings.TrimSpace(string(exitErr.Stderr))))
			}
			return "", func() {}, syncerrors.Wrap("decrypt", file, false, err)
		}
		outputPath := filepath.Join(dir, filepath.Base(file))
		if err := os.WriteFile(outputPath, output, 0o600); err != nil {
			cleanup()
			return "", func() {}, err
		}
	}
	if len(files) == 1 && !info.IsDir() {
		return filepath.Join(dir, filepath.Base(files[0])), cleanup, nil
	}
	return dir, cleanup, nil
}
