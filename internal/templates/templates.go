package templates

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// ConfigSyncData identifies the ConfigSync resource currently being reconciled.
type ConfigSyncData struct {
	Name      string
	Namespace string
}

// SourceData describes the fetched Git source.
type SourceData struct {
	Path     string
	Revision string
}

// TargetData describes the Kubernetes target receiving the rendered manifests.
type TargetData struct {
	Namespace string
	Name      string
	Type      string
}

// EnvironmentData describes the resolved environment selection.
type EnvironmentData struct {
	Name    string
	Branch  string
	Overlay string
	Data    map[string]string
}

// Data is the complete input available to source Go templates.
// Data and Values contain the same user-defined string values.
type Data struct {
	ConfigSync  ConfigSyncData
	Source      SourceData
	Target      TargetData
	Environment EnvironmentData
	Revision    string
	Data        map[string]string
	Values      map[string]string
}

// ValidateTemplate parses templateText and reports syntax errors.
func ValidateTemplate(templateText string) error {
	_, err := template.New("manifest").Option("missingkey=error").Parse(templateText)
	if err != nil {
		return fmt.Errorf("invalid Go template syntax: %w", err)
	}
	return nil
}

// Validate is a shorthand for ValidateTemplate.
func Validate(templateText string) error {
	return ValidateTemplate(templateText)
}

// RenderTemplate parses and executes templateText with data.
// Text/template performs no HTML or YAML escaping. Templates must add YAML
// quoting or encoding explicitly for values that need it.
func RenderTemplate(templateText string, data Data) (string, error) {
	tmpl, err := template.New("manifest").Option("missingkey=error").Parse(templateText)
	if err != nil {
		return "", fmt.Errorf("invalid Go template syntax: %w", err)
	}

	var output bytes.Buffer
	if err := tmpl.Execute(&output, data); err != nil {
		return "", fmt.Errorf("failed to execute Go template: %w", err)
	}
	return output.String(), nil
}

// Render is a shorthand for RenderTemplate.
func Render(templateText string, data Data) (string, error) {
	return RenderTemplate(templateText, data)
}

// ValidateSource validates every YAML template in a file or directory.
func ValidateSource(sourcePath string) error {
	files, err := templateFiles(sourcePath)
	if err != nil {
		return err
	}
	for _, filePath := range files {
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read template %s: %w", filePath, err)
		}
		if err := ValidateTemplate(string(content)); err != nil {
			return fmt.Errorf("%s: %w", filePath, err)
		}
	}
	return nil
}

// RenderSource renders a source file or directory into a temporary path.
// The caller must invoke cleanup after apply completes.
func RenderSource(sourcePath string, data Data) (renderedPath string, cleanup func(), err error) {
	if err := ValidateSource(sourcePath); err != nil {
		return "", func() {}, err
	}

	info, err := os.Stat(sourcePath)
	if err != nil {
		return "", func() {}, fmt.Errorf("failed to access template source %s: %w", sourcePath, err)
	}

	tempRoot, err := os.MkdirTemp("", "gitsync-rendered-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("failed to create rendered template directory: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(tempRoot) }
	cleanupOnError := true
	defer func() {
		if cleanupOnError {
			cleanup()
		}
	}()

	if !info.IsDir() {
		content, readErr := os.ReadFile(sourcePath)
		if readErr != nil {
			return "", func() {}, fmt.Errorf("failed to read template %s: %w", sourcePath, readErr)
		}
		rendered, renderErr := RenderTemplate(string(content), data)
		if renderErr != nil {
			return "", func() {}, fmt.Errorf("failed to render %s: %w", sourcePath, renderErr)
		}
		outputPath := filepath.Join(tempRoot, filepath.Base(sourcePath))
		if writeErr := os.WriteFile(outputPath, []byte(rendered), 0o600); writeErr != nil {
			return "", func() {}, fmt.Errorf("failed to write rendered template %s: %w", outputPath, writeErr)
		}
		cleanupOnError = false
		return outputPath, cleanup, nil
	}

	files, err := templateFiles(sourcePath)
	if err != nil {
		return "", func() {}, err
	}
	for _, filePath := range files {
		content, readErr := os.ReadFile(filePath)
		if readErr != nil {
			return "", func() {}, fmt.Errorf("failed to read template %s: %w", filePath, readErr)
		}
		rendered, renderErr := RenderTemplate(string(content), data)
		if renderErr != nil {
			return "", func() {}, fmt.Errorf("failed to render %s: %w", filePath, renderErr)
		}
		outputPath := filepath.Join(tempRoot, filepath.Base(filePath))
		if writeErr := os.WriteFile(outputPath, []byte(rendered), 0o600); writeErr != nil {
			return "", func() {}, fmt.Errorf("failed to write rendered template %s: %w", outputPath, writeErr)
		}
	}

	cleanupOnError = false
	return tempRoot, cleanup, nil
}

func templateFiles(sourcePath string) ([]string, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to access template source %s: %w", sourcePath, err)
	}
	if !info.IsDir() {
		name := strings.ToLower(info.Name())
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			return nil, fmt.Errorf("template source must be a YAML file or directory: %s", sourcePath)
		}
		return []string{sourcePath}, nil
	}

	entries, err := os.ReadDir(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to list template directory %s: %w", sourcePath, err)
	}
	files := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		name := strings.ToLower(entry.Name())
		if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
			files = append(files, filepath.Join(sourcePath, entry.Name()))
		}
	}
	return files, nil
}
