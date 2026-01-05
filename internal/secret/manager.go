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

package secret

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"text/template"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

// Manager handles secret operations
type Manager struct {
	client client.Client
}

// NewManager creates a new secret manager
func NewManager(c client.Client) *Manager {
	return &Manager{client: c}
}

// Credentials holds username and password
type Credentials struct {
	Username string
	Password string
}

// TLSCredentials holds TLS certificate data
type TLSCredentials struct {
	CA   []byte
	Cert []byte
	Key  []byte
}

// GetCredentials retrieves credentials from a CredentialSecretRef
func (m *Manager) GetCredentials(ctx context.Context, namespace string, ref *dbopsv1alpha1.CredentialSecretRef) (*Credentials, error) {
	if ref == nil {
		return nil, fmt.Errorf("credential reference is nil")
	}

	secret := &corev1.Secret{}
	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      ref.Name,
	}, secret)
	if err != nil {
		return nil, err
	}

	// Get key names (with defaults)
	usernameKey := "username"
	passwordKey := "password"
	if ref.Keys != nil {
		if ref.Keys.Username != "" {
			usernameKey = ref.Keys.Username
		}
		if ref.Keys.Password != "" {
			passwordKey = ref.Keys.Password
		}
	}

	username, ok := secret.Data[usernameKey]
	if !ok {
		return nil, fmt.Errorf("secret %s does not contain key %s", ref.Name, usernameKey)
	}

	password, ok := secret.Data[passwordKey]
	if !ok {
		return nil, fmt.Errorf("secret %s does not contain key %s", ref.Name, passwordKey)
	}

	return &Credentials{
		Username: string(username),
		Password: string(password),
	}, nil
}

// GetTLSCredentials retrieves TLS credentials from TLSConfig
func (m *Manager) GetTLSCredentials(ctx context.Context, namespace string, tlsConfig *dbopsv1alpha1.TLSConfig) (*TLSCredentials, error) {
	if tlsConfig == nil || !tlsConfig.Enabled {
		return nil, nil
	}

	if tlsConfig.SecretRef == nil {
		return nil, nil
	}

	secret := &corev1.Secret{}
	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      tlsConfig.SecretRef.Name,
	}, secret)
	if err != nil {
		return nil, fmt.Errorf("failed to get TLS secret: %w", err)
	}

	creds := &TLSCredentials{}

	// Get key names (with defaults)
	caKey := "ca.crt"
	certKey := "tls.crt"
	keyKey := "tls.key"
	if tlsConfig.SecretRef.Keys != nil {
		if tlsConfig.SecretRef.Keys.CA != "" {
			caKey = tlsConfig.SecretRef.Keys.CA
		}
		if tlsConfig.SecretRef.Keys.Cert != "" {
			certKey = tlsConfig.SecretRef.Keys.Cert
		}
		if tlsConfig.SecretRef.Keys.Key != "" {
			keyKey = tlsConfig.SecretRef.Keys.Key
		}
	}

	// Get CA certificate
	if ca, ok := secret.Data[caKey]; ok {
		creds.CA = ca
	}

	// Get client certificate
	if cert, ok := secret.Data[certKey]; ok {
		creds.Cert = cert
	}

	// Get client key
	if key, ok := secret.Data[keyKey]; ok {
		creds.Key = key
	}

	return creds, nil
}

// getSecretData retrieves a specific key from a secret
func (m *Manager) getSecretData(ctx context.Context, namespace, secretName, key string) ([]byte, error) {
	secret := &corev1.Secret{}
	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      secretName,
	}, secret)
	if err != nil {
		return nil, err
	}

	data, ok := secret.Data[key]
	if !ok {
		return nil, fmt.Errorf("secret %s does not contain key %s", secretName, key)
	}

	return data, nil
}

// GeneratePassword generates a random password based on PasswordConfig
func GeneratePassword(opts *dbopsv1alpha1.PasswordConfig) (string, error) {
	length := 32
	if opts != nil && opts.Length > 0 {
		length = int(opts.Length)
	}

	includeSpecial := true
	excludeChars := ""
	if opts != nil {
		includeSpecial = opts.IncludeSpecialChars
		excludeChars = opts.ExcludeChars
	}

	// Build character set
	charset := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	if includeSpecial {
		charset += "!@#$%^&*"
	}

	// Remove excluded characters
	if excludeChars != "" {
		filteredCharset := ""
		for _, c := range charset {
			excluded := false
			for _, e := range excludeChars {
				if c == e {
					excluded = true
					break
				}
			}
			if !excluded {
				filteredCharset += string(c)
			}
		}
		charset = filteredCharset
	}

	if charset == "" {
		charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	}

	password := make([]byte, length)
	charsetLen := big.NewInt(int64(len(charset)))

	for i := 0; i < length; i++ {
		idx, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			return "", fmt.Errorf("failed to generate random number: %w", err)
		}
		password[i] = charset[idx.Int64()]
	}

	return string(password), nil
}

// OwnerInfo contains information needed to set owner references
type OwnerInfo struct {
	APIVersion string
	Kind       string
	Name       string
	UID        types.UID
}

// CreateSecret creates a new secret with the given data
func (m *Manager) CreateSecret(ctx context.Context, namespace, name string, data map[string][]byte, owner *OwnerInfo) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}

	// Set owner reference if provided
	if owner != nil {
		controller := true
		secret.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: owner.APIVersion,
				Kind:       owner.Kind,
				Name:       owner.Name,
				UID:        owner.UID,
				Controller: &controller,
			},
		}
	}

	return m.client.Create(ctx, secret)
}

// CreateSecretWithOwner creates a new secret with owner reference from a runtime.Object
func (m *Manager) CreateSecretWithOwner(ctx context.Context, namespace, name string, data map[string][]byte, owner client.Object, scheme *runtime.Scheme) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}

	// Set owner reference if provided
	if owner != nil {
		gvk := owner.GetObjectKind().GroupVersionKind()
		if gvk.Kind == "" {
			// Try to get GVK from scheme
			gvks, _, err := scheme.ObjectKinds(owner)
			if err == nil && len(gvks) > 0 {
				gvk = gvks[0]
			}
		}

		controller := true
		secret.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: gvk.GroupVersion().String(),
				Kind:       gvk.Kind,
				Name:       owner.GetName(),
				UID:        owner.GetUID(),
				Controller: &controller,
			},
		}
	}

	return m.client.Create(ctx, secret)
}

// UpdateSecret updates an existing secret
func (m *Manager) UpdateSecret(ctx context.Context, namespace, name string, data map[string][]byte) error {
	secret := &corev1.Secret{}
	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, secret)
	if err != nil {
		return err
	}

	secret.Data = data
	return m.client.Update(ctx, secret)
}

// EnsureSecret creates or updates a secret
func (m *Manager) EnsureSecret(ctx context.Context, namespace, name string, data map[string][]byte, owner *OwnerInfo) error {
	secret := &corev1.Secret{}
	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, secret)
	if err != nil {
		if errors.IsNotFound(err) {
			return m.CreateSecret(ctx, namespace, name, data, owner)
		}
		return err
	}

	// Update existing secret
	secret.Data = data
	return m.client.Update(ctx, secret)
}

// EnsureSecretWithOwner creates or updates a secret with owner reference
func (m *Manager) EnsureSecretWithOwner(ctx context.Context, namespace, name string, data map[string][]byte, owner client.Object, scheme *runtime.Scheme) error {
	secret := &corev1.Secret{}
	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, secret)
	if err != nil {
		if errors.IsNotFound(err) {
			return m.CreateSecretWithOwner(ctx, namespace, name, data, owner, scheme)
		}
		return err
	}

	// Update existing secret
	secret.Data = data
	return m.client.Update(ctx, secret)
}

// DeleteSecret deletes a secret
func (m *Manager) DeleteSecret(ctx context.Context, namespace, name string) error {
	secret := &corev1.Secret{}
	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, secret)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return m.client.Delete(ctx, secret)
}

// SecretExists checks if a secret exists
func (m *Manager) SecretExists(ctx context.Context, namespace, name string) (bool, error) {
	secret := &corev1.Secret{}
	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, secret)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// TemplateData holds data for secret templating
type TemplateData struct {
	Username  string
	Password  string
	Host      string
	Port      int32
	Database  string
	SSLMode   string
	Namespace string
	Name      string
}

// RenderSecretTemplate renders a secret template with the given data
func RenderSecretTemplate(tmpl *dbopsv1alpha1.SecretTemplate, data TemplateData) (map[string][]byte, error) {
	if tmpl == nil {
		return nil, fmt.Errorf("template is nil")
	}

	result := make(map[string][]byte)

	// Render each key in Data (which is a map[string]string containing templates)
	for key, value := range tmpl.Data {
		rendered, err := renderTemplate(key, value, data)
		if err != nil {
			return nil, fmt.Errorf("failed to render template for key %s: %w", key, err)
		}
		result[key] = []byte(rendered)
	}

	return result, nil
}

// renderTemplate renders a single template string
func renderTemplate(name, tmplStr string, data TemplateData) (string, error) {
	tmpl, err := template.New(name).Parse(tmplStr)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// GetPassword retrieves a password from a secret
func (m *Manager) GetPassword(ctx context.Context, namespace string, secretRef *dbopsv1alpha1.ExistingPasswordSecret) (string, error) {
	if secretRef == nil {
		return "", fmt.Errorf("password secret reference is nil")
	}

	secretNamespace := namespace
	if secretRef.Namespace != "" {
		secretNamespace = secretRef.Namespace
	}

	data, err := m.getSecretData(ctx, secretNamespace, secretRef.Name, secretRef.Key)
	if err != nil {
		return "", err
	}

	return string(data), nil
}
