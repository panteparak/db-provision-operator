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

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

func TestComputeDeletionHash_Deterministic(t *testing.T) {
	children := []string{"Database/mydb", "DatabaseUser/admin"}
	hash1 := ComputeDeletionHash(children)
	hash2 := ComputeDeletionHash(children)
	assert.Equal(t, hash1, hash2, "same input should produce the same hash")
	assert.Len(t, hash1, 8, "hash should be 8 hex characters")
}

func TestComputeDeletionHash_OrderIndependent(t *testing.T) {
	hash1 := ComputeDeletionHash([]string{"Database/mydb", "DatabaseUser/admin"})
	hash2 := ComputeDeletionHash([]string{"DatabaseUser/admin", "Database/mydb"})
	assert.Equal(t, hash1, hash2, "order should not matter")
}

func TestComputeDeletionHash_ChangesWithDifferentChildren(t *testing.T) {
	hash1 := ComputeDeletionHash([]string{"Database/mydb"})
	hash2 := ComputeDeletionHash([]string{"Database/mydb", "DatabaseUser/admin"})
	assert.NotEqual(t, hash1, hash2, "different children should produce different hashes")
}

func TestComputeDeletionHash_EmptyChildren(t *testing.T) {
	hash := ComputeDeletionHash([]string{})
	assert.Len(t, hash, 8, "empty children should still produce a valid hash")
}

func TestIsForceDeleteConfirmed_True(t *testing.T) {
	children := []string{"Database/mydb", "DatabaseUser/admin"}
	hash := ComputeDeletionHash(children)

	obj := newTestObject(map[string]string{
		AnnotationConfirmForceDelete: hash,
	})

	assert.True(t, IsForceDeleteConfirmed(obj, children))
}

func TestIsForceDeleteConfirmed_WrongHash(t *testing.T) {
	children := []string{"Database/mydb", "DatabaseUser/admin"}

	obj := newTestObject(map[string]string{
		AnnotationConfirmForceDelete: "wronghash",
	})

	assert.False(t, IsForceDeleteConfirmed(obj, children))
}

func TestIsForceDeleteConfirmed_NoAnnotation(t *testing.T) {
	children := []string{"Database/mydb"}

	obj := newTestObject(nil)

	assert.False(t, IsForceDeleteConfirmed(obj, children))
}

func TestGetConfirmForceDeleteAnnotation_NilAnnotations(t *testing.T) {
	obj := newTestObject(nil)
	assert.Equal(t, "", GetConfirmForceDeleteAnnotation(obj))
}

func TestGetConfirmForceDeleteAnnotation_EmptyAnnotations(t *testing.T) {
	obj := newTestObject(map[string]string{})
	assert.Equal(t, "", GetConfirmForceDeleteAnnotation(obj))
}

func TestGetConfirmForceDeleteAnnotation_Set(t *testing.T) {
	obj := newTestObject(map[string]string{
		AnnotationConfirmForceDelete: "abc12345",
	})
	assert.Equal(t, "abc12345", GetConfirmForceDeleteAnnotation(obj))
}

func TestAnnotationConfirmForceDelete_Value(t *testing.T) {
	assert.Equal(t, "dbops.dbprovision.io/confirm-force-delete", AnnotationConfirmForceDelete)
}

// newTestObject creates a test client.Object with the given annotations.
func newTestObject(annotations map[string]string) client.Object {
	scheme := runtime.NewScheme()
	_ = dbopsv1alpha1.AddToScheme(scheme)

	obj := &dbopsv1alpha1.DatabaseInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test",
			Namespace:   "default",
			Annotations: annotations,
		},
	}
	return obj
}
