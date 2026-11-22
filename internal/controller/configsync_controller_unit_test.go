package controller

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configsv1alpha1 "github.com/joe-bresee/gitsync/api/v1alpha1"
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
