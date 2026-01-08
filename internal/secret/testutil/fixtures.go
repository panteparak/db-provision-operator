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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

const (
	// TestNamespace is the default namespace for test fixtures
	TestNamespace = "test-namespace"

	// TestCredentialSecretName is the default credential secret name
	TestCredentialSecretName = "test-credentials"

	// TestTLSSecretName is the default TLS secret name
	TestTLSSecretName = "test-tls"

	// TestUsername is the default test username
	TestUsername = "testuser"

	// TestPassword is the default test password
	TestPassword = "testpassword123"
)

// NewCredentialSecret creates a credential secret for testing with default keys
func NewCredentialSecret(name, namespace string) *corev1.Secret {
	return NewCredentialSecretWithKeys(name, namespace, "username", "password")
}

// NewCredentialSecretWithKeys creates a credential secret with custom key names
func NewCredentialSecretWithKeys(name, namespace, usernameKey, passwordKey string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			usernameKey: []byte(TestUsername),
			passwordKey: []byte(TestPassword),
		},
	}
}

// NewCredentialSecretWithData creates a credential secret with specific data
func NewCredentialSecretWithData(name, namespace, username, password string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"username": []byte(username),
			"password": []byte(password),
		},
	}
}

// NewTLSSecret creates a TLS secret for testing with default keys
func NewTLSSecret(name, namespace string) *corev1.Secret {
	return NewTLSSecretWithKeys(name, namespace, "ca.crt", "tls.crt", "tls.key")
}

// NewTLSSecretWithKeys creates a TLS secret with custom key names
func NewTLSSecretWithKeys(name, namespace, caKey, certKey, keyKey string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			caKey:   []byte(TestCACert),
			certKey: []byte(TestClientCert),
			keyKey:  []byte(TestClientKey),
		},
	}
}

// NewTLSSecretWithCA creates a TLS secret with only CA certificate
func NewTLSSecretWithCA(name, namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"ca.crt": []byte(TestCACert),
		},
	}
}

// NewPasswordSecret creates a secret containing just a password
func NewPasswordSecret(name, namespace, key, password string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			key: []byte(password),
		},
	}
}

// NewOpaqueSecret creates a generic opaque secret with the given data
func NewOpaqueSecret(name, namespace string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}
}

// NewCredentialSecretRef creates a CredentialSecretRef for testing
func NewCredentialSecretRef(name string) *dbopsv1alpha1.CredentialSecretRef {
	return &dbopsv1alpha1.CredentialSecretRef{
		Name: name,
	}
}

// NewCredentialSecretRefWithNamespace creates a CredentialSecretRef with namespace
func NewCredentialSecretRefWithNamespace(name, namespace string) *dbopsv1alpha1.CredentialSecretRef {
	return &dbopsv1alpha1.CredentialSecretRef{
		Name:      name,
		Namespace: namespace,
	}
}

// NewCredentialSecretRefWithKeys creates a CredentialSecretRef with custom key names
func NewCredentialSecretRefWithKeys(name, usernameKey, passwordKey string) *dbopsv1alpha1.CredentialSecretRef {
	return &dbopsv1alpha1.CredentialSecretRef{
		Name: name,
		Keys: &dbopsv1alpha1.CredentialKeys{
			Username: usernameKey,
			Password: passwordKey,
		},
	}
}

// NewTLSConfig creates a TLSConfig for testing
func NewTLSConfig(enabled bool, secretName string) *dbopsv1alpha1.TLSConfig {
	if !enabled {
		return &dbopsv1alpha1.TLSConfig{
			Enabled: false,
		}
	}
	return &dbopsv1alpha1.TLSConfig{
		Enabled: true,
		SecretRef: &dbopsv1alpha1.TLSSecretRef{
			Name: secretName,
		},
	}
}

// NewTLSConfigWithKeys creates a TLSConfig with custom key names
func NewTLSConfigWithKeys(secretName, caKey, certKey, keyKey string) *dbopsv1alpha1.TLSConfig {
	return &dbopsv1alpha1.TLSConfig{
		Enabled: true,
		SecretRef: &dbopsv1alpha1.TLSSecretRef{
			Name: secretName,
			Keys: &dbopsv1alpha1.TLSKeys{
				CA:   caKey,
				Cert: certKey,
				Key:  keyKey,
			},
		},
	}
}

// NewPasswordConfig creates a PasswordConfig for testing
func NewPasswordConfig(length int32, includeSpecial bool) *dbopsv1alpha1.PasswordConfig {
	return &dbopsv1alpha1.PasswordConfig{
		Length:              length,
		IncludeSpecialChars: includeSpecial,
		SecretName:          "generated-secret",
	}
}

// NewPasswordConfigWithExclusions creates a PasswordConfig with excluded characters
func NewPasswordConfigWithExclusions(length int32, includeSpecial bool, excludeChars string) *dbopsv1alpha1.PasswordConfig {
	return &dbopsv1alpha1.PasswordConfig{
		Length:              length,
		IncludeSpecialChars: includeSpecial,
		ExcludeChars:        excludeChars,
		SecretName:          "generated-secret",
	}
}

// NewExistingPasswordSecret creates an ExistingPasswordSecret reference
func NewExistingPasswordSecret(name, key string) *dbopsv1alpha1.ExistingPasswordSecret {
	return &dbopsv1alpha1.ExistingPasswordSecret{
		Name: name,
		Key:  key,
	}
}

// NewExistingPasswordSecretWithNamespace creates an ExistingPasswordSecret with namespace
func NewExistingPasswordSecretWithNamespace(name, namespace, key string) *dbopsv1alpha1.ExistingPasswordSecret {
	return &dbopsv1alpha1.ExistingPasswordSecret{
		Name:      name,
		Namespace: namespace,
		Key:       key,
	}
}

// NewSecretTemplate creates a SecretTemplate for testing
func NewSecretTemplate(data map[string]string) *dbopsv1alpha1.SecretTemplate {
	return &dbopsv1alpha1.SecretTemplate{
		Data: data,
	}
}

// Sample TLS certificates for testing (self-signed, not for production)
const (
	// TestCACert is a sample CA certificate for testing
	TestCACert = `-----BEGIN CERTIFICATE-----
MIIBkTCB+wIJAKHBfpq8aBplMA0GCSqGSIb3DQEBCwUAMBExDzANBgNVBAMMBnRl
c3RjYTAeFw0yNDAxMDEwMDAwMDBaFw0yNTAxMDEwMDAwMDBaMBExDzANBgNVBAMM
BnRlc3RjYTBcMA0GCSqGSIb3DQEBAQUAA0sAMEgCQQC8kT3J0m3ILR3eINAo9nEo
vQqQXqQExhTMjSqFRAW+7NBTTH/hDLRWL9Ib/Bqm3jRNy9OdQVZmBv2dBHu9VzAD
AgMBAAGjUzBRMB0GA1UdDgQWBBQoIi0G5c3fHnHyOJGkhJ5I8DqGpTAfBgNVHSME
GDAWgBQoIi0G5c3fHnHyOJGkhJ5I8DqGpTAPBgNVHRMBAf8EBTADAQH/MA0GCSqG
SIb3DQEBCwUAA0EAk+JuGGBv5gNfSrkALwTnT5mjPPDl+MgLs3L8lMPjdgBb1bEz
lJiPNJFDQVJdPgm7hI7s5m9V9c3GfAnSF/6tFw==
-----END CERTIFICATE-----`

	// TestClientCert is a sample client certificate for testing
	TestClientCert = `-----BEGIN CERTIFICATE-----
MIIBjTCB9wIJAKHBfpq8aBpmMA0GCSqGSIb3DQEBCwUAMBExDzANBgNVBAMMBnRl
c3RjYTAeFw0yNDAxMDEwMDAwMDBaFw0yNTAxMDEwMDAwMDBaMBUxEzARBgNVBAMM
CnRlc3RjbGllbnQwXDANBgkqhkiG9w0BAQEFAANLADBIAkEAvJE9ydJtyC0d3iDQ
KPZxKL0KkF6kBMYUzI0qhUQFvuzQU0x/4Qy0Vi/SG/waptsyTcvTnUFWZgb9nQR7
vVcwAwIDAQABozowODAJBgNVHRMEAjAAMAsGA1UdDwQEAwIFoDAdBgNVHSUEFjAU
BggrBgEFBQcDAgYIKwYBBQUHAwEwDQYJKoZIhvcNAQELBQADQQBKEuJvMsLb7n0d
SKFHPGKPr8jXA0cMH4fZ8lR3aO3LAv/sS7PkfRKLI3F7MvrZjD3QzLzRpBNEOyPQ
Gq5gXlUz
-----END CERTIFICATE-----`

	// TestClientKey is a sample client key for testing
	TestClientKey = `-----BEGIN RSA PRIVATE KEY-----
MIIBOgIBAAJBALyRPcnSbcgtHd4g0Cj2cSi9CpBepATGFMyNKoVEBb7s0FNMf+EM
tFYv0hv8GqbbMk3L051BVmYG/Z0Ee71XMAMCAwEAAQJAF0HPRvpxC7r4dLVz0vhE
5I4cPLZfA/S9SV3VhG5RnTBdF7Tv9pCVLBLJ3oqKZpEWdT3tJjZbDJf7pPnrPBJ2
AQIhAOPB3R/8G8SJqDdPBMBfCq3bHfQVhJqF3c2k5hJhEy3RAiEA0nyTzL9tVv0J
lG7DGdO0xS7WXsrVj0xB3cDU5GpHiwMCIEDzVPTvRT8iM4ssi9qAVmrVvBKhNaIZ
N0+OQpHnq3JRAiEAyzMkk9pqSZRj0EB/V5Qf3xeIVjVRR4LMxqCsTaLIiIsCIAre
SApI8p2gKm6i5dp7kSmOrGKuD0R5O4TqHGCgV3ib
-----END RSA PRIVATE KEY-----`
)
