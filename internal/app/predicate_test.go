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

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/event"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

func objWithLabels(labels map[string]string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetName("test-resource")
	obj.SetNamespace("default")
	if labels != nil {
		obj.SetLabels(labels)
	}
	return obj
}

func TestInstanceIDPredicate_DefaultOperator_UnlabeledResource(t *testing.T) {
	p := NewInstanceIDPredicate("default")
	obj := objWithLabels(nil)

	assert.True(t, p.Create(event.CreateEvent{Object: obj}))
	assert.True(t, p.Update(event.UpdateEvent{ObjectNew: obj}))
	assert.True(t, p.Delete(event.DeleteEvent{Object: obj}))
	assert.True(t, p.Generic(event.GenericEvent{Object: obj}))
}

func TestInstanceIDPredicate_DefaultOperator_ExplicitDefault(t *testing.T) {
	p := NewInstanceIDPredicate("default")
	obj := objWithLabels(map[string]string{
		dbopsv1alpha1.LabelOperatorInstanceID: "default",
	})

	assert.True(t, p.Create(event.CreateEvent{Object: obj}))
	assert.True(t, p.Update(event.UpdateEvent{ObjectNew: obj}))
	assert.True(t, p.Delete(event.DeleteEvent{Object: obj}))
	assert.True(t, p.Generic(event.GenericEvent{Object: obj}))
}

func TestInstanceIDPredicate_DefaultOperator_RejectsProduction(t *testing.T) {
	p := NewInstanceIDPredicate("default")
	obj := objWithLabels(map[string]string{
		dbopsv1alpha1.LabelOperatorInstanceID: "production",
	})

	assert.False(t, p.Create(event.CreateEvent{Object: obj}))
	assert.False(t, p.Update(event.UpdateEvent{ObjectNew: obj}))
	assert.False(t, p.Delete(event.DeleteEvent{Object: obj}))
	assert.False(t, p.Generic(event.GenericEvent{Object: obj}))
}

func TestInstanceIDPredicate_ProductionOperator_AcceptsProduction(t *testing.T) {
	p := NewInstanceIDPredicate("production")
	obj := objWithLabels(map[string]string{
		dbopsv1alpha1.LabelOperatorInstanceID: "production",
	})

	assert.True(t, p.Create(event.CreateEvent{Object: obj}))
	assert.True(t, p.Update(event.UpdateEvent{ObjectNew: obj}))
	assert.True(t, p.Delete(event.DeleteEvent{Object: obj}))
	assert.True(t, p.Generic(event.GenericEvent{Object: obj}))
}

func TestInstanceIDPredicate_ProductionOperator_RejectsUnlabeled(t *testing.T) {
	p := NewInstanceIDPredicate("production")
	obj := objWithLabels(nil)

	assert.False(t, p.Create(event.CreateEvent{Object: obj}))
	assert.False(t, p.Update(event.UpdateEvent{ObjectNew: obj}))
	assert.False(t, p.Delete(event.DeleteEvent{Object: obj}))
	assert.False(t, p.Generic(event.GenericEvent{Object: obj}))
}

func TestInstanceIDPredicate_ProductionOperator_RejectsDefault(t *testing.T) {
	p := NewInstanceIDPredicate("production")
	obj := objWithLabels(map[string]string{
		dbopsv1alpha1.LabelOperatorInstanceID: "default",
	})

	assert.False(t, p.Create(event.CreateEvent{Object: obj}))
	assert.False(t, p.Update(event.UpdateEvent{ObjectNew: obj}))
	assert.False(t, p.Delete(event.DeleteEvent{Object: obj}))
	assert.False(t, p.Generic(event.GenericEvent{Object: obj}))
}

func TestInstanceIDPredicate_NilObject(t *testing.T) {
	p := NewInstanceIDPredicate("default")

	assert.False(t, p.Create(event.CreateEvent{Object: nil}))
	assert.False(t, p.Update(event.UpdateEvent{ObjectNew: nil}))
	assert.False(t, p.Delete(event.DeleteEvent{Object: nil}))
	assert.False(t, p.Generic(event.GenericEvent{Object: nil}))
}

func TestInstanceIDPredicate_UpdateUsesNewObject(t *testing.T) {
	p := NewInstanceIDPredicate("production")

	oldObj := objWithLabels(map[string]string{
		dbopsv1alpha1.LabelOperatorInstanceID: "production",
	})
	newObj := objWithLabels(map[string]string{
		dbopsv1alpha1.LabelOperatorInstanceID: "staging",
	})

	// Update checks ObjectNew, not ObjectOld
	assert.False(t, p.Update(event.UpdateEvent{
		ObjectOld: oldObj,
		ObjectNew: newObj,
	}))
}

func TestInstanceIDPredicate_WithTypedObject(t *testing.T) {
	// Verify predicate works with actual API types (not just Unstructured)
	p := NewInstanceIDPredicate("production")
	obj := &metav1.PartialObjectMetadata{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-db",
			Namespace: "default",
			Labels: map[string]string{
				dbopsv1alpha1.LabelOperatorInstanceID: "production",
			},
		},
	}

	assert.True(t, p.Create(event.CreateEvent{Object: obj}))
}
