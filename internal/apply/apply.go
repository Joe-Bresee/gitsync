package apply

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	configsv1alpha1 "github.com/joe-bresee/gitsync/api/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"
)

// DryRunEnabled controls whether ApplyTarget validates each manifest with a
// server-side dry-run before applying it.
var DryRunEnabled = true

func ApplyTarget(ctx context.Context, c client.Client, sourcePath string, target configsv1alpha1.TargetRef) error {
	logger := log.FromContext(ctx)

	files, err := manifestFiles(sourcePath)
	if err != nil {
		return err
	}

	for _, filePath := range files {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", filePath, err)
		}

		docs := strings.Split(string(data), "\n---")
		for _, doc := range docs {
			doc = strings.TrimSpace(doc)
			if doc == "" {
				continue
			}

			obj := &unstructured.Unstructured{}
			jsonData, err := yaml.YAMLToJSON([]byte(doc))
			if err != nil {
				return fmt.Errorf("failed to convert YAML to JSON in %s: %w", filePath, err)
			}
			if err := obj.UnmarshalJSON(jsonData); err != nil {
				return fmt.Errorf("failed to unmarshal object in %s: %w", filePath, err)
			}

			if target.Namespace != "" {
				obj.SetNamespace(target.Namespace)
			}

			// Clean metadata completely
			cleanObjectForApply(obj)

			// Log object before applying
			logger.Info("Object before apply", "obj", obj.UnstructuredContent())

			applyOpts := []client.PatchOption{client.ForceOwnership, client.FieldOwner("configsync")}

			if DryRunEnabled {
				logger.Info("Performing server-side dry-run")
				if err := c.Patch(ctx, obj, client.Apply, append(applyOpts, client.DryRunAll)...); err != nil {
					return fmt.Errorf("dry-run failed for %s from %s: %w",
						obj.GetKind(), filePath, err)
				}
			}

			if err := c.Patch(ctx, obj, client.Apply, applyOpts...); err != nil {
				return fmt.Errorf("failed to apply %s from %s: %w",
					obj.GetKind(), filePath, err)
			}

			logger.Info("Applied manifest",
				"kind", obj.GetKind(),
				"name", obj.GetName(),
				"namespace", obj.GetNamespace(),
				"file", filePath,
			)
		}
	}

	return nil
}

func manifestFiles(sourcePath string) ([]string, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to access source path %s: %w", sourcePath, err)
	}

	if !info.IsDir() {
		return []string{sourcePath}, nil
	}

	entries, err := os.ReadDir(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to list source directory %s: %w", sourcePath, err)
	}

	files := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
			files = append(files, filepath.Join(sourcePath, name))
		}
	}
	return files, nil
}

// cleanObjectForApply removes all server-populated metadata fields to avoid managedFields errors
func cleanObjectForApply(obj *unstructured.Unstructured) {
	if obj == nil {
		return
	}

	content := obj.UnstructuredContent()

	// Remove status if present
	delete(content, "status")

	// Rebuild metadata with only safe fields
	meta := map[string]interface{}{
		"name":      obj.GetName(),
		"namespace": obj.GetNamespace(),
	}

	if labels := obj.GetLabels(); len(labels) > 0 {
		meta["labels"] = labels
	}
	if annotations := obj.GetAnnotations(); len(annotations) > 0 {
		meta["annotations"] = annotations
	}

	content["metadata"] = meta

	obj.SetUnstructuredContent(content)
}
