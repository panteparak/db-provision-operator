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

package discovery

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

// Result contains the result of a resource discovery scan.
// It captures resources that exist in the database but are not managed by Kubernetes CRs.
type Result struct {
	// InstanceName is the name of the DatabaseInstance that was scanned
	InstanceName string `json:"instanceName"`

	// ScanTime is when the discovery scan was performed
	ScanTime time.Time `json:"scanTime"`

	// Databases contains discovered database resources
	Databases []DiscoveredResource `json:"databases,omitempty"`

	// Users contains discovered user resources
	Users []DiscoveredResource `json:"users,omitempty"`

	// Roles contains discovered role resources
	Roles []DiscoveredResource `json:"roles,omitempty"`
}

// NewResult creates a new discovery result.
func NewResult(instanceName string) *Result {
	return &Result{
		InstanceName: instanceName,
		ScanTime:     time.Now(),
	}
}

// HasDiscoveries returns true if any unmanaged resources were discovered.
func (r *Result) HasDiscoveries() bool {
	return len(r.Databases) > 0 || len(r.Users) > 0 || len(r.Roles) > 0
}

// TotalCount returns the total number of discovered resources.
func (r *Result) TotalCount() int {
	return len(r.Databases) + len(r.Users) + len(r.Roles)
}

// ToAPIStatus converts the discovery result to the API DiscoveredResourcesStatus type.
func (r *Result) ToAPIStatus() *dbopsv1alpha1.DiscoveredResourcesStatus {
	lastScan := metav1.NewTime(r.ScanTime)
	status := &dbopsv1alpha1.DiscoveredResourcesStatus{
		LastScan: &lastScan,
	}

	for _, db := range r.Databases {
		discovered := metav1.NewTime(db.Discovered)
		status.Databases = append(status.Databases, dbopsv1alpha1.DiscoveredResource{
			Name:       db.Name,
			Discovered: &discovered,
			Adopted:    db.Adopted,
		})
	}

	for _, user := range r.Users {
		discovered := metav1.NewTime(user.Discovered)
		status.Users = append(status.Users, dbopsv1alpha1.DiscoveredResource{
			Name:       user.Name,
			Discovered: &discovered,
			Adopted:    user.Adopted,
		})
	}

	for _, role := range r.Roles {
		discovered := metav1.NewTime(role.Discovered)
		status.Roles = append(status.Roles, dbopsv1alpha1.DiscoveredResource{
			Name:       role.Name,
			Discovered: &discovered,
			Adopted:    role.Adopted,
		})
	}

	return status
}

// DiscoveredResource represents a resource found in the database
// that is not managed by a Kubernetes CR.
type DiscoveredResource struct {
	// Name is the name of the discovered resource
	Name string `json:"name"`

	// Type is the type of resource (database, user, role)
	Type string `json:"type"`

	// Discovered is when this resource was first discovered
	Discovered time.Time `json:"discovered"`

	// Adopted indicates if this resource has been adopted via annotation
	Adopted bool `json:"adopted"`
}

// AdoptionResult contains the result of resource adoption.
type AdoptionResult struct {
	// InstanceName is the name of the DatabaseInstance
	InstanceName string

	// Adopted contains resources that were successfully adopted
	Adopted []AdoptedResource

	// Failed contains resources that failed to adopt
	Failed []FailedAdoption
}

// NewAdoptionResult creates a new adoption result.
func NewAdoptionResult(instanceName string) *AdoptionResult {
	return &AdoptionResult{
		InstanceName: instanceName,
	}
}

// HasAdoptions returns true if any resources were adopted.
func (r *AdoptionResult) HasAdoptions() bool {
	return len(r.Adopted) > 0
}

// HasFailures returns true if any adoptions failed.
func (r *AdoptionResult) HasFailures() bool {
	return len(r.Failed) > 0
}

// AddAdopted adds a successfully adopted resource.
func (r *AdoptionResult) AddAdopted(resourceType, name, crName string) {
	r.Adopted = append(r.Adopted, AdoptedResource{
		Type:   resourceType,
		Name:   name,
		CRName: crName,
	})
}

// AddFailed adds a failed adoption.
func (r *AdoptionResult) AddFailed(resourceType, name string, err error) {
	r.Failed = append(r.Failed, FailedAdoption{
		Type:  resourceType,
		Name:  name,
		Error: err,
	})
}

// AdoptedResource represents a resource that was successfully adopted.
type AdoptedResource struct {
	// Type is the type of resource (database, user, role)
	Type string

	// Name is the name in the database
	Name string

	// CRName is the name of the created Kubernetes CR
	CRName string
}

// FailedAdoption represents a resource that failed to adopt.
type FailedAdoption struct {
	// Type is the type of resource
	Type string

	// Name is the name in the database
	Name string

	// Error is the adoption error
	Error error
}
