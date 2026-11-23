/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	configsv1alpha1 "github.com/joe-bresee/gitsync/api/v1alpha1"
	apply "github.com/joe-bresee/gitsync/internal/apply"
	source "github.com/joe-bresee/gitsync/internal/sources"
	"github.com/joe-bresee/gitsync/internal/templates"
)

// ConfigSyncReconciler reconciles a ConfigSync object
type ConfigSyncReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=configs.example.io,resources=configsyncs,verbs=get;list;watch
// +kubebuilder:rbac:groups=configs.example.io,resources=configsyncs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=configs.example.io,resources=configsyncs/finalizers,verbs=update

// +kubebuilder:rbac:groups="",resources=configmaps,verbs=patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ConfigSync object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.4/pkg/reconcile
func (r *ConfigSyncReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the CR
	var configSync configsv1alpha1.ConfigSync
	if err := r.Get(ctx, req.NamespacedName, &configSync); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	previousStatus := configSync.DeepCopy().Status

	// --------------------------------------------------------------
	// Step 1: Fetch source and determine revision
	// --------------------------------------------------------------
	revisionSHA, sourcePath, checkoutPath, commitMsg, err := source.FetchSource(&configSync, ctx, r.Client)
	if err != nil {
		setCondition(&configSync.Status, "Degraded", metav1.ConditionTrue, "SourceFetchFailed", err.Error())
		if err := r.updateStatusIfChanged(ctx, &configSync, previousStatus); err != nil {
			return ctrl.Result{}, err
		}
		requeueAfter, requeueErr := reconcileInterval(configSync.Spec.RefreshInterval)
		if requeueErr != nil {
			setCondition(&configSync.Status, "Degraded", metav1.ConditionTrue, "InvalidRefreshInterval", requeueErr.Error())
			if err := r.updateStatusIfChanged(ctx, &configSync, previousStatus); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	// Log the fetched commit message (if any)
	if commitMsg != "" {
		log.Info("fetched source commit", "commit", revisionSHA, "message", commitMsg)
	} else {
		log.Info("fetched source", "commit", revisionSHA)
	}

	if checkoutPath != "" {
		defer os.RemoveAll(checkoutPath)
	}

	environment, err := source.ResolveEnvironment(&configSync)
	if err != nil {
		setCondition(&configSync.Status, "Degraded", metav1.ConditionTrue, "EnvironmentInvalid", err.Error())
		if err := r.updateStatusIfChanged(ctx, &configSync, previousStatus); err != nil {
			return ctrl.Result{}, err
		}
		requeueAfter, requeueErr := reconcileInterval(configSync.Spec.RefreshInterval)
		if requeueErr != nil {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	// --------------------------------------------------------------
	// Step 2: Apply desired state
	// --------------------------------------------------------------
	// Always apply so periodic reconciliation repairs out-of-band edits and
	// restores targets that were deleted without a source revision change.
	log.Info("Applying desired state", "revision", revisionSHA)
	for _, target := range configSync.Spec.Targets {
		applyPath := sourcePath
		cleanupRendered := func() {}

		if configSync.Spec.Template != nil {
			values := configSync.Spec.Template.Data
			if values == nil {
				values = map[string]string{}
			}
			values = copyStringMap(values)
			for key, value := range environment.Data {
				values[key] = value
			}
			templateData := templates.Data{
				ConfigSync: templates.ConfigSyncData{
					Name:      configSync.Name,
					Namespace: configSync.Namespace,
				},
				Source: templates.SourceData{
					Path:     configSync.Spec.Source.Git.Path,
					Revision: revisionSHA,
				},
				Target: templates.TargetData{
					Namespace: target.Namespace,
					Name:      target.Name,
					Type:      target.Type,
				},
				Environment: templates.EnvironmentData{
					Name:    environment.Name,
					Branch:  environment.Branch,
					Overlay: environment.Overlay,
					Data:    environment.Data,
				},
				Revision: revisionSHA,
				Data:     values,
				Values:   values,
			}

			renderedPath, cleanup, err := templates.RenderSource(sourcePath, templateData)
			if err != nil {
				setCondition(&configSync.Status, "Degraded", metav1.ConditionTrue, "TemplateRenderFailed", err.Error())
				if err := r.updateStatusIfChanged(ctx, &configSync, previousStatus); err != nil {
					return ctrl.Result{}, err
				}
				requeueAfter, requeueErr := reconcileInterval(configSync.Spec.RefreshInterval)
				if requeueErr != nil {
					setCondition(&configSync.Status, "Degraded", metav1.ConditionTrue, "InvalidRefreshInterval", requeueErr.Error())
					if err := r.updateStatusIfChanged(ctx, &configSync, previousStatus); err != nil {
						return ctrl.Result{}, err
					}
					return ctrl.Result{}, nil
				}
				return ctrl.Result{RequeueAfter: requeueAfter}, nil
			}
			applyPath = renderedPath
			cleanupRendered = cleanup
		}

		err := apply.ApplyTarget(ctx, r.Client, applyPath, target)
		cleanupRendered()
		if err != nil {
			setCondition(&configSync.Status, "Degraded", metav1.ConditionTrue, "ApplyFailed", err.Error())
			if err := r.updateStatusIfChanged(ctx, &configSync, previousStatus); err != nil {
				return ctrl.Result{}, err
			}
			requeueAfter, requeueErr := reconcileInterval(configSync.Spec.RefreshInterval)
			if requeueErr != nil {
				setCondition(&configSync.Status, "Degraded", metav1.ConditionTrue, "InvalidRefreshInterval", requeueErr.Error())
				if err := r.updateStatusIfChanged(ctx, &configSync, previousStatus); err != nil {
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, nil
			}
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
	}
	setCondition(&configSync.Status, "Degraded", metav1.ConditionFalse, "ApplySucceeded", "All targets applied successfully")

	// --------------------------------------------------------------
	// Step 3: Update status after apply
	// --------------------------------------------------------------
	configSync.Status.LastSyncedTime = &metav1.Time{Time: time.Now()}
	configSync.Status.AppliedTargets = len(configSync.Spec.Targets)
	configSync.Status.SourceRevision = revisionSHA
	statusSourcePath := configSync.Spec.Source.Git.Path
	if environment.Overlay != "" {
		statusSourcePath = filepath.Join(statusSourcePath, environment.Overlay)
		statusSourcePath = filepath.ToSlash(statusSourcePath)
	}
	configSync.Status.SourcePath = statusSourcePath
	configSync.Status.Environment = environment.Name
	configSync.Status.EnvironmentBranch = environment.Branch
	configSync.Status.EnvironmentOverlay = environment.Overlay

	if err := r.updateStatusIfChanged(ctx, &configSync, previousStatus); err != nil {
		return ctrl.Result{}, err
	}

	// --------------------------------------------------------------
	// Step 4: Compute requeue interval
	// --------------------------------------------------------------
	requeueAfter, err := reconcileInterval(configSync.Spec.RefreshInterval)
	if err != nil {
		setCondition(&configSync.Status, "Degraded", metav1.ConditionTrue, "InvalidRefreshInterval", err.Error())
		if err := r.updateStatusIfChanged(ctx, &configSync, previousStatus); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	log.Info("Reconcile completed", "requeueAfter", requeueAfter.String())
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// SetupWithManager
// --------------------------------------------------------------
func (r *ConfigSyncReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(
			&configsv1alpha1.ConfigSync{},
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(r.findConfigSyncsForSecret)).
		Named("configsync").
		Complete(r)
}

func reconcileInterval(refreshInterval string) (time.Duration, error) {
	if refreshInterval == "" {
		return 30 * time.Second, nil
	}
	d, err := time.ParseDuration(refreshInterval)
	if err != nil {
		return 0, fmt.Errorf("RefreshInterval must be a valid duration (e.g. 30s, 5m): %w", err)
	}
	return d, nil
}

func copyStringMap(values map[string]string) map[string]string {
	copied := make(map[string]string, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}

func (r *ConfigSyncReconciler) updateStatusIfChanged(
	ctx context.Context,
	configSync *configsv1alpha1.ConfigSync,
	previousStatus configsv1alpha1.ConfigSyncStatus,
) error {
	if reflect.DeepEqual(previousStatus, configSync.Status) {
		return nil
	}
	return r.Status().Update(ctx, configSync)
}

func (r *ConfigSyncReconciler) findConfigSyncsForSecret(ctx context.Context, obj client.Object) []reconcile.Request {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return nil
	}

	var resources configsv1alpha1.ConfigSyncList
	if err := r.List(ctx, &resources); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, 0)
	for i := range resources.Items {
		resource := &resources.Items[i]
		gitSource := resource.Spec.Source.Git
		if gitSource == nil || gitSource.AuthSecretRef == nil {
			continue
		}

		namespace := gitSource.AuthSecretRef.Namespace
		if namespace == "" {
			namespace = resource.Namespace
		}
		if namespace == secret.Namespace && gitSource.AuthSecretRef.Name == secret.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      resource.Name,
					Namespace: resource.Namespace,
				},
			})
		}
	}
	return requests
}

// Condition helper
// --------------------------------------------------------------
func setCondition(status *configsv1alpha1.ConfigSyncStatus, conditionType string, statusValue metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	for i, c := range status.Conditions {
		if c.Type == conditionType {
			if c.Status == statusValue && c.Reason == reason && c.Message == message {
				return
			}
			transitionTime := c.LastTransitionTime
			if c.Status != statusValue {
				transitionTime = now
			}
			status.Conditions[i] = metav1.Condition{
				Type:               conditionType,
				Status:             statusValue,
				Reason:             reason,
				Message:            message,
				LastTransitionTime: transitionTime,
			}
			return
		}
	}

	status.Conditions = append(status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             statusValue,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
	})
}
