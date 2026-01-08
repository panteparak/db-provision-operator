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
	"context"
	"fmt"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

// FakeClient is a simple in-memory fake client for testing without envtest.
// It implements the client.Client interface and stores objects in memory.
type FakeClient struct {
	mu      sync.RWMutex
	objects map[string]client.Object
	scheme  *runtime.Scheme

	// Error injection for testing error scenarios
	GetError    error
	CreateError error
	UpdateError error
	DeleteError error
	ListError   error
	PatchError  error

	// Hooks for intercepting operations
	OnGet    func(ctx context.Context, key types.NamespacedName, obj client.Object) error
	OnCreate func(ctx context.Context, obj client.Object) error
	OnUpdate func(ctx context.Context, obj client.Object) error
	OnDelete func(ctx context.Context, obj client.Object) error
}

// NewFakeClient creates a new FakeClient with the dbopsv1alpha1 scheme registered.
func NewFakeClient() *FakeClient {
	s := runtime.NewScheme()
	_ = scheme.AddToScheme(s)
	_ = dbopsv1alpha1.AddToScheme(s)

	return &FakeClient{
		objects: make(map[string]client.Object),
		scheme:  s,
	}
}

// NewFakeClientWithObjects creates a new FakeClient pre-populated with objects.
func NewFakeClientWithObjects(objects ...client.Object) *FakeClient {
	c := NewFakeClient()
	for _, obj := range objects {
		key := objectKey(obj)
		c.objects[key] = obj.DeepCopyObject().(client.Object)
	}
	return c
}

// objectKey generates a unique key for an object based on GVK and namespace/name.
func objectKey(obj client.Object) string {
	gvk := obj.GetObjectKind().GroupVersionKind()
	return fmt.Sprintf("%s/%s/%s/%s", gvk.Group, gvk.Kind, obj.GetNamespace(), obj.GetName())
}

// objectKeyFromNamespacedName generates a key from a types.NamespacedName and GVK.
func objectKeyFromNamespacedName(gvk schema.GroupVersionKind, key types.NamespacedName) string {
	return fmt.Sprintf("%s/%s/%s/%s", gvk.Group, gvk.Kind, key.Namespace, key.Name)
}

// Get retrieves an object by key from the store.
func (c *FakeClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
	if c.GetError != nil {
		return c.GetError
	}
	if c.OnGet != nil {
		if err := c.OnGet(ctx, key, obj); err != nil {
			return err
		}
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	gvk, err := c.gvkForObject(obj)
	if err != nil {
		return err
	}

	k := objectKeyFromNamespacedName(gvk, key)
	stored, ok := c.objects[k]
	if !ok {
		return errors.NewNotFound(schema.GroupResource{
			Group:    gvk.Group,
			Resource: gvk.Kind,
		}, key.Name)
	}

	// Copy the stored object into the provided object
	storedVal := stored.DeepCopyObject()
	objVal := obj
	if storedCopy, ok := storedVal.(client.Object); ok {
		copyObject(storedCopy, objVal)
	}

	return nil
}

// Create stores a new object in the store.
func (c *FakeClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if c.CreateError != nil {
		return c.CreateError
	}
	if c.OnCreate != nil {
		if err := c.OnCreate(ctx, obj); err != nil {
			return err
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Set GVK if not already set
	if err := c.setGVK(obj); err != nil {
		return err
	}

	key := objectKey(obj)
	if _, exists := c.objects[key]; exists {
		gvk := obj.GetObjectKind().GroupVersionKind()
		return errors.NewAlreadyExists(schema.GroupResource{
			Group:    gvk.Group,
			Resource: gvk.Kind,
		}, obj.GetName())
	}

	// Set creation timestamp if not set
	creationTimestamp := obj.GetCreationTimestamp()
	if creationTimestamp.IsZero() {
		obj.SetCreationTimestamp(metav1.Now())
	}

	// Set resource version
	obj.SetResourceVersion("1")

	c.objects[key] = obj.DeepCopyObject().(client.Object)
	return nil
}

// Update updates an existing object in the store.
func (c *FakeClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if c.UpdateError != nil {
		return c.UpdateError
	}
	if c.OnUpdate != nil {
		if err := c.OnUpdate(ctx, obj); err != nil {
			return err
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.setGVK(obj); err != nil {
		return err
	}

	key := objectKey(obj)
	if _, exists := c.objects[key]; !exists {
		gvk := obj.GetObjectKind().GroupVersionKind()
		return errors.NewNotFound(schema.GroupResource{
			Group:    gvk.Group,
			Resource: gvk.Kind,
		}, obj.GetName())
	}

	// Increment resource version
	rv := obj.GetResourceVersion()
	if rv == "" {
		rv = "1"
	}
	obj.SetResourceVersion(incrementResourceVersion(rv))

	c.objects[key] = obj.DeepCopyObject().(client.Object)
	return nil
}

// Delete removes an object from the store.
func (c *FakeClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if c.DeleteError != nil {
		return c.DeleteError
	}
	if c.OnDelete != nil {
		if err := c.OnDelete(ctx, obj); err != nil {
			return err
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.setGVK(obj); err != nil {
		return err
	}

	key := objectKey(obj)
	if _, exists := c.objects[key]; !exists {
		gvk := obj.GetObjectKind().GroupVersionKind()
		return errors.NewNotFound(schema.GroupResource{
			Group:    gvk.Group,
			Resource: gvk.Kind,
		}, obj.GetName())
	}

	delete(c.objects, key)
	return nil
}

// Patch applies a patch to an object.
func (c *FakeClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if c.PatchError != nil {
		return c.PatchError
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.setGVK(obj); err != nil {
		return err
	}

	key := objectKey(obj)
	if _, exists := c.objects[key]; !exists {
		gvk := obj.GetObjectKind().GroupVersionKind()
		return errors.NewNotFound(schema.GroupResource{
			Group:    gvk.Group,
			Resource: gvk.Kind,
		}, obj.GetName())
	}

	// Increment resource version
	rv := obj.GetResourceVersion()
	if rv == "" {
		rv = "1"
	}
	obj.SetResourceVersion(incrementResourceVersion(rv))

	c.objects[key] = obj.DeepCopyObject().(client.Object)
	return nil
}

// DeleteAllOf deletes all objects of the given type in the given namespace.
func (c *FakeClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	gvk, err := c.gvkForObject(obj)
	if err != nil {
		return err
	}

	deleteOpts := &client.DeleteAllOfOptions{}
	for _, opt := range opts {
		opt.ApplyToDeleteAllOf(deleteOpts)
	}

	for key := range c.objects {
		stored := c.objects[key]
		storedGVK := stored.GetObjectKind().GroupVersionKind()
		if storedGVK.Group == gvk.Group && storedGVK.Kind == gvk.Kind {
			if deleteOpts.Namespace == "" || stored.GetNamespace() == deleteOpts.Namespace {
				delete(c.objects, key)
			}
		}
	}

	return nil
}

// List lists objects of the given type.
func (c *FakeClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if c.ListError != nil {
		return c.ListError
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	listOpts := &client.ListOptions{}
	for _, opt := range opts {
		opt.ApplyToList(listOpts)
	}

	// Get the GVK for the list
	gvk, err := c.gvkForObject(list)
	if err != nil {
		return err
	}

	// Convert list kind to item kind (remove "List" suffix)
	itemKind := gvk.Kind
	if len(itemKind) > 4 && itemKind[len(itemKind)-4:] == "List" {
		itemKind = itemKind[:len(itemKind)-4]
	}

	var items []runtime.Object
	for _, obj := range c.objects {
		objGVK := obj.GetObjectKind().GroupVersionKind()
		if objGVK.Group == gvk.Group && objGVK.Kind == itemKind {
			if listOpts.Namespace == "" || obj.GetNamespace() == listOpts.Namespace {
				items = append(items, obj.DeepCopyObject())
			}
		}
	}

	return meta.SetList(list, items)
}

// Status returns a StatusWriter for status subresource updates.
func (c *FakeClient) Status() client.StatusWriter {
	return &fakeStatusWriter{client: c}
}

// Scheme returns the scheme used by the client.
func (c *FakeClient) Scheme() *runtime.Scheme {
	return c.scheme
}

// RESTMapper returns nil as we don't need a real RESTMapper for fake tests.
func (c *FakeClient) RESTMapper() meta.RESTMapper {
	return nil
}

// GroupVersionKindFor returns the GVK for an object.
func (c *FakeClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return c.gvkForObject(obj)
}

// IsObjectNamespaced returns true if the object is namespaced.
func (c *FakeClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	// For simplicity, assume all objects are namespaced except for cluster-scoped resources
	return true, nil
}

// SubResource returns a SubResourceClient for the given subresource.
func (c *FakeClient) SubResource(subResource string) client.SubResourceClient {
	return &fakeSubResourceClient{client: c, subResource: subResource}
}

// gvkForObject returns the GVK for an object using the scheme.
func (c *FakeClient) gvkForObject(obj runtime.Object) (schema.GroupVersionKind, error) {
	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Kind != "" {
		return gvk, nil
	}

	gvks, _, err := c.scheme.ObjectKinds(obj)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}
	if len(gvks) == 0 {
		return schema.GroupVersionKind{}, fmt.Errorf("no GVK found for object")
	}

	return gvks[0], nil
}

// setGVK sets the GVK on an object if not already set.
func (c *FakeClient) setGVK(obj client.Object) error {
	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Kind != "" {
		return nil
	}

	gvks, _, err := c.scheme.ObjectKinds(obj)
	if err != nil {
		return err
	}
	if len(gvks) == 0 {
		return fmt.Errorf("no GVK found for object")
	}

	obj.GetObjectKind().SetGroupVersionKind(gvks[0])
	return nil
}

// Reset clears all objects from the client.
func (c *FakeClient) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.objects = make(map[string]client.Object)
	c.GetError = nil
	c.CreateError = nil
	c.UpdateError = nil
	c.DeleteError = nil
	c.ListError = nil
	c.PatchError = nil
}

// GetObjects returns all objects stored in the client.
func (c *FakeClient) GetObjects() []client.Object {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var objects []client.Object
	for _, obj := range c.objects {
		objects = append(objects, obj.DeepCopyObject().(client.Object))
	}
	return objects
}

// fakeStatusWriter implements client.StatusWriter for the FakeClient.
type fakeStatusWriter struct {
	client *FakeClient
}

// Create creates a status subresource.
func (w *fakeStatusWriter) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	return w.client.Update(ctx, obj)
}

// Update updates the status subresource.
func (w *fakeStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	return w.client.Update(ctx, obj)
}

// Patch patches the status subresource.
func (w *fakeStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return w.client.Patch(ctx, obj, patch)
}

// fakeSubResourceClient implements client.SubResourceClient for the FakeClient.
type fakeSubResourceClient struct {
	client      *FakeClient
	subResource string
}

// Get gets a subresource.
func (c *fakeSubResourceClient) Get(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceGetOption) error {
	return c.client.Get(ctx, types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}, subResource)
}

// Create creates a subresource.
func (c *fakeSubResourceClient) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	return c.client.Create(ctx, subResource)
}

// Update updates a subresource.
func (c *fakeSubResourceClient) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	return c.client.Update(ctx, obj)
}

// Patch patches a subresource.
func (c *fakeSubResourceClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return c.client.Patch(ctx, obj, patch)
}

// copyObject copies fields from src to dst.
func copyObject(src, dst client.Object) {
	dst.SetName(src.GetName())
	dst.SetNamespace(src.GetNamespace())
	dst.SetLabels(src.GetLabels())
	dst.SetAnnotations(src.GetAnnotations())
	dst.SetFinalizers(src.GetFinalizers())
	dst.SetOwnerReferences(src.GetOwnerReferences())
	dst.SetResourceVersion(src.GetResourceVersion())
	dst.SetGeneration(src.GetGeneration())
	dst.SetDeletionTimestamp(src.GetDeletionTimestamp())
	dst.SetCreationTimestamp(src.GetCreationTimestamp())
	dst.GetObjectKind().SetGroupVersionKind(src.GetObjectKind().GroupVersionKind())
}

// incrementResourceVersion increments the resource version string.
func incrementResourceVersion(rv string) string {
	var version int
	if _, err := fmt.Sscanf(rv, "%d", &version); err != nil {
		return "1"
	}
	return fmt.Sprintf("%d", version+1)
}

// CreateTestSecret creates a test secret with username and password.
func CreateTestSecret(name, namespace, username, password string) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"username": []byte(username),
			"password": []byte(password),
		},
	}
}

// CreateTestSecretWithKeys creates a test secret with custom keys.
func CreateTestSecretWithKeys(name, namespace string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
	}
}
