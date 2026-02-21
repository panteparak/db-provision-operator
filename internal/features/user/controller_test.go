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
