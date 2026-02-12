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
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/secret"
	"github.com/db-provision-operator/internal/util"
)

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
		Client:        client,
		Scheme:        scheme,
		Recorder:      record.NewFakeRecorder(10),
		Handler:       handler,
		SecretManager: secret.NewManager(client),
		Logger:        logr.Discard(),
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
		Client:        client,
		Scheme:        scheme,
		Recorder:      record.NewFakeRecorder(10),
		Handler:       handler,
		SecretManager: secret.NewManager(client),
		Logger:        logr.Discard(),
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
		Client:        client,
		Scheme:        scheme,
		Recorder:      record.NewFakeRecorder(10),
		Handler:       handler,
		SecretManager: secret.NewManager(client),
		Logger:        logr.Discard(),
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
		Client:        client,
		Scheme:        scheme,
		Recorder:      record.NewFakeRecorder(10),
		Handler:       handler,
		SecretManager: secret.NewManager(client),
		Logger:        logr.Discard(),
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
		Client:        client,
		Scheme:        scheme,
		Recorder:      record.NewFakeRecorder(10),
		Handler:       handler,
		SecretManager: secret.NewManager(client),
		Logger:        logr.Discard(),
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
		Client:        client,
		Scheme:        scheme,
		Recorder:      record.NewFakeRecorder(10),
		Handler:       handler,
		SecretManager: secret.NewManager(client),
		Logger:        logr.Discard(),
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
		Client:        client,
		Scheme:        scheme,
		Recorder:      record.NewFakeRecorder(10),
		Handler:       handler,
		SecretManager: secret.NewManager(client),
		Logger:        logr.Discard(),
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
		Client:        client,
		Scheme:        scheme,
		Recorder:      record.NewFakeRecorder(10),
		Handler:       handler,
		SecretManager: secret.NewManager(client),
		Logger:        logr.Discard(),
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
		Client:        client,
		Scheme:        scheme,
		Recorder:      record.NewFakeRecorder(10),
		Handler:       handler,
		SecretManager: secret.NewManager(client),
		Logger:        logr.Discard(),
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
		Client:        client,
		Scheme:        scheme,
		Recorder:      recorder,
		Handler:       handler,
		SecretManager: secret.NewManager(client),
		Logger:        logr.Discard(),
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
		Client:        client,
		Scheme:        scheme,
		Recorder:      record.NewFakeRecorder(10),
		Handler:       handler,
		SecretManager: secret.NewManager(client),
		Logger:        logr.Discard(),
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
		Client:        client,
		Scheme:        scheme,
		Recorder:      record.NewFakeRecorder(10),
		Handler:       handler,
		SecretManager: secret.NewManager(client),
		Logger:        logr.Discard(),
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
		Client:        client,
		Scheme:        scheme,
		Recorder:      record.NewFakeRecorder(10),
		Handler:       handler,
		SecretManager: secret.NewManager(client),
		Logger:        logr.Discard(),
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
