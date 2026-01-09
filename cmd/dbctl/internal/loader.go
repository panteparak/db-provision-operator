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

package internal

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"sigs.k8s.io/yaml"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

const (
	expectedAPIVersion = "dbops.dbprovision.io/v1alpha1"
)

// ResourceKind represents the kind of database resource
type ResourceKind string

const (
	KindDatabase        ResourceKind = "Database"
	KindDatabaseUser    ResourceKind = "DatabaseUser"
	KindDatabaseRole    ResourceKind = "DatabaseRole"
	KindDatabaseGrant   ResourceKind = "DatabaseGrant"
	KindDatabaseBackup  ResourceKind = "DatabaseBackup"
	KindDatabaseRestore ResourceKind = "DatabaseRestore"
)

// Resource represents a loaded YAML resource
type Resource struct {
	APIVersion string
	Kind       ResourceKind
	Metadata   Metadata
	// The actual spec, type depends on Kind
	Spec interface{}
}

// Metadata contains resource metadata
type Metadata struct {
	Name      string
	Namespace string
}

// typeMeta is used for initial parsing to determine the kind
type typeMeta struct {
	APIVersion string   `json:"apiVersion"`
	Kind       string   `json:"kind"`
	Metadata   Metadata `json:"metadata"`
}

// LoadFile loads resources from a YAML file
// Supports multi-document YAML files separated by "---"
func LoadFile(path string) ([]Resource, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	return LoadReader(file)
}

// LoadReader loads resources from a reader
func LoadReader(r io.Reader) ([]Resource, error) {
	var resources []Resource

	// Read all content
	content, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read content: %w", err)
	}

	// Split by document separator
	documents := splitYAMLDocuments(content)

	for i, doc := range documents {
		// Skip empty documents
		if len(bytes.TrimSpace(doc)) == 0 {
			continue
		}

		resource, err := parseDocument(doc)
		if err != nil {
			return nil, fmt.Errorf("failed to parse document %d: %w", i+1, err)
		}

		resources = append(resources, resource)
	}

	if len(resources) == 0 {
		return nil, fmt.Errorf("no valid resources found in file")
	}

	return resources, nil
}

// splitYAMLDocuments splits YAML content by "---" separator
func splitYAMLDocuments(content []byte) [][]byte {
	var documents [][]byte
	var current bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(content))

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			if current.Len() > 0 {
				documents = append(documents, bytes.Clone(current.Bytes()))
				current.Reset()
			}
		} else {
			current.WriteString(line)
			current.WriteString("\n")
		}
	}

	// Add last document
	if current.Len() > 0 {
		documents = append(documents, current.Bytes())
	}

	return documents
}

// parseDocument parses a single YAML document into a Resource
func parseDocument(doc []byte) (Resource, error) {
	// First, parse type metadata
	var meta typeMeta
	if err := yaml.Unmarshal(doc, &meta); err != nil {
		return Resource{}, fmt.Errorf("failed to parse metadata: %w", err)
	}

	// Validate API version
	if meta.APIVersion != expectedAPIVersion {
		return Resource{}, fmt.Errorf("unexpected apiVersion: %s (expected %s)", meta.APIVersion, expectedAPIVersion)
	}

	// Validate kind
	kind := ResourceKind(meta.Kind)
	if !isValidKind(kind) {
		return Resource{}, fmt.Errorf("unsupported kind: %s", meta.Kind)
	}

	// Parse spec based on kind
	spec, err := parseSpec(doc, kind)
	if err != nil {
		return Resource{}, fmt.Errorf("failed to parse spec: %w", err)
	}

	return Resource{
		APIVersion: meta.APIVersion,
		Kind:       kind,
		Metadata:   meta.Metadata,
		Spec:       spec,
	}, nil
}

// isValidKind checks if the kind is valid
func isValidKind(kind ResourceKind) bool {
	switch kind {
	case KindDatabase, KindDatabaseUser, KindDatabaseRole, KindDatabaseGrant,
		KindDatabaseBackup, KindDatabaseRestore:
		return true
	}
	return false
}

// parseSpec parses the spec based on the resource kind
func parseSpec(doc []byte, kind ResourceKind) (interface{}, error) {
	switch kind {
	case KindDatabase:
		var res struct {
			Spec dbopsv1alpha1.DatabaseSpec `json:"spec"`
		}
		if err := yaml.Unmarshal(doc, &res); err != nil {
			return nil, err
		}
		return &res.Spec, nil

	case KindDatabaseUser:
		var res struct {
			Spec dbopsv1alpha1.DatabaseUserSpec `json:"spec"`
		}
		if err := yaml.Unmarshal(doc, &res); err != nil {
			return nil, err
		}
		return &res.Spec, nil

	case KindDatabaseRole:
		var res struct {
			Spec dbopsv1alpha1.DatabaseRoleSpec `json:"spec"`
		}
		if err := yaml.Unmarshal(doc, &res); err != nil {
			return nil, err
		}
		return &res.Spec, nil

	case KindDatabaseGrant:
		var res struct {
			Spec dbopsv1alpha1.DatabaseGrantSpec `json:"spec"`
		}
		if err := yaml.Unmarshal(doc, &res); err != nil {
			return nil, err
		}
		return &res.Spec, nil

	case KindDatabaseBackup:
		var res struct {
			Spec dbopsv1alpha1.DatabaseBackupSpec `json:"spec"`
		}
		if err := yaml.Unmarshal(doc, &res); err != nil {
			return nil, err
		}
		return &res.Spec, nil

	case KindDatabaseRestore:
		var res struct {
			Spec dbopsv1alpha1.DatabaseRestoreSpec `json:"spec"`
		}
		if err := yaml.Unmarshal(doc, &res); err != nil {
			return nil, err
		}
		return &res.Spec, nil

	default:
		return nil, fmt.Errorf("unknown kind: %s", kind)
	}
}

// GetDatabaseSpec type asserts the spec to DatabaseSpec
func (r *Resource) GetDatabaseSpec() (*dbopsv1alpha1.DatabaseSpec, bool) {
	spec, ok := r.Spec.(*dbopsv1alpha1.DatabaseSpec)
	return spec, ok
}

// GetDatabaseUserSpec type asserts the spec to DatabaseUserSpec
func (r *Resource) GetDatabaseUserSpec() (*dbopsv1alpha1.DatabaseUserSpec, bool) {
	spec, ok := r.Spec.(*dbopsv1alpha1.DatabaseUserSpec)
	return spec, ok
}

// GetDatabaseRoleSpec type asserts the spec to DatabaseRoleSpec
func (r *Resource) GetDatabaseRoleSpec() (*dbopsv1alpha1.DatabaseRoleSpec, bool) {
	spec, ok := r.Spec.(*dbopsv1alpha1.DatabaseRoleSpec)
	return spec, ok
}

// GetDatabaseGrantSpec type asserts the spec to DatabaseGrantSpec
func (r *Resource) GetDatabaseGrantSpec() (*dbopsv1alpha1.DatabaseGrantSpec, bool) {
	spec, ok := r.Spec.(*dbopsv1alpha1.DatabaseGrantSpec)
	return spec, ok
}

// GetDatabaseBackupSpec type asserts the spec to DatabaseBackupSpec
func (r *Resource) GetDatabaseBackupSpec() (*dbopsv1alpha1.DatabaseBackupSpec, bool) {
	spec, ok := r.Spec.(*dbopsv1alpha1.DatabaseBackupSpec)
	return spec, ok
}

// GetDatabaseRestoreSpec type asserts the spec to DatabaseRestoreSpec
func (r *Resource) GetDatabaseRestoreSpec() (*dbopsv1alpha1.DatabaseRestoreSpec, bool) {
	spec, ok := r.Spec.(*dbopsv1alpha1.DatabaseRestoreSpec)
	return spec, ok
}
