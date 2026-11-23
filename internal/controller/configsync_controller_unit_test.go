package controller

import (
	"errors"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	configsv1alpha1 "github.com/joe-bresee/gitsync/api/v1alpha1"
	syncerrors "github.com/joe-bresee/gitsync/internal/errors"
)

func TestSetConditionPreservesTransitionTimeWhenStatusIsUnchanged(t *testing.T) {
	status := &configsv1alpha1.ConfigSyncStatus{}
	setCondition(status, "Degraded", metav1.ConditionFalse, "ApplySucceeded", "ok")
	original := status.Conditions[0].LastTransitionTime

	time.Sleep(time.Millisecond)
	setCondition(status, "Degraded", metav1.ConditionFalse, "ApplySucceeded", "updated message")
	if !status.Conditions[0].LastTransitionTime.Equal(&original) {
		t.Fatal("condition transition time changed while status was unchanged")
	}

	time.Sleep(time.Millisecond)
	setCondition(status, "Degraded", metav1.ConditionTrue, "ApplyFailed", "failed")
	if !status.Conditions[0].LastTransitionTime.After(original.Time) {
		t.Fatal("condition transition time did not change when status changed")
	}
}

func TestRetryDelayIsBoundedAndResets(t *testing.T) {
	reconciler := &ConfigSyncReconciler{}
	key := types.NamespacedName{Namespace: "default", Name: "sync"}
	err := syncerrors.Wrap("fetch", "repo", true, errors.New("temporary"))

	want := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 32 * time.Second, 2 * time.Minute}
	for _, expected := range want {
		delay, retry := reconciler.retryDelay(key, err)
		if !retry || delay != expected {
			t.Fatalf("expected retry delay %s, got %s (retry=%t)", expected, delay, retry)
		}
	}
	reconciler.clearRetry(key)
	delay, retry := reconciler.retryDelay(key, err)
	if !retry || delay != 2*time.Second {
		t.Fatalf("expected reset retry delay of 2s, got %s (retry=%t)", delay, retry)
	}
}

func TestNonRetryableErrorsDoNotScheduleRetry(t *testing.T) {
	reconciler := &ConfigSyncReconciler{}
	delay, retry := reconciler.retryDelay(types.NamespacedName{Name: "sync"}, errors.New("invalid manifest"))
	if retry || delay != 0 {
		t.Fatalf("expected no retry, got %s (retry=%t)", delay, retry)
	}
}

func TestNamespaceBoundClientRejectsOutOfScopeNamespace(t *testing.T) {
	bound := &namespaceBoundClient{allowed: []string{"allowed"}}
	obj := &metav1.PartialObjectMetadata{}
	obj.SetNamespace("forbidden")
	if err := bound.checkNamespace(obj); err == nil {
		t.Fatal("expected namespace boundary rejection")
	}
	obj.SetNamespace("allowed")
	if err := bound.checkNamespace(obj); err != nil {
		t.Fatalf("expected allowed namespace, got %v", err)
	}
}
