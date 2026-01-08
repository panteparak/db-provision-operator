/*
Copyright 2026.

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

package testutil

import (
	"fmt"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

// HaveCondition returns a Gomega matcher that checks if an object has a condition
// of the specified type with the specified status.
func HaveCondition(conditionType string, status metav1.ConditionStatus) types.GomegaMatcher {
	return &haveConditionMatcher{
		conditionType: conditionType,
		status:        status,
	}
}

// HaveConditionWithReason returns a Gomega matcher that checks if an object has a condition
// of the specified type with the specified status and reason.
func HaveConditionWithReason(conditionType string, status metav1.ConditionStatus, reason string) types.GomegaMatcher {
	return &haveConditionMatcher{
		conditionType: conditionType,
		status:        status,
		reason:        reason,
		checkReason:   true,
	}
}

type haveConditionMatcher struct {
	conditionType string
	status        metav1.ConditionStatus
	reason        string
	checkReason   bool
}

func (m *haveConditionMatcher) Match(actual interface{}) (success bool, err error) {
	conditions, err := extractConditions(actual)
	if err != nil {
		return false, err
	}

	for _, c := range conditions {
		if c.Type == m.conditionType {
			if c.Status != m.status {
				return false, nil
			}
			if m.checkReason && c.Reason != m.reason {
				return false, nil
			}
			return true, nil
		}
	}
	return false, nil
}

func (m *haveConditionMatcher) FailureMessage(actual interface{}) string {
	conditions, _ := extractConditions(actual)
	if m.checkReason {
		return fmt.Sprintf("Expected object to have condition %q with status %q and reason %q, but got conditions: %s",
			m.conditionType, m.status, m.reason, format.Object(conditions, 1))
	}
	return fmt.Sprintf("Expected object to have condition %q with status %q, but got conditions: %s",
		m.conditionType, m.status, format.Object(conditions, 1))
}

func (m *haveConditionMatcher) NegatedFailureMessage(actual interface{}) string {
	if m.checkReason {
		return fmt.Sprintf("Expected object NOT to have condition %q with status %q and reason %q",
			m.conditionType, m.status, m.reason)
	}
	return fmt.Sprintf("Expected object NOT to have condition %q with status %q",
		m.conditionType, m.status)
}

// HavePhase returns a Gomega matcher that checks if an object has the specified phase.
func HavePhase(phase dbopsv1alpha1.Phase) types.GomegaMatcher {
	return &havePhaseMatcher{
		expectedPhase: phase,
	}
}

type havePhaseMatcher struct {
	expectedPhase dbopsv1alpha1.Phase
}

func (m *havePhaseMatcher) Match(actual interface{}) (success bool, err error) {
	phase, err := extractPhase(actual)
	if err != nil {
		return false, err
	}
	return phase == m.expectedPhase, nil
}

func (m *havePhaseMatcher) FailureMessage(actual interface{}) string {
	phase, _ := extractPhase(actual)
	return fmt.Sprintf("Expected object to have phase %q, but got %q", m.expectedPhase, phase)
}

func (m *havePhaseMatcher) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected object NOT to have phase %q", m.expectedPhase)
}

// HaveFinalizer returns a Gomega matcher that checks if an object has the specified finalizer.
func HaveFinalizer(finalizer string) types.GomegaMatcher {
	return &haveFinalizerMatcher{
		finalizer: finalizer,
	}
}

type haveFinalizerMatcher struct {
	finalizer string
}

func (m *haveFinalizerMatcher) Match(actual interface{}) (success bool, err error) {
	obj, ok := actual.(client.Object)
	if !ok {
		return false, fmt.Errorf("HaveFinalizer matcher expects a client.Object, got %T", actual)
	}

	finalizers := obj.GetFinalizers()
	for _, f := range finalizers {
		if f == m.finalizer {
			return true, nil
		}
	}
	return false, nil
}

func (m *haveFinalizerMatcher) FailureMessage(actual interface{}) string {
	obj := actual.(client.Object)
	return fmt.Sprintf("Expected object to have finalizer %q, but got finalizers: %v",
		m.finalizer, obj.GetFinalizers())
}

func (m *haveFinalizerMatcher) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected object NOT to have finalizer %q", m.finalizer)
}

// BeReady returns a Gomega matcher that checks if an object has phase Ready.
func BeReady() types.GomegaMatcher {
	return HavePhase(dbopsv1alpha1.PhaseReady)
}

// BePending returns a Gomega matcher that checks if an object has phase Pending.
func BePending() types.GomegaMatcher {
	return HavePhase(dbopsv1alpha1.PhasePending)
}

// BeFailed returns a Gomega matcher that checks if an object has phase Failed.
func BeFailed() types.GomegaMatcher {
	return HavePhase(dbopsv1alpha1.PhaseFailed)
}

// BeCreating returns a Gomega matcher that checks if an object has phase Creating.
func BeCreating() types.GomegaMatcher {
	return HavePhase(dbopsv1alpha1.PhaseCreating)
}

// BeDeleting returns a Gomega matcher that checks if an object has phase Deleting.
func BeDeleting() types.GomegaMatcher {
	return HavePhase(dbopsv1alpha1.PhaseDeleting)
}

// BeCompleted returns a Gomega matcher that checks if an object has phase Completed.
func BeCompleted() types.GomegaMatcher {
	return HavePhase(dbopsv1alpha1.PhaseCompleted)
}

// BeRunning returns a Gomega matcher that checks if an object has phase Running.
func BeRunning() types.GomegaMatcher {
	return HavePhase(dbopsv1alpha1.PhaseRunning)
}

// BeActive returns a Gomega matcher that checks if an object has phase Active.
func BeActive() types.GomegaMatcher {
	return HavePhase(dbopsv1alpha1.PhaseActive)
}

// BePaused returns a Gomega matcher that checks if an object has phase Paused.
func BePaused() types.GomegaMatcher {
	return HavePhase(dbopsv1alpha1.PhasePaused)
}

// HaveReadyCondition returns a Gomega matcher that checks if an object has a Ready condition with True status.
func HaveReadyCondition() types.GomegaMatcher {
	return HaveCondition(dbopsv1alpha1.ConditionTypeReady, metav1.ConditionTrue)
}

// HaveConnectedCondition returns a Gomega matcher that checks if an object has a Connected condition with True status.
func HaveConnectedCondition() types.GomegaMatcher {
	return HaveCondition(dbopsv1alpha1.ConditionTypeConnected, metav1.ConditionTrue)
}

// HaveCompleteCondition returns a Gomega matcher that checks if an object has a Complete condition with True status.
func HaveCompleteCondition() types.GomegaMatcher {
	return HaveCondition(dbopsv1alpha1.ConditionTypeComplete, metav1.ConditionTrue)
}

// NotHaveReadyCondition returns a Gomega matcher that checks if an object does NOT have a Ready condition with True status.
func NotHaveReadyCondition() types.GomegaMatcher {
	return gomega.Not(HaveReadyCondition())
}

// HaveObservedGeneration returns a Gomega matcher that checks if the observed generation matches.
func HaveObservedGeneration(generation int64) types.GomegaMatcher {
	return &haveObservedGenerationMatcher{
		expectedGeneration: generation,
	}
}

type haveObservedGenerationMatcher struct {
	expectedGeneration int64
}

func (m *haveObservedGenerationMatcher) Match(actual interface{}) (success bool, err error) {
	generation, err := extractObservedGeneration(actual)
	if err != nil {
		return false, err
	}
	return generation == m.expectedGeneration, nil
}

func (m *haveObservedGenerationMatcher) FailureMessage(actual interface{}) string {
	generation, _ := extractObservedGeneration(actual)
	return fmt.Sprintf("Expected object to have observedGeneration %d, but got %d",
		m.expectedGeneration, generation)
}

func (m *haveObservedGenerationMatcher) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected object NOT to have observedGeneration %d", m.expectedGeneration)
}

// HaveAnnotation returns a Gomega matcher that checks if an object has the specified annotation.
func HaveAnnotation(key, value string) types.GomegaMatcher {
	return &haveAnnotationMatcher{
		key:   key,
		value: value,
	}
}

type haveAnnotationMatcher struct {
	key   string
	value string
}

func (m *haveAnnotationMatcher) Match(actual interface{}) (success bool, err error) {
	obj, ok := actual.(client.Object)
	if !ok {
		return false, fmt.Errorf("HaveAnnotation matcher expects a client.Object, got %T", actual)
	}

	annotations := obj.GetAnnotations()
	if annotations == nil {
		return false, nil
	}

	actualValue, exists := annotations[m.key]
	if !exists {
		return false, nil
	}
	return actualValue == m.value, nil
}

func (m *haveAnnotationMatcher) FailureMessage(actual interface{}) string {
	obj := actual.(client.Object)
	return fmt.Sprintf("Expected object to have annotation %q=%q, but got annotations: %v",
		m.key, m.value, obj.GetAnnotations())
}

func (m *haveAnnotationMatcher) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected object NOT to have annotation %q=%q", m.key, m.value)
}

// HaveLabel returns a Gomega matcher that checks if an object has the specified label.
func HaveLabel(key, value string) types.GomegaMatcher {
	return &haveLabelMatcher{
		key:   key,
		value: value,
	}
}

type haveLabelMatcher struct {
	key   string
	value string
}

func (m *haveLabelMatcher) Match(actual interface{}) (success bool, err error) {
	obj, ok := actual.(client.Object)
	if !ok {
		return false, fmt.Errorf("HaveLabel matcher expects a client.Object, got %T", actual)
	}

	labels := obj.GetLabels()
	if labels == nil {
		return false, nil
	}

	actualValue, exists := labels[m.key]
	if !exists {
		return false, nil
	}
	return actualValue == m.value, nil
}

func (m *haveLabelMatcher) FailureMessage(actual interface{}) string {
	obj := actual.(client.Object)
	return fmt.Sprintf("Expected object to have label %q=%q, but got labels: %v",
		m.key, m.value, obj.GetLabels())
}

func (m *haveLabelMatcher) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected object NOT to have label %q=%q", m.key, m.value)
}

// BeMarkedForDeletion returns a Gomega matcher that checks if an object is marked for deletion.
func BeMarkedForDeletion() types.GomegaMatcher {
	return &beMarkedForDeletionMatcher{}
}

type beMarkedForDeletionMatcher struct{}

func (m *beMarkedForDeletionMatcher) Match(actual interface{}) (success bool, err error) {
	obj, ok := actual.(client.Object)
	if !ok {
		return false, fmt.Errorf("BeMarkedForDeletion matcher expects a client.Object, got %T", actual)
	}
	return !obj.GetDeletionTimestamp().IsZero(), nil
}

func (m *beMarkedForDeletionMatcher) FailureMessage(actual interface{}) string {
	return "Expected object to be marked for deletion (have non-zero deletion timestamp)"
}

func (m *beMarkedForDeletionMatcher) NegatedFailureMessage(actual interface{}) string {
	return "Expected object NOT to be marked for deletion"
}

// extractConditions extracts conditions from various object types.
func extractConditions(actual interface{}) ([]metav1.Condition, error) {
	switch obj := actual.(type) {
	case *dbopsv1alpha1.DatabaseInstance:
		return obj.Status.Conditions, nil
	case *dbopsv1alpha1.Database:
		return obj.Status.Conditions, nil
	case *dbopsv1alpha1.DatabaseUser:
		return obj.Status.Conditions, nil
	case *dbopsv1alpha1.DatabaseRole:
		return obj.Status.Conditions, nil
	case *dbopsv1alpha1.DatabaseGrant:
		return obj.Status.Conditions, nil
	case *dbopsv1alpha1.DatabaseBackup:
		return obj.Status.Conditions, nil
	case *dbopsv1alpha1.DatabaseRestore:
		return obj.Status.Conditions, nil
	case *dbopsv1alpha1.DatabaseBackupSchedule:
		return obj.Status.Conditions, nil
	default:
		return nil, fmt.Errorf("HaveCondition matcher expects a dbops object with status.conditions, got %T", actual)
	}
}

// extractPhase extracts the phase from various object types.
func extractPhase(actual interface{}) (dbopsv1alpha1.Phase, error) {
	switch obj := actual.(type) {
	case *dbopsv1alpha1.DatabaseInstance:
		return obj.Status.Phase, nil
	case *dbopsv1alpha1.Database:
		return obj.Status.Phase, nil
	case *dbopsv1alpha1.DatabaseUser:
		return obj.Status.Phase, nil
	case *dbopsv1alpha1.DatabaseRole:
		return obj.Status.Phase, nil
	case *dbopsv1alpha1.DatabaseGrant:
		return obj.Status.Phase, nil
	case *dbopsv1alpha1.DatabaseBackup:
		return obj.Status.Phase, nil
	case *dbopsv1alpha1.DatabaseRestore:
		return obj.Status.Phase, nil
	case *dbopsv1alpha1.DatabaseBackupSchedule:
		return obj.Status.Phase, nil
	default:
		return "", fmt.Errorf("HavePhase matcher expects a dbops object with status.phase, got %T", actual)
	}
}

// extractObservedGeneration extracts the observedGeneration from various object types.
func extractObservedGeneration(actual interface{}) (int64, error) {
	switch obj := actual.(type) {
	case *dbopsv1alpha1.DatabaseInstance:
		return obj.Status.ObservedGeneration, nil
	case *dbopsv1alpha1.Database:
		return obj.Status.ObservedGeneration, nil
	case *dbopsv1alpha1.DatabaseUser:
		return obj.Status.ObservedGeneration, nil
	case *dbopsv1alpha1.DatabaseRole:
		return obj.Status.ObservedGeneration, nil
	case *dbopsv1alpha1.DatabaseGrant:
		return obj.Status.ObservedGeneration, nil
	default:
		return 0, fmt.Errorf("HaveObservedGeneration matcher expects a dbops object with status.observedGeneration, got %T", actual)
	}
}
