package apply

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	configsv1alpha1 "github.com/joe-bresee/gitsync/api/v1alpha1"
	syncerrors "github.com/joe-bresee/gitsync/internal/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8svalidation "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// DryRunEnabled controls whether ApplyTarget validates each manifest with a
// server-side dry-run before applying it.
var DryRunEnabled = true

// ManifestValidationError identifies the exact source file and object that
// failed validation, for actionable reconciliation status.
type ManifestValidationError struct {
	File   string
	Kind   string
	Name   string
	Reason string
}

func (e *ManifestValidationError) Error() string {
	object := e.Kind
	if e.Name != "" {
		object += " " + e.Name
	}
	return fmt.Sprintf("manifest %s: object %s is invalid: %s", e.File, object, e.Reason)
}

func ApplyTarget(ctx context.Context, c client.Client, sourcePath string, target configsv1alpha1.TargetRef) error {
	logger := log.FromContext(ctx)

	files, err := manifestFiles(sourcePath)
	if err != nil {
		return syncerrors.Wrap("read manifests", sourcePath, false, err)
	}

	if target.Namespace != "" {
		if errs := k8svalidation.IsDNS1123Label(target.Namespace); len(errs) > 0 {
			return &ManifestValidationError{
				File: "target", Kind: target.Type, Name: target.Name,
				Reason: fmt.Sprintf("target namespace %q is invalid: %s", target.Namespace, strings.Join(errs, "; ")),
			}
		}
	}

	type manifestObject struct {
		file string
		obj  *unstructured.Unstructured
	}
	objects := make([]manifestObject, 0)

	for _, filePath := range files {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return syncerrors.Wrap("read manifest", filePath, false, err)
		}

		decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
		document := 0
		for {
			document++
			obj := &unstructured.Unstructured{}
			err := decoder.Decode(obj)
			if err == io.EOF {
				break
			}
			if err != nil {
				return &ManifestValidationError{
					File: filePath, Kind: "YAML", Name: fmt.Sprintf("document %d", document),
					Reason: fmt.Sprintf("failed to parse YAML/JSON: %v", err),
				}
			}
			if len(obj.Object) == 0 {
				continue
			}

			if target.Namespace != "" {
				obj.SetNamespace(target.Namespace)
			}

			cleanObjectForApply(obj)
			if err := validateSupportedObject(obj, filePath); err != nil {
				return err
			}
			objects = append(objects, manifestObject{file: filePath, obj: obj})
		}
	}

	for _, manifest := range objects {
		obj := manifest.obj
		logger.Info("Object before apply", "obj", obj.UnstructuredContent())

		applyOpts := []client.PatchOption{client.ForceOwnership, client.FieldOwner("configsync")}

		if DryRunEnabled {
			logger.Info("Performing server-side dry-run")
			if err := c.Patch(ctx, obj, client.Apply, append(applyOpts, client.DryRunAll)...); err != nil {
				return syncerrors.Wrap("dry-run apply", fmt.Sprintf("%s/%s", obj.GetKind(), obj.GetName()), isTransientAPIError(err), err)
			}
		}

		if err := c.Patch(ctx, obj, client.Apply, applyOpts...); err != nil {
			return syncerrors.Wrap("apply", fmt.Sprintf("%s/%s", obj.GetKind(), obj.GetName()), isTransientAPIError(err), err)
		}

		logger.Info("Applied manifest",
			"kind", obj.GetKind(),
			"name", obj.GetName(),
			"namespace", obj.GetNamespace(),
			"file", manifest.file,
		)
	}

	return nil
}

func isTransientAPIError(err error) bool {
	return apierrors.IsTimeout(err) ||
		apierrors.IsTooManyRequests(err) ||
		apierrors.IsServerTimeout(err) ||
		apierrors.IsServiceUnavailable(err) ||
		apierrors.IsInternalError(err)
}

func validateSupportedObject(obj *unstructured.Unstructured, filePath string) error {
	kind := obj.GetKind()
	name := obj.GetName()
	validationError := func(reason string) error {
		return &ManifestValidationError{File: filePath, Kind: kind, Name: name, Reason: reason}
	}

	if obj.GetAPIVersion() == "" {
		return validationError("apiVersion is required")
	}
	if kind == "" {
		return validationError("kind is required")
	}
	if name == "" {
		return validationError("metadata.name is required")
	}
	if errs := k8svalidation.IsDNS1123Subdomain(name); len(errs) > 0 {
		return validationError(fmt.Sprintf("metadata.name %q is invalid: %s", name, strings.Join(errs, "; ")))
	}
	if namespace := obj.GetNamespace(); namespace != "" {
		if errs := k8svalidation.IsDNS1123Label(namespace); len(errs) > 0 {
			return validationError(fmt.Sprintf("metadata.namespace %q is invalid: %s", namespace, strings.Join(errs, "; ")))
		}
	}

	switch kind {
	case "ConfigMap":
		if obj.GetAPIVersion() != "v1" {
			return validationError(fmt.Sprintf("apiVersion %q is invalid for ConfigMap; expected v1", obj.GetAPIVersion()))
		}
		var resource corev1.ConfigMap
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &resource); err != nil {
			return validationError(fmt.Sprintf("ConfigMap fields are invalid: %v", err))
		}
	case "Secret":
		if obj.GetAPIVersion() != "v1" {
			return validationError(fmt.Sprintf("apiVersion %q is invalid for Secret; expected v1", obj.GetAPIVersion()))
		}
		var resource corev1.Secret
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &resource); err != nil {
			return validationError(fmt.Sprintf("Secret fields are invalid: %v", err))
		}
	case "Deployment":
		if obj.GetAPIVersion() != "apps/v1" {
			return validationError(fmt.Sprintf("apiVersion %q is invalid for Deployment; expected apps/v1", obj.GetAPIVersion()))
		}
		var resource appsv1.Deployment
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &resource); err != nil {
			return validationError(fmt.Sprintf("Deployment fields are invalid: %v", err))
		}
		if resource.Spec.Selector == nil {
			return validationError("Deployment spec.selector is required")
		}
		if len(resource.Spec.Selector.MatchLabels) == 0 && len(resource.Spec.Selector.MatchExpressions) == 0 {
			return validationError("Deployment spec.selector must contain matchLabels or matchExpressions")
		}
		if resource.Spec.Template.Spec.Containers == nil || len(resource.Spec.Template.Spec.Containers) == 0 {
			return validationError("Deployment spec.template.spec.containers is required and must contain at least one container")
		}
		for i, container := range resource.Spec.Template.Spec.Containers {
			if container.Name == "" {
				return validationError(fmt.Sprintf("Deployment spec.template.spec.containers[%d].name is required", i))
			}
			if container.Image == "" {
				return validationError(fmt.Sprintf("Deployment container %q requires spec.template.spec.containers[%d].image", container.Name, i))
			}
		}
	default:
		return validationError(fmt.Sprintf("unsupported kind %q; supported kinds are ConfigMap, Secret, and Deployment", kind))
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
