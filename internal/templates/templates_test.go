package templates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateTemplateRejectsSyntaxErrors(t *testing.T) {
	err := ValidateTemplate("name: {{ .Data.environment ")
	if err == nil {
		t.Fatal("expected invalid template syntax to be rejected")
	}
	if !strings.Contains(err.Error(), "invalid Go template syntax") {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestRenderTemplateExposesDocumentedData(t *testing.T) {
	values := map[string]string{"environment": "staging"}
	data := Data{
		ConfigSync: ConfigSyncData{Name: "app", Namespace: "default"},
		Source:     SourceData{Path: "config/app.yaml", Revision: "abc123"},
		Target:     TargetData{Namespace: "staging", Name: "app-config", Type: "ConfigMap"},
		Revision:   "abc123",
		Data:       values,
		Values:     values,
	}

	rendered, err := RenderTemplate(`apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Target.Name }}
  namespace: {{ .Target.Namespace }}
data:
  environment: {{ printf "%q" .Values.environment }}
  configSync: {{ printf "%q" .ConfigSync.Name }}
  revision: {{ printf "%q" .Revision }}
`, data)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"name: app-config",
		"namespace: staging",
		`environment: "staging"`,
		`configSync: "app"`,
		`revision: "abc123"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered template does not contain %q:\n%s", want, rendered)
		}
	}
}

func TestRenderTemplateRejectsMissingMapKey(t *testing.T) {
	_, err := RenderTemplate("value: {{ .Data.missing }}", Data{
		Data:   map[string]string{},
		Values: map[string]string{},
	})
	if err == nil {
		t.Fatal("expected missing template data key to be rejected")
	}
	if !strings.Contains(err.Error(), `map has no entry for key "missing"`) {
		t.Fatalf("unexpected missing-key error: %v", err)
	}
}

func TestRenderTemplateDoesNotHTMLEscapeValues(t *testing.T) {
	rendered, err := RenderTemplate(`value: {{ .Data.value }}`, Data{
		Data:   map[string]string{"value": "<production & staging>"},
		Values: map[string]string{"value": "<production & staging>"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, "<production & staging>") {
		t.Fatalf("text/template unexpectedly changed value: %s", rendered)
	}
}

func TestRenderSourceRejectsInvalidSyntaxBeforeWriting(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "invalid.yaml")
	if err := os.WriteFile(sourcePath, []byte("value: {{"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := RenderSource(sourcePath, Data{})
	if err == nil {
		t.Fatal("expected invalid source template to be rejected")
	}
}

func TestRenderSourceRendersFileAndCleanup(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "app.yaml")
	if err := os.WriteFile(sourcePath, []byte("name: {{ .Target.Name }}"), 0o600); err != nil {
		t.Fatal(err)
	}

	renderedPath, cleanup, err := RenderSource(sourcePath, Data{
		Target: TargetData{Name: "app-config"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	content, err := os.ReadFile(renderedPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "name: app-config" {
		t.Fatalf("unexpected rendered content: %q", content)
	}
	cleanup()
	if _, err := os.Stat(filepath.Dir(renderedPath)); !os.IsNotExist(err) {
		t.Fatalf("expected rendered temporary directory to be removed, got %v", err)
	}
}
