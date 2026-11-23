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

type Credentials struct {
	Env       map[string]string
	FilePaths map[string]string
	Dir       string
}

func (c *Credentials) Cleanup() {
	if c != nil && c.Dir != "" {
		_ = os.RemoveAll(c.Dir)
	}
}

// LoadCredentials creates subprocess-only credentials from an explicitly
// declared mapping. It also applies a conservative set of conventional
// mappings for backward-compatible SecretRef usage.
func LoadCredentials(spec *configsv1alpha1.EncryptedConfigSpec, secretData map[string][]byte) (*Credentials, error) {
	credentials := &Credentials{
		Env:       make(map[string]string),
		FilePaths: make(map[string]string),
	}
	if spec == nil || len(secretData) == 0 {
		return credentials, nil
	}

	dir, err := os.MkdirTemp("", "gitsync-sops-credentials-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary credential directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("failed to secure temporary credential directory: %w", err)
	}
	credentials.Dir = dir

	for envName, secretKey := range spec.Env {
		if !validEnvName(envName) {
			credentials.Cleanup()
			return nil, fmt.Errorf("invalid environment variable name %q", envName)
		}
		value, ok := secretData[secretKey]
		if !ok {
			credentials.Cleanup()
			return nil, fmt.Errorf("SOPS key Secret is missing mapped key %q", secretKey)
		}
		credentials.Env[envName] = string(value)
	}

	for envName, secretKey := range spec.Files {
		if !validEnvName(envName) {
			credentials.Cleanup()
			return nil, fmt.Errorf("invalid environment variable name %q", envName)
		}
		value, ok := secretData[secretKey]
		if !ok {
			credentials.Cleanup()
			return nil, fmt.Errorf("SOPS key Secret is missing mapped file key %q", secretKey)
		}
		path, err := writeCredentialFile(dir, secretKey, value)
		if err != nil {
			credentials.Cleanup()
			return nil, err
		}
		credentials.FilePaths[envName] = path
	}

	if len(spec.Env) == 0 && len(spec.Files) == 0 {
		for _, envName := range []string{
			"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN",
			"AWS_REGION", "AWS_DEFAULT_REGION", "AZURE_CLIENT_ID",
			"AZURE_CLIENT_SECRET", "AZURE_TENANT_ID",
		} {
			if value, ok := secretData[envName]; ok {
				credentials.Env[envName] = string(value)
			}
		}
		if value, ok := secretData["SOPS_AGE_KEY"]; ok {
			credentials.Env["SOPS_AGE_KEY"] = string(value)
		}
		for _, secretKey := range []string{
			"SOPS_AGE_KEY_FILE", "age_key_file", "age-key.txt", "age.key", "age_key.txt",
		} {
			if value, ok := secretData[secretKey]; ok {
				path, err := writeCredentialFile(dir, secretKey, value)
				if err != nil {
					credentials.Cleanup()
					return nil, err
				}
				credentials.FilePaths["SOPS_AGE_KEY_FILE"] = path
				break
			}
		}
		for _, secretKey := range []string{
			"GOOGLE_APPLICATION_CREDENTIALS", "google-credentials.json",
			"google_application_credentials", "service-account.json", "credentials.json",
		} {
			if value, ok := secretData[secretKey]; ok {
				path, err := writeCredentialFile(dir, secretKey, value)
				if err != nil {
					credentials.Cleanup()
					return nil, err
				}
				credentials.FilePaths["GOOGLE_APPLICATION_CREDENTIALS"] = path
				break
			}
		}
	}
	return credentials, nil
}

func validEnvName(name string) bool {
	if name == "" {
		return false
	}
	for i, char := range name {
		if (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') ||
			(char == '_') || (i > 0 && char >= '0' && char <= '9') {
			continue
		}
		return false
	}
	return true
}

func writeCredentialFile(dir, secretKey string, value []byte) (string, error) {
	name := filepath.Base(secretKey)
	if name == "." || name == "" || name == string(filepath.Separator) {
		return "", fmt.Errorf("invalid SOPS credential Secret key %q", secretKey)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, value, 0o600); err != nil {
		return "", fmt.Errorf("failed to materialize SOPS credential key %q: %w", secretKey, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return "", fmt.Errorf("failed to secure SOPS credential key %q: %w", secretKey, err)
	}
	return path, nil
}

// DecryptSource decrypts an individual SOPS file or all YAML files in a directory.
// Credentials are inherited from the operator environment or the key Secret is
// made available as environment variables by the caller.
func DecryptSource(sourcePath string, spec *configsv1alpha1.EncryptedConfigSpec, secretData ...map[string][]byte) (string, func(), error) {
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
	if err := os.Chmod(dir, 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return "", func() {}, fmt.Errorf("failed to secure decrypted source directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }

	credentials := &Credentials{Env: make(map[string]string), FilePaths: make(map[string]string)}
	cleanupAll := func() {
		credentials.Cleanup()
		cleanup()
	}
	for _, data := range secretData {
		loaded, err := LoadCredentials(spec, data)
		if err != nil {
			cleanupAll()
			return "", func() {}, err
		}
		credentials.Cleanup()
		credentials = loaded
	}

	env := environmentMap(os.Environ())
	for name, value := range credentials.Env {
		env[name] = value
	}
	for name, path := range credentials.FilePaths {
		env[name] = path
	}
	if credentials.Dir != "" {
		env["SOPS_CREDENTIALS_DIR"] = credentials.Dir
	}
	cmdEnv := make([]string, 0, len(env))
	for name, value := range env {
		cmdEnv = append(cmdEnv, name+"="+value)
	}

	files := []string{}
	if info.IsDir() {
		entries, err := os.ReadDir(sourcePath)
		if err != nil {
			cleanupAll()
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
		cmd.Env = cmdEnv
		output, err := cmd.Output()
		if err != nil {
			cleanupAll()
			if exitErr, ok := err.(*exec.ExitError); ok {
				return "", func() {}, syncerrors.Wrap("decrypt", file, false,
					fmt.Errorf("SOPS decryption failed: %s", strings.TrimSpace(string(exitErr.Stderr))))
			}
			return "", func() {}, syncerrors.Wrap("decrypt", file, false, err)
		}
		outputPath := filepath.Join(dir, filepath.Base(file))
		if err := os.WriteFile(outputPath, output, 0o600); err != nil {
			cleanupAll()
			return "", func() {}, err
		}
	}
	// Credentials are needed only while SOPS processes are running. Release
	// them immediately after successful decryption, before transforms or apply.
	credentials.Cleanup()
	credentials.Dir = ""
	if len(files) == 1 && !info.IsDir() {
		return filepath.Join(dir, filepath.Base(files[0])), func() {
			cleanup()
		}, nil
	}
	return dir, cleanup, nil
}

func environmentMap(env []string) map[string]string {
	result := make(map[string]string)
	for _, entry := range env {
		name, value, found := strings.Cut(entry, "=")
		if found {
			result[name] = value
		}
	}
	return result
}
