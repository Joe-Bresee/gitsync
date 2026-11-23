package transforms

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	configsv1alpha1 "github.com/joe-bresee/gitsync/api/v1alpha1"
	syncerrors "github.com/joe-bresee/gitsync/internal/errors"
)

// TransformSource runs an explicitly selected external transform. External
// binaries are optional: a missing binary returns an actionable error.
func TransformSource(sourcePath string, spec *configsv1alpha1.TransformSpec) (string, func(), error) {
	if spec == nil {
		return sourcePath, func() {}, nil
	}

	binary := spec.Binary
	if binary == "" {
		binary = spec.Type
	}
	path, err := exec.LookPath(binary)
	if err != nil {
		return "", func() {}, syncerrors.Wrap("transform", spec.Type, false,
			fmt.Errorf("external transform %q is unavailable; install %s or set spec.transform.binary", spec.Type, spec.Type))
	}

	args := append([]string(nil), spec.Args...)
	switch spec.Type {
	case "kustomize":
		args = append([]string{"build", sourcePath}, args...)
	case "helm":
		release := spec.Release
		if release == "" {
			return "", func() {}, fmt.Errorf("helm transform requires spec.transform.release")
		}
		args = append([]string{"template", release, sourcePath}, args...)
	case "jsonnet":
		// Jsonnet's CLI emits JSON, which the YAML decoder accepts.
		args = append([]string{sourcePath}, args...)
	default:
		return "", func() {}, fmt.Errorf("unsupported transform type %q", spec.Type)
	}

	cmd := exec.Command(path, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", func() {}, syncerrors.Wrap("transform", spec.Type, false, fmt.Errorf("%s failed: %s", spec.Type, message))
	}

	dir, err := os.MkdirTemp(".", ".gitsync-transform-*")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	outputPath := filepath.Join(dir, "output.yaml")
	if err := os.WriteFile(outputPath, stdout.Bytes(), 0o600); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return outputPath, cleanup, nil
}
