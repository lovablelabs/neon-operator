package utils

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	neonv1alpha1 "oltp.molnett.org/neon-operator/api/v1alpha1"
)

func TestSetCondition_AddsCondition(t *testing.T) {
	cluster := &neonv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Generation: 3},
	}
	SetCondition(cluster, &cluster.Status.Conditions, ConditionAvailable, metav1.ConditionTrue, ReasonAsExpected, "Cluster is Available")

	if len(cluster.Status.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(cluster.Status.Conditions))
	}
	c := cluster.Status.Conditions[0]
	if c.Type != ConditionAvailable {
		t.Errorf("expected type %s, got %s", ConditionAvailable, c.Type)
	}
	if c.Status != metav1.ConditionTrue {
		t.Errorf("expected status True, got %s", c.Status)
	}
	if c.Reason != ReasonAsExpected {
		t.Errorf("expected reason %s, got %s", ReasonAsExpected, c.Reason)
	}
	if c.ObservedGeneration != 3 {
		t.Errorf("expected ObservedGeneration 3, got %d", c.ObservedGeneration)
	}
	if c.LastTransitionTime.IsZero() {
		t.Error("expected LastTransitionTime to be set")
	}
}

func TestSetCondition_PreservesLastTransitionTimeOnUnchangedStatus(t *testing.T) {
	cluster := &neonv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Generation: 1},
	}
	SetCondition(cluster, &cluster.Status.Conditions, ConditionAvailable, metav1.ConditionTrue, ReasonAsExpected, "first")
	first := cluster.Status.Conditions[0].LastTransitionTime

	cluster.Generation = 2
	SetCondition(cluster, &cluster.Status.Conditions, ConditionAvailable, metav1.ConditionTrue, ReasonAsExpected, "second")
	second := cluster.Status.Conditions[0].LastTransitionTime

	if !first.Equal(&second) {
		t.Errorf("LastTransitionTime should not change when Status stays the same: %v vs %v", first, second)
	}
	if cluster.Status.Conditions[0].ObservedGeneration != 2 {
		t.Errorf("ObservedGeneration should update to 2, got %d", cluster.Status.Conditions[0].ObservedGeneration)
	}
}

func TestSetCondition_UpdatesLastTransitionTimeOnStatusFlip(t *testing.T) {
	cluster := &neonv1alpha1.Cluster{}
	SetCondition(cluster, &cluster.Status.Conditions, ConditionAvailable, metav1.ConditionFalse, ReasonReconciling, "wait")
	first := cluster.Status.Conditions[0].LastTransitionTime

	cluster.Status.Conditions[0].LastTransitionTime = metav1.NewTime(first.Add(-1))

	SetCondition(cluster, &cluster.Status.Conditions, ConditionAvailable, metav1.ConditionTrue, ReasonAsExpected, "ready")
	second := cluster.Status.Conditions[0].LastTransitionTime

	if !second.After(cluster.Status.Conditions[0].LastTransitionTime.Add(-1)) {
		t.Errorf("LastTransitionTime should advance when Status flips")
	}
	if cluster.Status.Conditions[0].Status != metav1.ConditionTrue {
		t.Errorf("expected status True after flip, got %s", cluster.Status.Conditions[0].Status)
	}
}

func TestSetCondition_PreservesOtherConditions(t *testing.T) {
	cluster := &neonv1alpha1.Cluster{}
	SetCondition(cluster, &cluster.Status.Conditions, ConditionAvailable, metav1.ConditionTrue, ReasonAsExpected, "")
	SetCondition(cluster, &cluster.Status.Conditions, ConditionProgressing, metav1.ConditionFalse, ReasonAsExpected, "")

	SetCondition(cluster, &cluster.Status.Conditions, ConditionAvailable, metav1.ConditionFalse, ReasonReconciling, "")

	if len(cluster.Status.Conditions) != 2 {
		t.Fatalf("expected 2 conditions, got %d", len(cluster.Status.Conditions))
	}
	var available, progressing *metav1.Condition
	for i := range cluster.Status.Conditions {
		switch cluster.Status.Conditions[i].Type {
		case ConditionAvailable:
			available = &cluster.Status.Conditions[i]
		case ConditionProgressing:
			progressing = &cluster.Status.Conditions[i]
		}
	}
	if available == nil || progressing == nil {
		t.Fatal("expected both Available and Progressing conditions")
	}
	if available.Status != metav1.ConditionFalse {
		t.Errorf("expected Available False, got %s", available.Status)
	}
	if progressing.Status != metav1.ConditionFalse {
		t.Errorf("expected Progressing to remain False, got %s", progressing.Status)
	}
}
