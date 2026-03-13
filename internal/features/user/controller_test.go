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

package user

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/secret"
	"github.com/db-provision-operator/internal/service/drift"
	"github.com/db-provision-operator/internal/util"
)

// testDefaultDriftInterval is used as the default drift interval for controllers
// in unit tests. It must match the value used by the production default.
const testDefaultDriftInterval = 8 * time.Hour

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = dbopsv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	return scheme
}

func newTestUser(name, namespace string) *dbopsv1alpha1.DatabaseUser {
	return &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: name,
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		},
	}
}

func newTestInstance(name, namespace string) *dbopsv1alpha1.DatabaseInstance {
	return &dbopsv1alpha1.DatabaseInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: dbopsv1alpha1.DatabaseInstanceSpec{
			Engine: dbopsv1alpha1.EngineTypePostgres,
			Connection: dbopsv1alpha1.ConnectionConfig{
				Host: "localhost",
				Port: 5432,
			},
		},
		Status: dbopsv1alpha1.DatabaseInstanceStatus{
			Phase: dbopsv1alpha1.PhaseReady,
		},
	}
}

func TestController_Reconcile_NewUser(t *testing.T) {
	scheme := newTestScheme()
	user := newTestUser("testuser", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return false, nil
	}
	mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace, password string) (*Result, error) {
		return &Result{Created: true, Message: "created"}, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:               client,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(client),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testuser",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result) // Should requeue

	// Verify the user was updated
	var updatedUser dbopsv1alpha1.DatabaseUser
	err = client.Get(context.Background(), types.NamespacedName{Name: "testuser", Namespace: "default"}, &updatedUser)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedUser.Status.Phase)
	assert.True(t, mockRepo.WasCalled("Create"))
}

func TestController_Reconcile_ExistingUser(t *testing.T) {
	scheme := newTestScheme()
	user := newTestUser("testuser", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return true, nil // User already exists
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:               client,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(client),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testuser",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	// Verify Create was NOT called since user exists
	assert.False(t, mockRepo.WasCalled("Create"))
	assert.True(t, mockRepo.WasCalled("Exists"))
}

func TestController_Reconcile_UserNotFound(t *testing.T) {
	scheme := newTestScheme()

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	mockRepo := NewMockRepository()
	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:               client,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(client),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "nonexistent",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result) // No requeue for not found
}

func TestController_Reconcile_SkipWithAnnotation(t *testing.T) {
	scheme := newTestScheme()
	user := newTestUser("testuser", "default")
	user.Annotations = map[string]string{
		util.AnnotationSkipReconcile: "true",
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:               client,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(client),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testuser",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify no repository methods were called
	assert.False(t, mockRepo.WasCalled("Exists"))
	assert.False(t, mockRepo.WasCalled("Create"))
}

func TestController_Reconcile_Deletion(t *testing.T) {
	scheme := newTestScheme()
	user := newTestUser("testuser", "default")
	user.Annotations = map[string]string{
		"dbops.dbprovision.io/deletion-policy": string(dbopsv1alpha1.DeletionPolicyDelete),
	}
	user.Finalizers = []string{util.FinalizerDatabaseUser}
	now := metav1.Now()
	user.DeletionTimestamp = &now

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.DeleteFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, force bool) error {
		return nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:               client,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(client),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testuser",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify delete was called
	assert.True(t, mockRepo.WasCalled("Delete"))
}

func TestController_Reconcile_DeletionProtected(t *testing.T) {
	scheme := newTestScheme()
	user := newTestUser("testuser", "default")
	user.Annotations = map[string]string{
		"dbops.dbprovision.io/deletion-protection": "true",
	}
	user.Finalizers = []string{util.FinalizerDatabaseUser}
	now := metav1.Now()
	user.DeletionTimestamp = &now

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:               client,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(client),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testuser",
			Namespace: "default",
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "deletion protection")

	// Verify delete was NOT called
	assert.False(t, mockRepo.WasCalled("Delete"))
}

func TestController_Reconcile_DeletionProtectedWithForceDelete(t *testing.T) {
	scheme := newTestScheme()
	user := newTestUser("testuser", "default")
	user.Annotations = map[string]string{
		"dbops.dbprovision.io/deletion-protection": "true",
		"dbops.dbprovision.io/force-delete":        "true",
		"dbops.dbprovision.io/deletion-policy":     string(dbopsv1alpha1.DeletionPolicyDelete),
	}
	user.Finalizers = []string{util.FinalizerDatabaseUser}
	now := metav1.Now()
	user.DeletionTimestamp = &now

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.DeleteFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, force bool) error {
		return nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:               client,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(client),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testuser",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify delete was called (force delete overrides protection)
	assert.True(t, mockRepo.WasCalled("Delete"))
}

func TestController_Reconcile_SecretTemplateLabelsAndAnnotations(t *testing.T) {
	scheme := newTestScheme()
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testuser",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "testuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
			PasswordSecret: &dbopsv1alpha1.PasswordConfig{
				SecretName: "custom-secret",
				SecretTemplate: &dbopsv1alpha1.SecretTemplate{
					Type: corev1.SecretTypeBasicAuth,
					Labels: map[string]string{
						"custom-label":    "custom-value",
						"app.example.com": "myapp",
					},
					Annotations: map[string]string{
						"secret.reloader.stakater.com/reload": "true",
						"vault.hashicorp.com/agent-inject":    "false",
					},
				},
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return false, nil
	}
	mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace, password string) (*Result, error) {
		return &Result{Created: true, Message: "created"}, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:               client,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(client),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testuser",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	// Verify the secret was created with custom labels, annotations, and type
	var createdSecret corev1.Secret
	err = client.Get(context.Background(), types.NamespacedName{Name: "custom-secret", Namespace: "default"}, &createdSecret)
	require.NoError(t, err)

	// Verify secret type
	assert.Equal(t, corev1.SecretTypeBasicAuth, createdSecret.Type)

	// Verify custom labels are present (merged with defaults)
	assert.Equal(t, "custom-value", createdSecret.Labels["custom-label"])
	assert.Equal(t, "myapp", createdSecret.Labels["app.example.com"])
	assert.Equal(t, "db-provision-operator", createdSecret.Labels["app.kubernetes.io/managed-by"]) // default label preserved
	assert.Equal(t, "testuser", createdSecret.Labels["dbops.dbprovision.io/user"])                 // default label preserved

	// Verify custom annotations are present
	assert.Equal(t, "true", createdSecret.Annotations["secret.reloader.stakater.com/reload"])
	assert.Equal(t, "false", createdSecret.Annotations["vault.hashicorp.com/agent-inject"])
}

func TestController_Reconcile_SecretTemplateOverridesDefaultLabels(t *testing.T) {
	scheme := newTestScheme()
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testuser",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "testuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
			PasswordSecret: &dbopsv1alpha1.PasswordConfig{
				SecretName: "override-secret",
				SecretTemplate: &dbopsv1alpha1.SecretTemplate{
					Labels: map[string]string{
						// Override the default managed-by label
						"app.kubernetes.io/managed-by": "custom-operator",
					},
				},
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return false, nil
	}
	mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace, password string) (*Result, error) {
		return &Result{Created: true, Message: "created"}, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:               client,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(client),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testuser",
			Namespace: "default",
		},
	})

	require.NoError(t, err)

	// Verify the secret has the overridden label
	var createdSecret corev1.Secret
	err = client.Get(context.Background(), types.NamespacedName{Name: "override-secret", Namespace: "default"}, &createdSecret)
	require.NoError(t, err)

	// Verify custom label overrides default
	assert.Equal(t, "custom-operator", createdSecret.Labels["app.kubernetes.io/managed-by"])
	// But other default labels are still present
	assert.Equal(t, "testuser", createdSecret.Labels["dbops.dbprovision.io/user"])
}

func TestController_Reconcile_DeletionBlockedByOwnership(t *testing.T) {
	scheme := newTestScheme()
	user := newTestUser("testuser", "default")
	user.Annotations = map[string]string{
		"dbops.dbprovision.io/deletion-policy": string(dbopsv1alpha1.DeletionPolicyDelete),
	}
	user.Finalizers = []string{util.FinalizerDatabaseUser}
	now := metav1.Now()
	user.DeletionTimestamp = &now

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	// User owns objects - this should block deletion
	mockRepo.GetOwnedObjectsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) ([]OwnedObject, error) {
		return []OwnedObject{
			{Schema: "public", Name: "users_table", Type: "table"},
			{Schema: "public", Name: "user_id_seq", Type: "sequence"},
		}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	recorder := record.NewFakeRecorder(10)
	controller := NewController(ControllerConfig{
		Client:               client,
		Scheme:               scheme,
		Recorder:             recorder,
		Handler:              handler,
		SecretManager:        secret.NewManager(client),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testuser",
			Namespace: "default",
		},
	})

	// Should return error (deletion blocked)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user owns database objects")
	assert.Equal(t, RequeueAfterError, result.RequeueAfter)

	// Verify Delete was NOT called (ownership blocked it)
	assert.False(t, mockRepo.WasCalled("Delete"))

	// Verify GetOwnedObjects was called
	assert.True(t, mockRepo.WasCalled("GetOwnedObjects"))

	// Verify status was updated with ownership block info
	var updatedUser dbopsv1alpha1.DatabaseUser
	err = client.Get(context.Background(), types.NamespacedName{Name: "testuser", Namespace: "default"}, &updatedUser)
	require.NoError(t, err)
	assert.NotNil(t, updatedUser.Status.OwnershipBlock)
	assert.True(t, updatedUser.Status.OwnershipBlock.Blocked)
	assert.Len(t, updatedUser.Status.OwnershipBlock.OwnedObjects, 2)
	assert.Contains(t, updatedUser.Status.OwnershipBlock.Resolution, "REASSIGN OWNED BY")
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedUser.Status.Phase)

	// Verify event was emitted
	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, "DeletionBlocked")
		assert.Contains(t, event, "owns 2 database objects")
	default:
		t.Error("Expected DeletionBlocked event to be emitted")
	}
}

func TestController_Reconcile_DeletionProceedsNoOwnership(t *testing.T) {
	scheme := newTestScheme()
	user := newTestUser("testuser", "default")
	user.Annotations = map[string]string{
		"dbops.dbprovision.io/deletion-policy": string(dbopsv1alpha1.DeletionPolicyDelete),
	}
	user.Finalizers = []string{util.FinalizerDatabaseUser}
	now := metav1.Now()
	user.DeletionTimestamp = &now

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	// User owns NO objects - deletion should proceed
	mockRepo.GetOwnedObjectsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) ([]OwnedObject, error) {
		return []OwnedObject{}, nil
	}
	mockRepo.DeleteFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, force bool) error {
		return nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:               client,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(client),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testuser",
			Namespace: "default",
		},
	})

	// Should succeed
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify ownership check was performed
	assert.True(t, mockRepo.WasCalled("GetOwnedObjects"))

	// Verify Delete WAS called (no ownership blocking)
	assert.True(t, mockRepo.WasCalled("Delete"))
}

func TestController_Reconcile_DeletionWithForceBypassesOwnership(t *testing.T) {
	scheme := newTestScheme()
	user := newTestUser("testuser", "default")
	user.Annotations = map[string]string{
		"dbops.dbprovision.io/deletion-policy": string(dbopsv1alpha1.DeletionPolicyDelete),
		"dbops.dbprovision.io/force-delete":    "true", // Force delete bypasses ownership check
	}
	user.Finalizers = []string{util.FinalizerDatabaseUser}
	now := metav1.Now()
	user.DeletionTimestamp = &now

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	// User owns objects, but force-delete should bypass the check
	mockRepo.GetOwnedObjectsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) ([]OwnedObject, error) {
		return []OwnedObject{
			{Schema: "public", Name: "important_table", Type: "table"},
		}, nil
	}
	mockRepo.DeleteFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, force bool) error {
		return nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:               client,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(client),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testuser",
			Namespace: "default",
		},
	})

	// Should succeed (force bypasses ownership)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify GetOwnedObjects was NOT called (skipped due to force)
	assert.False(t, mockRepo.WasCalled("GetOwnedObjects"))

	// Verify Delete WAS called
	assert.True(t, mockRepo.WasCalled("Delete"))
}

func TestController_Reconcile_SetsReconcileID(t *testing.T) {
	scheme := newTestScheme()
	user := newTestUser("testuser", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return false, nil
	}
	mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace, password string) (*Result, error) {
		return &Result{Created: true, Message: "created"}, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:               client,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(client),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testuser",
			Namespace: "default",
		},
	})

	require.NoError(t, err)

	// Verify the user status has reconcileID set
	var updatedUser dbopsv1alpha1.DatabaseUser
	err = client.Get(context.Background(), types.NamespacedName{Name: "testuser", Namespace: "default"}, &updatedUser)
	require.NoError(t, err)

	// ReconcileID should be an 8-character hex string
	assert.NotEmpty(t, updatedUser.Status.ReconcileID, "ReconcileID should be set")
	assert.Len(t, updatedUser.Status.ReconcileID, 8, "ReconcileID should be 8 characters")
	assert.NotNil(t, updatedUser.Status.LastReconcileTime, "LastReconcileTime should be set")
}

func TestController_Reconcile_ExistsError(t *testing.T) {
	scheme := newTestScheme()
	user := newTestUser("testuser", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return false, fmt.Errorf("connection refused")
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:               client,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(client),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testuser",
			Namespace: "default",
		},
	})

	// The handleError path calls reconcileutil.ClassifyRequeue which may or may not return the error,
	// so we check status instead of the returned error.
	_ = err

	// Verify status was set to failed with appropriate message
	var updatedUser dbopsv1alpha1.DatabaseUser
	err = client.Get(context.Background(), types.NamespacedName{Name: "testuser", Namespace: "default"}, &updatedUser)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedUser.Status.Phase)
	assert.Contains(t, updatedUser.Status.Message, "check existence")

	// Verify Create was NOT called since Exists returned an error
	assert.False(t, mockRepo.WasCalled("Create"))
}

func TestController_Reconcile_CreateError(t *testing.T) {
	scheme := newTestScheme()
	user := newTestUser("testuser", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return false, nil
	}
	mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace, password string) (*Result, error) {
		return nil, fmt.Errorf("permission denied")
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:               client,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(client),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testuser",
			Namespace: "default",
		},
	})

	_ = err

	// Verify status was set to failed with appropriate message
	var updatedUser dbopsv1alpha1.DatabaseUser
	err = client.Get(context.Background(), types.NamespacedName{Name: "testuser", Namespace: "default"}, &updatedUser)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedUser.Status.Phase)
	assert.Contains(t, updatedUser.Status.Message, "create")
}

func TestController_Reconcile_DeletionDeleteError(t *testing.T) {
	scheme := newTestScheme()
	user := newTestUser("testuser", "default")
	user.Annotations = map[string]string{
		"dbops.dbprovision.io/deletion-policy": string(dbopsv1alpha1.DeletionPolicyDelete),
	}
	user.Finalizers = []string{util.FinalizerDatabaseUser}
	now := metav1.Now()
	user.DeletionTimestamp = &now

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.GetOwnedObjectsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) ([]OwnedObject, error) {
		return []OwnedObject{}, nil
	}
	mockRepo.DeleteFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, force bool) error {
		return fmt.Errorf("database unavailable")
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:               client,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(client),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testuser",
			Namespace: "default",
		},
	})

	// Should return error with requeue
	require.Error(t, err)
	assert.True(t, result.RequeueAfter > 0, "Expected RequeueAfter > 0")

	// Verify Delete WAS called
	assert.True(t, mockRepo.WasCalled("Delete"))

	// Verify finalizer NOT removed (still present)
	var updatedUser dbopsv1alpha1.DatabaseUser
	err = client.Get(context.Background(), types.NamespacedName{Name: "testuser", Namespace: "default"}, &updatedUser)
	require.NoError(t, err)
	assert.Contains(t, updatedUser.Finalizers, util.FinalizerDatabaseUser, "Finalizer should still be present after delete error")
}

func TestController_Reconcile_StatusFieldsPopulated(t *testing.T) {
	scheme := newTestScheme()
	user := newTestUser("testuser", "default")
	user.Generation = 3 // Set a non-zero generation to verify ObservedGeneration is not copied

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return false, nil
	}
	mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace, password string) (*Result, error) {
		return &Result{Created: true, Message: "created"}, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:               client,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(client),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testuser",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, testDefaultDriftInterval, result.RequeueAfter, "Should requeue after ready interval")

	// Fetch the updated user to verify all status fields
	var updatedUser dbopsv1alpha1.DatabaseUser
	err = client.Get(context.Background(), types.NamespacedName{Name: "testuser", Namespace: "default"}, &updatedUser)
	require.NoError(t, err)

	// --- Phase and Message ---
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedUser.Status.Phase, "Phase should be Ready")
	assert.Equal(t, "User is ready", updatedUser.Status.Message, "Message should indicate user is ready")

	// --- Secret info ---
	require.NotNil(t, updatedUser.Status.Secret, "Secret status should be populated")
	assert.Equal(t, "testuser-credentials", updatedUser.Status.Secret.Name, "Secret name should be <username>-credentials")
	assert.Equal(t, "default", updatedUser.Status.Secret.Namespace, "Secret namespace should match user namespace")

	// --- User info ---
	// Note: The controller does not currently set Status.User; verify it is nil
	assert.Nil(t, updatedUser.Status.User, "User info is not populated by the controller")

	// --- ReconcileID ---
	assert.NotEmpty(t, updatedUser.Status.ReconcileID, "ReconcileID should be set")
	assert.Len(t, updatedUser.Status.ReconcileID, 8, "ReconcileID should be 8 characters (hex string)")

	// --- LastReconcileTime ---
	assert.NotNil(t, updatedUser.Status.LastReconcileTime, "LastReconcileTime should be set")

	// --- ObservedGeneration ---
	// Note: The controller does not currently set ObservedGeneration; verify it remains at zero-value
	assert.Equal(t, int64(0), updatedUser.Status.ObservedGeneration, "ObservedGeneration is not populated by the controller")

	// --- Conditions ---
	// Verify Ready condition
	readyCondition := util.GetCondition(updatedUser.Status.Conditions, util.ConditionTypeReady)
	require.NotNil(t, readyCondition, "Ready condition should exist")
	assert.Equal(t, metav1.ConditionTrue, readyCondition.Status, "Ready condition should be True")
	assert.Equal(t, util.ReasonReconcileSuccess, readyCondition.Reason, "Ready condition reason should be ReconcileSuccess")
	assert.Equal(t, "User is ready", readyCondition.Message, "Ready condition message should match")

	// Verify Synced condition
	syncedCondition := util.GetCondition(updatedUser.Status.Conditions, util.ConditionTypeSynced)
	require.NotNil(t, syncedCondition, "Synced condition should exist")
	assert.Equal(t, metav1.ConditionTrue, syncedCondition.Status, "Synced condition should be True")
	assert.Equal(t, util.ReasonReconcileSuccess, syncedCondition.Reason, "Synced condition reason should be ReconcileSuccess")
	assert.Equal(t, "User is synced", syncedCondition.Message, "Synced condition message should match")
}

func TestController_Reconcile_DeletionDeleteError_WithForce(t *testing.T) {
	scheme := newTestScheme()
	user := newTestUser("testuser", "default")
	user.Annotations = map[string]string{
		"dbops.dbprovision.io/deletion-policy": string(dbopsv1alpha1.DeletionPolicyDelete),
		"dbops.dbprovision.io/force-delete":    "true",
	}
	user.Finalizers = []string{util.FinalizerDatabaseUser}
	now := metav1.Now()
	user.DeletionTimestamp = &now

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.GetOwnedObjectsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) ([]OwnedObject, error) {
		return []OwnedObject{}, nil
	}
	mockRepo.DeleteFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, force bool) error {
		return fmt.Errorf("database unavailable")
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:               client,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(client),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testuser",
			Namespace: "default",
		},
	})

	// With force-delete, error is swallowed and finalizer is removed
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify Delete WAS called
	assert.True(t, mockRepo.WasCalled("Delete"))
}

// --- Drift detection tests ---

func TestController_Reconcile_DriftDetected_DetectMode(t *testing.T) {
	scheme := newTestScheme()
	user := newTestUser("testuser", "default")
	// Default drift policy is detect mode (no DriftPolicy set on spec)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return true, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		result := drift.NewResult("user", spec.Username)
		result.AddDiff(drift.Diff{
			Field:    "connectionLimit",
			Expected: "10",
			Actual:   "5",
		})
		return result, nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:               client,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(client),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testuser",
			Namespace: "default",
		},
	})
	require.NoError(t, err)

	// In detect mode (default), drift is logged but CorrectDrift is NOT called
	assert.False(t, mockRepo.WasCalled("CorrectDrift"))

	var updatedUser dbopsv1alpha1.DatabaseUser
	err = client.Get(context.Background(), types.NamespacedName{Name: "testuser", Namespace: "default"}, &updatedUser)
	require.NoError(t, err)
	assert.NotNil(t, updatedUser.Status.Drift)
	assert.True(t, updatedUser.Status.Drift.Detected)
}

func TestController_Reconcile_DriftDetected_CorrectMode(t *testing.T) {
	scheme := newTestScheme()
	user := newTestUser("testuser", "default")
	user.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{
		Mode: dbopsv1alpha1.DriftModeCorrect,
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	driftResult := drift.NewResult("user", "testuser")
	driftResult.AddDiff(drift.Diff{
		Field:    "connectionLimit",
		Expected: "10",
		Actual:   "5",
	})

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return true, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		return driftResult, nil
	}
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, dr *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		cr := drift.NewCorrectionResult(spec.Username)
		cr.AddCorrected(driftResult.Diffs[0])
		return cr, nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:               client,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(client),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testuser",
			Namespace: "default",
		},
	})
	require.NoError(t, err)

	// In correct mode, CorrectDrift should be called
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))
}

func TestController_Reconcile_DriftCorrection_PartialFail(t *testing.T) {
	scheme := newTestScheme()
	user := newTestUser("testuser", "default")
	user.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{
		Mode: dbopsv1alpha1.DriftModeCorrect,
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	connectionLimitDiff := drift.Diff{
		Field:    "connectionLimit",
		Expected: "10",
		Actual:   "5",
	}
	superuserDiff := drift.Diff{
		Field:    "superuser",
		Expected: "false",
		Actual:   "true",
	}

	driftResult := drift.NewResult("user", "testuser")
	driftResult.AddDiff(connectionLimitDiff)
	driftResult.AddDiff(superuserDiff)

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return true, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		return driftResult, nil
	}
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, dr *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		cr := drift.NewCorrectionResult(spec.Username)
		cr.AddCorrected(connectionLimitDiff)
		cr.AddFailed(superuserDiff, fmt.Errorf("permission denied"))
		return cr, nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:               client,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(client),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testuser",
			Namespace: "default",
		},
	})

	// Partial failure does not fail the reconcile
	require.NoError(t, err)
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))

	var updatedUser dbopsv1alpha1.DatabaseUser
	err = client.Get(context.Background(), types.NamespacedName{Name: "testuser", Namespace: "default"}, &updatedUser)
	require.NoError(t, err)
	// Phase is still Ready because drift correction failures do not fail reconcile
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedUser.Status.Phase)
}

func TestController_Reconcile_DriftCorrection_AllFailed(t *testing.T) {
	scheme := newTestScheme()
	user := newTestUser("testuser", "default")
	user.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{
		Mode: dbopsv1alpha1.DriftModeCorrect,
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	connectionLimitDiff := drift.Diff{
		Field:    "connectionLimit",
		Expected: "10",
		Actual:   "5",
	}

	driftResult := drift.NewResult("user", "testuser")
	driftResult.AddDiff(connectionLimitDiff)

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return true, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		return driftResult, nil
	}
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, dr *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		cr := drift.NewCorrectionResult(spec.Username)
		cr.AddFailed(connectionLimitDiff, fmt.Errorf("database connection lost"))
		return cr, nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:               client,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(client),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testuser",
			Namespace: "default",
		},
	})

	// All corrections failed but reconcile still succeeds (drift errors are non-fatal)
	require.NoError(t, err)
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))

	var updatedUser dbopsv1alpha1.DatabaseUser
	err = client.Get(context.Background(), types.NamespacedName{Name: "testuser", Namespace: "default"}, &updatedUser)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedUser.Status.Phase)
	// Drift status should still show detected since corrections failed (not cleared)
	assert.NotNil(t, updatedUser.Status.Drift)
	assert.True(t, updatedUser.Status.Drift.Detected)
}

func TestController_Reconcile_DriftDetected_IgnoreMode(t *testing.T) {
	scheme := newTestScheme()
	user := newTestUser("testuser", "default")
	user.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{
		Mode: dbopsv1alpha1.DriftModeIgnore,
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return true, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:               client,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(client),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testuser",
			Namespace: "default",
		},
	})
	require.NoError(t, err)

	// In ignore mode, DetectDrift should NOT be called
	assert.False(t, mockRepo.WasCalled("DetectDrift"))
}

func TestController_Reconcile_DriftDetection_Error(t *testing.T) {
	scheme := newTestScheme()
	user := newTestUser("testuser", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return true, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		return nil, fmt.Errorf("drift detection failed")
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:               client,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(client),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testuser",
			Namespace: "default",
		},
	})

	// Drift detection error should NOT fail the reconcile
	require.NoError(t, err)

	var updatedUser dbopsv1alpha1.DatabaseUser
	err = client.Get(context.Background(), types.NamespacedName{Name: "testuser", Namespace: "default"}, &updatedUser)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedUser.Status.Phase)
}

func TestController_Reconcile_DriftCorrection_Destructive(t *testing.T) {
	scheme := newTestScheme()
	user := newTestUser("testuser", "default")
	user.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{
		Mode: dbopsv1alpha1.DriftModeCorrect,
	}
	user.Annotations = map[string]string{
		dbopsv1alpha1.AnnotationAllowDestructiveDrift: "true",
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	destructiveDiff := drift.Diff{
		Field:       "superuser",
		Expected:    "true",
		Actual:      "false",
		Destructive: true,
	}

	driftResult := drift.NewResult("user", "testuser")
	driftResult.AddDiff(destructiveDiff)

	var capturedAllowDestructive bool
	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return true, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		capturedAllowDestructive = allowDestructive
		return driftResult, nil
	}
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, dr *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		cr := drift.NewCorrectionResult(spec.Username)
		if allowDestructive {
			cr.AddCorrected(destructiveDiff)
		} else {
			cr.AddSkipped(destructiveDiff, "destructive correction not allowed")
		}
		return cr, nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:               client,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(client),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testuser",
			Namespace: "default",
		},
	})
	require.NoError(t, err)

	// allowDestructive should be true because of the annotation
	assert.True(t, capturedAllowDestructive)
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))
}

func TestController_Reconcile_DriftCorrection_NoDestructive(t *testing.T) {
	scheme := newTestScheme()
	user := newTestUser("testuser", "default")
	user.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{
		Mode: dbopsv1alpha1.DriftModeCorrect,
	}
	// No allow-destructive-drift annotation

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	destructiveDiff := drift.Diff{
		Field:       "superuser",
		Expected:    "true",
		Actual:      "false",
		Destructive: true,
	}

	driftResult := drift.NewResult("user", "testuser")
	driftResult.AddDiff(destructiveDiff)

	var capturedAllowDestructive bool
	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return true, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		capturedAllowDestructive = allowDestructive
		return driftResult, nil
	}
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, dr *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		cr := drift.NewCorrectionResult(spec.Username)
		// Without destructive allowed, skips destructive changes
		cr.AddSkipped(destructiveDiff, "destructive correction not allowed")
		return cr, nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:               client,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(client),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testuser",
			Namespace: "default",
		},
	})
	require.NoError(t, err)

	// allowDestructive should be false (no annotation)
	assert.False(t, capturedAllowDestructive)
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))

	var updatedUser dbopsv1alpha1.DatabaseUser
	err = client.Get(context.Background(), types.NamespacedName{Name: "testuser", Namespace: "default"}, &updatedUser)
	require.NoError(t, err)
	// Drift should still be detected since the correction was skipped
	assert.NotNil(t, updatedUser.Status.Drift)
	assert.True(t, updatedUser.Status.Drift.Detected)
}

// --- Grant dependency deletion tests ---

func TestController_Reconcile_DeletionBlockedByGrantDependencies(t *testing.T) {
	scheme := newTestScheme()
	user := newTestUser("testuser", "default")
	// Default deletion policy is Retain (no annotation), finalizer + deletionTimestamp
	user.Finalizers = []string{util.FinalizerDatabaseUser}
	now := metav1.Now()
	user.DeletionTimestamp = &now

	// Create a DatabaseGrant that references this user
	grant := &dbopsv1alpha1.DatabaseGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-grant",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseGrantSpec{
			UserRef:     &dbopsv1alpha1.UserReference{Name: user.Name},
			DatabaseRef: &dbopsv1alpha1.DatabaseReference{Name: "somedb"},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user, grant).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	recorder := record.NewFakeRecorder(10)
	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             recorder,
		Handler:              handler,
		SecretManager:        secret.NewManager(fakeClient),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testuser",
			Namespace: "default",
		},
	})

	// Should requeue after 10s with no error (grant dependency blocks deletion)
	require.NoError(t, err)
	assert.Equal(t, 10*time.Second, result.RequeueAfter)

	// Verify finalizer is NOT removed
	var updatedUser dbopsv1alpha1.DatabaseUser
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testuser", Namespace: "default"}, &updatedUser)
	require.NoError(t, err)
	assert.Contains(t, updatedUser.Finalizers, util.FinalizerDatabaseUser, "Finalizer should still be present")

	// Verify Phase=Failed and DependenciesExist condition
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedUser.Status.Phase)
	readyCondition := util.GetCondition(updatedUser.Status.Conditions, util.ConditionTypeReady)
	require.NotNil(t, readyCondition, "Ready condition should exist")
	assert.Equal(t, util.ReasonDependenciesExist, readyCondition.Reason)

	// Verify Delete was NOT called
	assert.False(t, mockRepo.WasCalled("Delete"))
}

func TestController_Reconcile_DeletionSucceedsWhenNoGrants(t *testing.T) {
	scheme := newTestScheme()
	user := newTestUser("testuser", "default")
	// Default deletion policy is Retain, finalizer + deletionTimestamp, no grants
	user.Finalizers = []string{util.FinalizerDatabaseUser}
	now := metav1.Now()
	user.DeletionTimestamp = &now

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(fakeClient),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testuser",
			Namespace: "default",
		},
	})

	// Should succeed with empty result (Retain policy, no grants, finalizer removed)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify finalizer is removed (user should be gone or finalizer cleared)
	var updatedUser dbopsv1alpha1.DatabaseUser
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testuser", Namespace: "default"}, &updatedUser)
	if apierrors.IsNotFound(err) {
		// User was garbage collected after finalizer removal — acceptable
		return
	}
	require.NoError(t, err)
	assert.NotContains(t, updatedUser.Finalizers, util.FinalizerDatabaseUser, "Finalizer should be removed")
}

func TestController_Reconcile_ForceDeleteBypassesGrantCheck(t *testing.T) {
	scheme := newTestScheme()
	user := newTestUser("testuser", "default")
	user.Annotations = map[string]string{
		util.AnnotationForceDelete:             "true",
		"dbops.dbprovision.io/deletion-policy": string(dbopsv1alpha1.DeletionPolicyDelete),
	}
	user.Finalizers = []string{util.FinalizerDatabaseUser}
	now := metav1.Now()
	user.DeletionTimestamp = &now

	// Create a DatabaseGrant referencing this user
	grant := &dbopsv1alpha1.DatabaseGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-grant",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseGrantSpec{
			UserRef:     &dbopsv1alpha1.UserReference{Name: user.Name},
			DatabaseRef: &dbopsv1alpha1.DatabaseReference{Name: "somedb"},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user, grant).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.DeleteFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, force bool) error {
		return nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(fakeClient),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testuser",
			Namespace: "default",
		},
	})

	// Should succeed — force-delete bypasses grant dependency check
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify Delete WAS called (force delete proceeds despite grant)
	assert.True(t, mockRepo.WasCalled("Delete"))

	// Verify finalizer is removed
	var updatedUser dbopsv1alpha1.DatabaseUser
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testuser", Namespace: "default"}, &updatedUser)
	if apierrors.IsNotFound(err) {
		return
	}
	require.NoError(t, err)
	assert.NotContains(t, updatedUser.Finalizers, util.FinalizerDatabaseUser, "Finalizer should be removed")
}

func TestController_Reconcile_SecretTemplateDataReplacesDefaultKeys(t *testing.T) {
	scheme := newTestScheme()
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testuser",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "myuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
			PasswordSecret: &dbopsv1alpha1.PasswordConfig{
				Generate:   true,
				SecretName: "custom-creds",
				SecretTemplate: &dbopsv1alpha1.SecretTemplate{
					Data: map[string]string{
						"DATABASE_URL": "postgresql://{{ .Username }}:{{ .Password }}@{{ .Host }}:{{ .Port }}/{{ .Database }}",
						"JDBC_URL":     "jdbc:postgresql://{{ .Host }}:{{ .Port }}/{{ .Database }}",
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return false, nil
	}
	mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace, password string) (*Result, error) {
		return &Result{Created: true}, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return &dbopsv1alpha1.DatabaseInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "test-instance", Namespace: namespace},
			Spec: dbopsv1alpha1.DatabaseInstanceSpec{
				Engine: dbopsv1alpha1.EngineTypePostgres,
				Connection: dbopsv1alpha1.ConnectionConfig{
					Host:     "db.example.com",
					Port:     5432,
					Database: "mydb",
				},
			},
		}, nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{repo: mockRepo, eventBus: NewMockEventBus(), logger: logr.Discard()}
	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(fakeClient),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testuser", Namespace: "default"},
	})
	require.NoError(t, err)

	// Verify the secret has template-rendered keys, NOT default keys
	var credSecret corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "custom-creds", Namespace: "default"}, &credSecret)
	require.NoError(t, err)

	// Should have template-defined keys (fake client stores StringData, not Data)
	assert.Contains(t, credSecret.StringData, "DATABASE_URL")
	assert.Contains(t, credSecret.StringData, "JDBC_URL")

	// Should NOT have default keys
	assert.NotContains(t, credSecret.StringData, "username")
	assert.NotContains(t, credSecret.StringData, "password")
	assert.NotContains(t, credSecret.StringData, "host")
	assert.NotContains(t, credSecret.StringData, "port")

	// Verify template rendered correctly
	assert.Contains(t, credSecret.StringData["JDBC_URL"], "jdbc:postgresql://db.example.com:5432/mydb")
}

func TestController_Reconcile_EmptySecretTemplateDataUsesDefaults(t *testing.T) {
	scheme := newTestScheme()
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testuser",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "myuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
			PasswordSecret: &dbopsv1alpha1.PasswordConfig{
				Generate:   true,
				SecretName: "default-creds",
				SecretTemplate: &dbopsv1alpha1.SecretTemplate{
					Labels: map[string]string{"app": "test"},
					// Data is nil — should use default keys
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return false, nil
	}
	mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace, password string) (*Result, error) {
		return &Result{Created: true}, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{repo: mockRepo, eventBus: NewMockEventBus(), logger: logr.Discard()}
	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(fakeClient),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testuser", Namespace: "default"},
	})
	require.NoError(t, err)

	// Verify default keys exist
	var credSecret corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "default-creds", Namespace: "default"}, &credSecret)
	require.NoError(t, err)

	assert.Contains(t, credSecret.StringData, "username")
	assert.Contains(t, credSecret.StringData, "password")
	assert.Contains(t, credSecret.StringData, "host")
	assert.Contains(t, credSecret.StringData, "port")
}

func TestController_Reconcile_SecretTemplateDataWithTLS(t *testing.T) {
	scheme := newTestScheme()
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testuser",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "secureuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
			PasswordSecret: &dbopsv1alpha1.PasswordConfig{
				Generate:   true,
				SecretName: "tls-creds",
				SecretTemplate: &dbopsv1alpha1.SecretTemplate{
					Data: map[string]string{
						"ca.crt":       "{{ .CA }}",
						"tls.crt":      "{{ .TLSCert }}",
						"tls.key":      "{{ .TLSKey }}",
						"DATABASE_URL": "postgresql://{{ .Username }}@{{ .Host }}:{{ .Port }}/{{ .Database }}?sslmode={{ .SSLMode }}",
					},
				},
			},
		},
	}

	// Create the TLS secret that the instance references
	tlsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "instance-tls-certs",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt":  []byte("test-ca-cert"),
			"tls.crt": []byte("test-client-cert"),
			"tls.key": []byte("test-client-key"),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user, tlsSecret).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return false, nil
	}
	mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace, password string) (*Result, error) {
		return &Result{Created: true}, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return &dbopsv1alpha1.DatabaseInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "test-instance", Namespace: namespace},
			Spec: dbopsv1alpha1.DatabaseInstanceSpec{
				Engine: dbopsv1alpha1.EngineTypePostgres,
				Connection: dbopsv1alpha1.ConnectionConfig{
					Host:     "db.example.com",
					Port:     5432,
					Database: "mydb",
				},
				TLS: &dbopsv1alpha1.TLSConfig{
					Enabled: true,
					Mode:    "verify-full",
					SecretRef: &dbopsv1alpha1.TLSSecretRef{
						Name: "instance-tls-certs",
					},
				},
			},
		}, nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{repo: mockRepo, eventBus: NewMockEventBus(), logger: logr.Discard()}
	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(fakeClient),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testuser", Namespace: "default"},
	})
	require.NoError(t, err)

	// Verify TLS certs were rendered into the secret
	var credSecret corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "tls-creds", Namespace: "default"}, &credSecret)
	require.NoError(t, err)

	assert.Equal(t, "test-ca-cert", credSecret.StringData["ca.crt"])
	assert.Equal(t, "test-client-cert", credSecret.StringData["tls.crt"])
	assert.Equal(t, "test-client-key", credSecret.StringData["tls.key"])
	assert.Contains(t, credSecret.StringData["DATABASE_URL"], "sslmode=verify-full")
}

func TestController_Reconcile_SecretTemplateDataWithUrlEncode(t *testing.T) {
	scheme := newTestScheme()
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testuser",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "myuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
			PasswordSecret: &dbopsv1alpha1.PasswordConfig{
				Generate:   true,
				SecretName: "encoded-creds",
				SecretTemplate: &dbopsv1alpha1.SecretTemplate{
					Data: map[string]string{
						"DSN": `postgresql://{{ urlEncode .Username }}:{{ urlEncode .Password }}@{{ .Host }}:{{ .Port }}/{{ .Database }}`,
					},
				},
			},
		},
	}

	// Pre-create the secret with a known password containing special chars
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "encoded-creds",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"password": []byte("p@ss!w0rd"),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user, existingSecret).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return true, nil // User already exists
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return &dbopsv1alpha1.DatabaseInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "test-instance", Namespace: namespace},
			Spec: dbopsv1alpha1.DatabaseInstanceSpec{
				Engine: dbopsv1alpha1.EngineTypePostgres,
				Connection: dbopsv1alpha1.ConnectionConfig{
					Host:     "db.example.com",
					Port:     5432,
					Database: "mydb",
				},
			},
		}, nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{repo: mockRepo, eventBus: NewMockEventBus(), logger: logr.Discard()}
	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(fakeClient),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testuser", Namespace: "default"},
	})
	require.NoError(t, err)

	// Verify URL encoding in the DSN
	var credSecret corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "encoded-creds", Namespace: "default"}, &credSecret)
	require.NoError(t, err)

	dsn := credSecret.StringData["DSN"]
	assert.Contains(t, dsn, "p%40ss%21w0rd") // URL-encoded password
	assert.Contains(t, dsn, "db.example.com:5432/mydb")
}

func TestController_Reconcile_TLSSecretMissing_GracefulDegradation(t *testing.T) {
	scheme := newTestScheme()
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testuser",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "myuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
			PasswordSecret: &dbopsv1alpha1.PasswordConfig{
				Generate:   true,
				SecretName: "graceful-creds",
				SecretTemplate: &dbopsv1alpha1.SecretTemplate{
					Data: map[string]string{
						"ca.crt":       "{{ .CA }}",
						"DATABASE_URL": "postgresql://{{ .Username }}@{{ .Host }}:{{ .Port }}/{{ .Database }}",
					},
				},
			},
		},
	}

	// No TLS secret created — it's missing

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return false, nil
	}
	mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace, password string) (*Result, error) {
		return &Result{Created: true}, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return &dbopsv1alpha1.DatabaseInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "test-instance", Namespace: namespace},
			Spec: dbopsv1alpha1.DatabaseInstanceSpec{
				Engine: dbopsv1alpha1.EngineTypePostgres,
				Connection: dbopsv1alpha1.ConnectionConfig{
					Host:     "db.example.com",
					Port:     5432,
					Database: "mydb",
				},
				TLS: &dbopsv1alpha1.TLSConfig{
					Enabled: true,
					Mode:    "verify-full",
					SecretRef: &dbopsv1alpha1.TLSSecretRef{
						Name: "nonexistent-tls-secret", // This doesn't exist
					},
				},
			},
		}, nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{repo: mockRepo, eventBus: NewMockEventBus(), logger: logr.Discard()}
	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(fakeClient),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	// Should succeed even though TLS secret is missing (graceful degradation)
	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testuser", Namespace: "default"},
	})
	require.NoError(t, err)

	// Verify secret was still created with empty TLS fields
	var credSecret corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "graceful-creds", Namespace: "default"}, &credSecret)
	require.NoError(t, err)

	// CA should be empty since TLS secret was missing
	assert.Equal(t, "", credSecret.StringData["ca.crt"])
	// DATABASE_URL should still render correctly
	assert.Contains(t, credSecret.StringData["DATABASE_URL"], "db.example.com:5432/mydb")
}

// ===== Controller SecretTemplate Edge Cases (C1-C17) =====

// C1: SecretTemplate with multi-key data + labels
func TestController_Reconcile_SecretTemplateMultiKeyWithLabels(t *testing.T) {
	scheme := newTestScheme()
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testuser",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "myuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
			PasswordSecret: &dbopsv1alpha1.PasswordConfig{
				Generate:   true,
				SecretName: "multi-key-creds",
				SecretTemplate: &dbopsv1alpha1.SecretTemplate{
					Labels: map[string]string{"app": "test", "env": "dev"},
					Data: map[string]string{
						"DATABASE_URL": "postgresql://{{ .Username }}@{{ .Host }}:{{ .Port }}/{{ .Database }}",
						"APP_USER":     "{{ .Username }}",
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return false, nil
	}
	mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace, password string) (*Result, error) {
		return &Result{Created: true}, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return &dbopsv1alpha1.DatabaseInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "test-instance", Namespace: namespace},
			Spec: dbopsv1alpha1.DatabaseInstanceSpec{
				Engine: dbopsv1alpha1.EngineTypePostgres,
				Connection: dbopsv1alpha1.ConnectionConfig{
					Host: "db.example.com", Port: 5432, Database: "mydb",
				},
			},
		}, nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{repo: mockRepo, eventBus: NewMockEventBus(), logger: logr.Discard()}
	controller := NewController(ControllerConfig{
		Client: fakeClient, Scheme: scheme, Recorder: record.NewFakeRecorder(10),
		Handler: handler, SecretManager: secret.NewManager(fakeClient),
		DefaultDriftInterval: testDefaultDriftInterval, Logger: logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testuser", Namespace: "default"},
	})
	require.NoError(t, err)

	var credSecret corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "multi-key-creds", Namespace: "default"}, &credSecret)
	require.NoError(t, err)

	// Both template keys rendered
	assert.Contains(t, credSecret.StringData, "DATABASE_URL")
	assert.Contains(t, credSecret.StringData, "APP_USER")
	assert.Equal(t, "myuser", credSecret.StringData["APP_USER"])

	// Labels present
	assert.Equal(t, "test", credSecret.Labels["app"])
	assert.Equal(t, "dev", credSecret.Labels["env"])
}

// C2: SecretTemplate with .Database field
func TestController_Reconcile_SecretTemplateWithDatabaseField(t *testing.T) {
	scheme := newTestScheme()
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testuser",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "myuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
			PasswordSecret: &dbopsv1alpha1.PasswordConfig{
				Generate:   true,
				SecretName: "db-field-creds",
				SecretTemplate: &dbopsv1alpha1.SecretTemplate{
					Data: map[string]string{
						"database": "{{ .Database }}",
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return false, nil
	}
	mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace, password string) (*Result, error) {
		return &Result{Created: true}, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return &dbopsv1alpha1.DatabaseInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "test-instance", Namespace: namespace},
			Spec: dbopsv1alpha1.DatabaseInstanceSpec{
				Engine: dbopsv1alpha1.EngineTypePostgres,
				Connection: dbopsv1alpha1.ConnectionConfig{
					Host: "db.example.com", Port: 5432, Database: "production_db",
				},
			},
		}, nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{repo: mockRepo, eventBus: NewMockEventBus(), logger: logr.Discard()}
	controller := NewController(ControllerConfig{
		Client: fakeClient, Scheme: scheme, Recorder: record.NewFakeRecorder(10),
		Handler: handler, SecretManager: secret.NewManager(fakeClient),
		DefaultDriftInterval: testDefaultDriftInterval, Logger: logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testuser", Namespace: "default"},
	})
	require.NoError(t, err)

	var credSecret corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "db-field-creds", Namespace: "default"}, &credSecret)
	require.NoError(t, err)
	assert.Equal(t, "production_db", credSecret.StringData["database"])
}

// C3: SecretTemplate update — changed template re-renders secret
func TestController_Reconcile_SecretTemplateUpdateReRendersSecret(t *testing.T) {
	scheme := newTestScheme()
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testuser",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "myuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
			PasswordSecret: &dbopsv1alpha1.PasswordConfig{
				Generate:   true,
				SecretName: "update-creds",
				SecretTemplate: &dbopsv1alpha1.SecretTemplate{
					Data: map[string]string{
						"DSN": "postgresql://{{ .Username }}@{{ .Host }}:{{ .Port }}/{{ .Database }}",
					},
				},
			},
		},
	}

	// Pre-create the secret with old data
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "update-creds",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"password": []byte("oldpass"),
			"DSN":      []byte("old-dsn-value"),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user, existingSecret).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return true, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return &dbopsv1alpha1.DatabaseInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "test-instance", Namespace: namespace},
			Spec: dbopsv1alpha1.DatabaseInstanceSpec{
				Engine: dbopsv1alpha1.EngineTypePostgres,
				Connection: dbopsv1alpha1.ConnectionConfig{
					Host: "new-host.example.com", Port: 5432, Database: "newdb",
				},
			},
		}, nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{repo: mockRepo, eventBus: NewMockEventBus(), logger: logr.Discard()}
	controller := NewController(ControllerConfig{
		Client: fakeClient, Scheme: scheme, Recorder: record.NewFakeRecorder(10),
		Handler: handler, SecretManager: secret.NewManager(fakeClient),
		DefaultDriftInterval: testDefaultDriftInterval, Logger: logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testuser", Namespace: "default"},
	})
	require.NoError(t, err)

	var credSecret corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "update-creds", Namespace: "default"}, &credSecret)
	require.NoError(t, err)

	// Secret should be updated with new template-rendered data
	assert.Contains(t, credSecret.StringData["DSN"], "new-host.example.com:5432/newdb")
	// Old default key should NOT be present since template Data is set
	assert.NotContains(t, credSecret.StringData, "password")
}

// C4: hasSecretTemplateData with nil PasswordSecret
func TestHasSecretTemplateData_NilPasswordSecret(t *testing.T) {
	user := &dbopsv1alpha1.DatabaseUser{
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			PasswordSecret: nil,
		},
	}
	assert.False(t, hasSecretTemplateData(user))
}

// C5: hasSecretTemplateData with empty SecretTemplate
func TestHasSecretTemplateData_EmptySecretTemplate(t *testing.T) {
	user := &dbopsv1alpha1.DatabaseUser{
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			PasswordSecret: &dbopsv1alpha1.PasswordConfig{
				SecretTemplate: &dbopsv1alpha1.SecretTemplate{},
			},
		},
	}
	assert.False(t, hasSecretTemplateData(user))
}

// C6: SecretTemplate with .Namespace and .Name
func TestController_Reconcile_SecretTemplateWithNamespaceAndName(t *testing.T) {
	scheme := newTestScheme()
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testuser",
			Namespace: "my-namespace",
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "myuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
			PasswordSecret: &dbopsv1alpha1.PasswordConfig{
				Generate:   true,
				SecretName: "ns-name-creds",
				SecretTemplate: &dbopsv1alpha1.SecretTemplate{
					Data: map[string]string{
						"metadata": "ns={{ .Namespace }},name={{ .Name }}",
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return false, nil
	}
	mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace, password string) (*Result, error) {
		return &Result{Created: true}, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{repo: mockRepo, eventBus: NewMockEventBus(), logger: logr.Discard()}
	controller := NewController(ControllerConfig{
		Client: fakeClient, Scheme: scheme, Recorder: record.NewFakeRecorder(10),
		Handler: handler, SecretManager: secret.NewManager(fakeClient),
		DefaultDriftInterval: testDefaultDriftInterval, Logger: logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testuser", Namespace: "my-namespace"},
	})
	require.NoError(t, err)

	var credSecret corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "ns-name-creds", Namespace: "my-namespace"}, &credSecret)
	require.NoError(t, err)
	assert.Equal(t, "ns=my-namespace,name=testuser", credSecret.StringData["metadata"])
}

// C7: SecretTemplate render failure (invalid syntax)
func TestController_Reconcile_SecretTemplateInvalidSyntax(t *testing.T) {
	scheme := newTestScheme()
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testuser",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "myuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
			PasswordSecret: &dbopsv1alpha1.PasswordConfig{
				Generate:   true,
				SecretName: "bad-syntax-creds",
				SecretTemplate: &dbopsv1alpha1.SecretTemplate{
					Data: map[string]string{
						"bad": "{{ .Username ",
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return false, nil
	}
	mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace, password string) (*Result, error) {
		return &Result{Created: true}, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{repo: mockRepo, eventBus: NewMockEventBus(), logger: logr.Discard()}
	controller := NewController(ControllerConfig{
		Client: fakeClient, Scheme: scheme, Recorder: record.NewFakeRecorder(10),
		Handler: handler, SecretManager: secret.NewManager(fakeClient),
		DefaultDriftInterval: testDefaultDriftInterval, Logger: logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testuser", Namespace: "default"},
	})
	// Template parse error should propagate
	require.Error(t, err)
	assert.Contains(t, err.Error(), "render secret template")
}

// C8: SecretTemplate render failure (undefined field reference)
func TestController_Reconcile_SecretTemplateUndefinedFieldError(t *testing.T) {
	scheme := newTestScheme()
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testuser",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "myuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
			PasswordSecret: &dbopsv1alpha1.PasswordConfig{
				Generate:   true,
				SecretName: "func-err-creds",
				SecretTemplate: &dbopsv1alpha1.SecretTemplate{
					Data: map[string]string{
						"bad": `{{ .NonexistentField }}`,
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return false, nil
	}
	mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace, password string) (*Result, error) {
		return &Result{Created: true}, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{repo: mockRepo, eventBus: NewMockEventBus(), logger: logr.Discard()}
	controller := NewController(ControllerConfig{
		Client: fakeClient, Scheme: scheme, Recorder: record.NewFakeRecorder(10),
		Handler: handler, SecretManager: secret.NewManager(fakeClient),
		DefaultDriftInterval: testDefaultDriftInterval, Logger: logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testuser", Namespace: "default"},
	})
	// Undefined field reference should cause template execution error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "render secret template")
}

// C9: Instance not found during reconcile
func TestController_Reconcile_InstanceNotFound(t *testing.T) {
	scheme := newTestScheme()
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testuser",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "myuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "nonexistent-instance",
			},
			PasswordSecret: &dbopsv1alpha1.PasswordConfig{
				Generate:   true,
				SecretName: "no-instance-creds",
				SecretTemplate: &dbopsv1alpha1.SecretTemplate{
					Data: map[string]string{
						"DSN": "postgresql://{{ .Username }}@{{ .Host }}",
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return nil, fmt.Errorf("DatabaseInstance nonexistent-instance not found")
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "", fmt.Errorf("DatabaseInstance nonexistent-instance not found")
	}

	handler := &Handler{repo: mockRepo, eventBus: NewMockEventBus(), logger: logr.Discard()}
	controller := NewController(ControllerConfig{
		Client: fakeClient, Scheme: scheme, Recorder: record.NewFakeRecorder(10),
		Handler: handler, SecretManager: secret.NewManager(fakeClient),
		DefaultDriftInterval: testDefaultDriftInterval, Logger: logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testuser", Namespace: "default"},
	})
	// Instance not found should cause reconcile error
	require.Error(t, err)
}

// C10: TLS enabled but only CA present (no cert/key in TLS secret)
func TestController_Reconcile_TLSOnlyCAPresentInSecret(t *testing.T) {
	scheme := newTestScheme()
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testuser",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "myuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
			PasswordSecret: &dbopsv1alpha1.PasswordConfig{
				Generate:   true,
				SecretName: "ca-only-creds",
				SecretTemplate: &dbopsv1alpha1.SecretTemplate{
					Data: map[string]string{
						"ca.crt":  "{{ .CA }}",
						"tls.crt": "{{ .TLSCert }}",
						"tls.key": "{{ .TLSKey }}",
					},
				},
			},
		},
	}

	// TLS secret with only CA
	tlsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "ca-only-tls", Namespace: "default"},
		Data: map[string][]byte{
			"ca.crt": []byte("test-ca-only"),
			// tls.crt and tls.key are absent
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user, tlsSecret).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return false, nil
	}
	mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace, password string) (*Result, error) {
		return &Result{Created: true}, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return &dbopsv1alpha1.DatabaseInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "test-instance", Namespace: namespace},
			Spec: dbopsv1alpha1.DatabaseInstanceSpec{
				Engine: dbopsv1alpha1.EngineTypePostgres,
				Connection: dbopsv1alpha1.ConnectionConfig{
					Host: "db.example.com", Port: 5432, Database: "mydb",
				},
				TLS: &dbopsv1alpha1.TLSConfig{
					Enabled:   true,
					Mode:      "verify-ca",
					SecretRef: &dbopsv1alpha1.TLSSecretRef{Name: "ca-only-tls"},
				},
			},
		}, nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{repo: mockRepo, eventBus: NewMockEventBus(), logger: logr.Discard()}
	controller := NewController(ControllerConfig{
		Client: fakeClient, Scheme: scheme, Recorder: record.NewFakeRecorder(10),
		Handler: handler, SecretManager: secret.NewManager(fakeClient),
		DefaultDriftInterval: testDefaultDriftInterval, Logger: logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testuser", Namespace: "default"},
	})
	require.NoError(t, err)

	var credSecret corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "ca-only-creds", Namespace: "default"}, &credSecret)
	require.NoError(t, err)

	assert.Equal(t, "test-ca-only", credSecret.StringData["ca.crt"])
	assert.Equal(t, "", credSecret.StringData["tls.crt"])
	assert.Equal(t, "", credSecret.StringData["tls.key"])
}

// C11: TLS disabled but template uses {{ .TLSCert }}
func TestController_Reconcile_TLSDisabledTemplateUsesTLSFields(t *testing.T) {
	scheme := newTestScheme()
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testuser",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "myuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
			PasswordSecret: &dbopsv1alpha1.PasswordConfig{
				Generate:   true,
				SecretName: "no-tls-creds",
				SecretTemplate: &dbopsv1alpha1.SecretTemplate{
					Data: map[string]string{
						"ca.crt": "{{ .CA }}",
						"cert":   "{{ .TLSCert }}",
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return false, nil
	}
	mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace, password string) (*Result, error) {
		return &Result{Created: true}, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return &dbopsv1alpha1.DatabaseInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "test-instance", Namespace: namespace},
			Spec: dbopsv1alpha1.DatabaseInstanceSpec{
				Engine: dbopsv1alpha1.EngineTypePostgres,
				Connection: dbopsv1alpha1.ConnectionConfig{
					Host: "db.example.com", Port: 5432, Database: "mydb",
				},
				// TLS is nil — disabled
			},
		}, nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{repo: mockRepo, eventBus: NewMockEventBus(), logger: logr.Discard()}
	controller := NewController(ControllerConfig{
		Client: fakeClient, Scheme: scheme, Recorder: record.NewFakeRecorder(10),
		Handler: handler, SecretManager: secret.NewManager(fakeClient),
		DefaultDriftInterval: testDefaultDriftInterval, Logger: logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testuser", Namespace: "default"},
	})
	require.NoError(t, err)

	var credSecret corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "no-tls-creds", Namespace: "default"}, &credSecret)
	require.NoError(t, err)

	// TLS fields should render as empty strings since TLS is disabled
	assert.Equal(t, "", credSecret.StringData["ca.crt"])
	assert.Equal(t, "", credSecret.StringData["cert"])
}

// C14: Instance has empty Host / Port=0 / empty Database
func TestController_Reconcile_InstanceEmptyConnectionFields(t *testing.T) {
	scheme := newTestScheme()
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testuser",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "myuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
			PasswordSecret: &dbopsv1alpha1.PasswordConfig{
				Generate:   true,
				SecretName: "empty-conn-creds",
				SecretTemplate: &dbopsv1alpha1.SecretTemplate{
					Data: map[string]string{
						"DSN": "postgresql://{{ .Username }}@{{ .Host }}:{{ .Port }}/{{ .Database }}",
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return false, nil
	}
	mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace, password string) (*Result, error) {
		return &Result{Created: true}, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return &dbopsv1alpha1.DatabaseInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "test-instance", Namespace: namespace},
			Spec: dbopsv1alpha1.DatabaseInstanceSpec{
				Engine: dbopsv1alpha1.EngineTypePostgres,
				Connection: dbopsv1alpha1.ConnectionConfig{
					Host: "", Port: 0, Database: "",
				},
			},
		}, nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{repo: mockRepo, eventBus: NewMockEventBus(), logger: logr.Discard()}
	controller := NewController(ControllerConfig{
		Client: fakeClient, Scheme: scheme, Recorder: record.NewFakeRecorder(10),
		Handler: handler, SecretManager: secret.NewManager(fakeClient),
		DefaultDriftInterval: testDefaultDriftInterval, Logger: logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testuser", Namespace: "default"},
	})
	require.NoError(t, err)

	var credSecret corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "empty-conn-creds", Namespace: "default"}, &credSecret)
	require.NoError(t, err)

	// Should render with zero/empty values
	assert.Equal(t, "postgresql://myuser@:0/", credSecret.StringData["DSN"])
}

// C15: hasSecretTemplateData — SecretTemplate non-nil but Data map is nil
func TestHasSecretTemplateData_NonNilTemplateNilData(t *testing.T) {
	user := &dbopsv1alpha1.DatabaseUser{
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			PasswordSecret: &dbopsv1alpha1.PasswordConfig{
				SecretTemplate: &dbopsv1alpha1.SecretTemplate{
					Labels: map[string]string{"app": "test"},
					// Data is nil
				},
			},
		},
	}
	assert.False(t, hasSecretTemplateData(user))
}

// C16: hasSecretTemplateData — SecretTemplate.Data is non-nil empty map
func TestHasSecretTemplateData_EmptyDataMap(t *testing.T) {
	user := &dbopsv1alpha1.DatabaseUser{
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			PasswordSecret: &dbopsv1alpha1.PasswordConfig{
				SecretTemplate: &dbopsv1alpha1.SecretTemplate{
					Data: map[string]string{},
				},
			},
		},
	}
	assert.False(t, hasSecretTemplateData(user))
}

// C17: Secret already exists — update path with SecretTemplate.Data
func TestController_Reconcile_SecretExistsUpdateWithTemplate(t *testing.T) {
	scheme := newTestScheme()
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testuser",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "myuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
			PasswordSecret: &dbopsv1alpha1.PasswordConfig{
				Generate:   true,
				SecretName: "existing-tmpl-creds",
				SecretTemplate: &dbopsv1alpha1.SecretTemplate{
					Data: map[string]string{
						"DATABASE_URL": "postgresql://{{ .Username }}@{{ .Host }}:{{ .Port }}/{{ .Database }}",
					},
				},
			},
		},
	}

	// Pre-create an existing secret (simulates previous reconcile)
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "existing-tmpl-creds",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"DATABASE_URL": []byte("old-value"),
			"password":     []byte("old-password"),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user, existingSecret).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return true, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return &dbopsv1alpha1.DatabaseInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "test-instance", Namespace: namespace},
			Spec: dbopsv1alpha1.DatabaseInstanceSpec{
				Engine: dbopsv1alpha1.EngineTypePostgres,
				Connection: dbopsv1alpha1.ConnectionConfig{
					Host: "db.example.com", Port: 5432, Database: "mydb",
				},
			},
		}, nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{repo: mockRepo, eventBus: NewMockEventBus(), logger: logr.Discard()}
	controller := NewController(ControllerConfig{
		Client: fakeClient, Scheme: scheme, Recorder: record.NewFakeRecorder(10),
		Handler: handler, SecretManager: secret.NewManager(fakeClient),
		DefaultDriftInterval: testDefaultDriftInterval, Logger: logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testuser", Namespace: "default"},
	})
	require.NoError(t, err)

	var credSecret corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "existing-tmpl-creds", Namespace: "default"}, &credSecret)
	require.NoError(t, err)

	// Updated with new template data
	assert.Contains(t, credSecret.StringData, "DATABASE_URL")
	assert.Contains(t, credSecret.StringData["DATABASE_URL"], "db.example.com:5432/mydb")
	// Old default key "password" should be replaced (StringData now only has template keys)
	assert.NotContains(t, credSecret.StringData, "password")
}

// ===== Secret Update Merge Tests =====

func TestController_Reconcile_SecretUpdatePreservesExternalAnnotations(t *testing.T) {
	scheme := newTestScheme()
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testuser",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "myuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
			PasswordSecret: &dbopsv1alpha1.PasswordConfig{
				Generate:   true,
				SecretName: "merge-ann-creds",
				SecretTemplate: &dbopsv1alpha1.SecretTemplate{
					Annotations: map[string]string{
						"operator-ann": "value",
					},
				},
			},
		},
	}

	// Pre-create secret with external annotations
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "merge-ann-creds",
			Namespace: "default",
			Annotations: map[string]string{
				"reloader.stakater.com/match": "true",
				"custom.io/managed":           "external",
			},
		},
		Data: map[string][]byte{"password": []byte("old")},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user, existingSecret).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return true, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{repo: mockRepo, eventBus: NewMockEventBus(), logger: logr.Discard()}
	controller := NewController(ControllerConfig{
		Client: fakeClient, Scheme: scheme, Recorder: record.NewFakeRecorder(10),
		Handler: handler, SecretManager: secret.NewManager(fakeClient),
		DefaultDriftInterval: testDefaultDriftInterval, Logger: logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testuser", Namespace: "default"},
	})
	require.NoError(t, err)

	var credSecret corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "merge-ann-creds", Namespace: "default"}, &credSecret)
	require.NoError(t, err)

	// External annotations preserved
	assert.Equal(t, "true", credSecret.Annotations["reloader.stakater.com/match"])
	assert.Equal(t, "external", credSecret.Annotations["custom.io/managed"])
	// Operator annotation merged
	assert.Equal(t, "value", credSecret.Annotations["operator-ann"])
	// Default operator labels set
	assert.Equal(t, "db-provision-operator", credSecret.Labels["app.kubernetes.io/managed-by"])
	assert.Equal(t, "testuser", credSecret.Labels["dbops.dbprovision.io/user"])
}

func TestController_Reconcile_SecretUpdatePreservesExternalLabels(t *testing.T) {
	scheme := newTestScheme()
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testuser",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "myuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
			PasswordSecret: &dbopsv1alpha1.PasswordConfig{
				Generate:   true,
				SecretName: "merge-lbl-creds",
				SecretTemplate: &dbopsv1alpha1.SecretTemplate{
					Labels: map[string]string{
						"custom-label": "value",
					},
				},
			},
		},
	}

	// Pre-create secret with external labels
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "merge-lbl-creds",
			Namespace: "default",
			Labels: map[string]string{
				"argocd.argoproj.io/instance": "myapp",
				"team":                        "platform",
			},
		},
		Data: map[string][]byte{"password": []byte("old")},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user, existingSecret).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return true, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{repo: mockRepo, eventBus: NewMockEventBus(), logger: logr.Discard()}
	controller := NewController(ControllerConfig{
		Client: fakeClient, Scheme: scheme, Recorder: record.NewFakeRecorder(10),
		Handler: handler, SecretManager: secret.NewManager(fakeClient),
		DefaultDriftInterval: testDefaultDriftInterval, Logger: logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testuser", Namespace: "default"},
	})
	require.NoError(t, err)

	var credSecret corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "merge-lbl-creds", Namespace: "default"}, &credSecret)
	require.NoError(t, err)

	// External labels preserved
	assert.Equal(t, "myapp", credSecret.Labels["argocd.argoproj.io/instance"])
	assert.Equal(t, "platform", credSecret.Labels["team"])
	// Custom label merged
	assert.Equal(t, "value", credSecret.Labels["custom-label"])
	// Default operator labels set
	assert.Equal(t, "db-provision-operator", credSecret.Labels["app.kubernetes.io/managed-by"])
	assert.Equal(t, "testuser", credSecret.Labels["dbops.dbprovision.io/user"])
}

func TestController_Reconcile_SecretUpdateMergesOperatorAndExternalMetadata(t *testing.T) {
	scheme := newTestScheme()
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testuser",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "myuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
			PasswordSecret: &dbopsv1alpha1.PasswordConfig{
				Generate:   true,
				SecretName: "merge-both-creds",
				SecretTemplate: &dbopsv1alpha1.SecretTemplate{
					Labels: map[string]string{
						"operator-label": "op-value",
					},
					Annotations: map[string]string{
						"operator-ann": "op-ann-value",
					},
				},
			},
		},
	}

	// Pre-create secret with both external annotations AND labels
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "merge-both-creds",
			Namespace: "default",
			Labels: map[string]string{
				"external-label": "ext-value",
			},
			Annotations: map[string]string{
				"external-ann": "ext-ann-value",
			},
		},
		StringData: map[string]string{"old-key": "old-value"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user, existingSecret).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return true, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{repo: mockRepo, eventBus: NewMockEventBus(), logger: logr.Discard()}
	controller := NewController(ControllerConfig{
		Client: fakeClient, Scheme: scheme, Recorder: record.NewFakeRecorder(10),
		Handler: handler, SecretManager: secret.NewManager(fakeClient),
		DefaultDriftInterval: testDefaultDriftInterval, Logger: logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testuser", Namespace: "default"},
	})
	require.NoError(t, err)

	var credSecret corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "merge-both-creds", Namespace: "default"}, &credSecret)
	require.NoError(t, err)

	// External metadata preserved
	assert.Equal(t, "ext-value", credSecret.Labels["external-label"])
	assert.Equal(t, "ext-ann-value", credSecret.Annotations["external-ann"])
	// Operator metadata merged
	assert.Equal(t, "op-value", credSecret.Labels["operator-label"])
	assert.Equal(t, "op-ann-value", credSecret.Annotations["operator-ann"])
	// Default operator labels
	assert.Equal(t, "db-provision-operator", credSecret.Labels["app.kubernetes.io/managed-by"])
	// StringData is fully replaced (operator-owned) — old key gone
	assert.NotContains(t, credSecret.StringData, "old-key")
	assert.Contains(t, credSecret.StringData, "username")
}

func TestController_Reconcile_SecretUpdateOverwritesOperatorAnnotationValues(t *testing.T) {
	scheme := newTestScheme()
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testuser",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "myuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
			PasswordSecret: &dbopsv1alpha1.PasswordConfig{
				Generate:   true,
				SecretName: "overwrite-ann-creds",
				SecretTemplate: &dbopsv1alpha1.SecretTemplate{
					Annotations: map[string]string{
						"operator-ann": "new",
					},
				},
			},
		},
	}

	// Pre-create secret with the same annotation at an old value
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "overwrite-ann-creds",
			Namespace: "default",
			Annotations: map[string]string{
				"operator-ann": "old",
				"external-ann": "keep-me",
			},
		},
		Data: map[string][]byte{"password": []byte("old")},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user, existingSecret).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return true, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{repo: mockRepo, eventBus: NewMockEventBus(), logger: logr.Discard()}
	controller := NewController(ControllerConfig{
		Client: fakeClient, Scheme: scheme, Recorder: record.NewFakeRecorder(10),
		Handler: handler, SecretManager: secret.NewManager(fakeClient),
		DefaultDriftInterval: testDefaultDriftInterval, Logger: logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testuser", Namespace: "default"},
	})
	require.NoError(t, err)

	var credSecret corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "overwrite-ann-creds", Namespace: "default"}, &credSecret)
	require.NoError(t, err)

	// Operator annotation updated to new value
	assert.Equal(t, "new", credSecret.Annotations["operator-ann"])
	// External annotation untouched
	assert.Equal(t, "keep-me", credSecret.Annotations["external-ann"])
}

func TestController_Reconcile_SecretUpdateWithNilAnnotationsOnExisting(t *testing.T) {
	scheme := newTestScheme()
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testuser",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "myuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
			PasswordSecret: &dbopsv1alpha1.PasswordConfig{
				Generate:   true,
				SecretName: "nil-meta-creds",
				SecretTemplate: &dbopsv1alpha1.SecretTemplate{
					Annotations: map[string]string{
						"new-ann": "added",
					},
				},
			},
		},
	}

	// Pre-create bare secret with nil annotations and labels
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nil-meta-creds",
			Namespace: "default",
		},
		Data: map[string][]byte{"password": []byte("old")},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user, existingSecret).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return true, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{repo: mockRepo, eventBus: NewMockEventBus(), logger: logr.Discard()}
	controller := NewController(ControllerConfig{
		Client: fakeClient, Scheme: scheme, Recorder: record.NewFakeRecorder(10),
		Handler: handler, SecretManager: secret.NewManager(fakeClient),
		DefaultDriftInterval: testDefaultDriftInterval, Logger: logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testuser", Namespace: "default"},
	})
	require.NoError(t, err)

	var credSecret corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "nil-meta-creds", Namespace: "default"}, &credSecret)
	require.NoError(t, err)

	// Annotations added without nil-pointer panic
	assert.Equal(t, "added", credSecret.Annotations["new-ann"])
	// Default operator labels set
	assert.Equal(t, "db-provision-operator", credSecret.Labels["app.kubernetes.io/managed-by"])
	assert.Equal(t, "testuser", credSecret.Labels["dbops.dbprovision.io/user"])
}

func TestController_Reconcile_ClusterInstanceRefDoesNotPanic(t *testing.T) {
	scheme := newTestScheme()

	// Create a DatabaseUser with clusterInstanceRef (NOT instanceRef)
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vault",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "vault",
			ClusterInstanceRef: &dbopsv1alpha1.ClusterInstanceReference{
				Name: "shared-postgres",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(user).
		WithStatusSubresource(user).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return false, nil
	}
	mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace, password string) (*Result, error) {
		return &Result{Created: true, Message: "created"}, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("shared-postgres", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		SecretManager:        secret.NewManager(fakeClient),
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})

	// This must not panic — the bug was a nil deref on spec.InstanceRef.Name
	// when clusterInstanceRef is used instead.
	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "vault", Namespace: "default"},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	var updatedUser dbopsv1alpha1.DatabaseUser
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "vault", Namespace: "default"}, &updatedUser)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedUser.Status.Phase)
	assert.True(t, mockRepo.WasCalled("Create"))
}
