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
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/adapter"
	"github.com/db-provision-operator/internal/service"
)

// Service handles resource discovery and adoption operations.
// It contains the business logic for discovering database resources that
// exist in the database but are not managed by Kubernetes CRs, as well as
// adopting those resources by creating corresponding CRs.
//
// The service follows the same patterns as other services in this package:
// - Uses structured logging via logr
// - Returns typed results for controllers to process
// - Does not directly emit events (that's the controller's job)
type Service struct {
	adapter   adapter.DatabaseAdapter
	k8sClient client.Client
	config    *Config
	log       logr.Logger
}

// Config contains configuration for resource discovery.
type Config struct {
	// InstanceName is the name of the DatabaseInstance being scanned
	InstanceName string

	// InstanceNamespace is the namespace of the DatabaseInstance
	InstanceNamespace string

	// Engine is the database engine type (for building CRs)
	Engine dbopsv1alpha1.EngineType

	// Logger is the logger to use for discovery operations
	Logger logr.Logger
}

// NewService creates a new discovery service.
// The adapter should implement ResourceDiscovery interface for full functionality.
func NewService(adp adapter.DatabaseAdapter, k8sClient client.Client, cfg *Config) *Service {
	log := cfg.Logger
	if log.GetSink() == nil {
		log = logr.Discard()
	}

	return &Service{
		adapter:   adp,
		k8sClient: k8sClient,
		config:    cfg,
		log:       log.WithName("DiscoveryService"),
	}
}

// NewConfig creates a new discovery service config.
func NewConfig(serviceCfg *service.Config, instanceName, instanceNamespace string, engine dbopsv1alpha1.EngineType) *Config {
	return &Config{
		InstanceName:      instanceName,
		InstanceNamespace: instanceNamespace,
		Engine:            engine,
		Logger:            serviceCfg.GetLogger(),
	}
}

// Discover scans the database for resources not managed by Kubernetes CRs.
// It compares resources in the database against existing CRs and returns
// those that are unmanaged ("orphaned" from Kubernetes perspective).
//
// The method:
// 1. Lists all databases, users, and roles from the database adapter
// 2. Lists all Database, DatabaseUser, and DatabaseRole CRs for this instance
// 3. Returns resources found in DB but not in Kubernetes
func (s *Service) Discover(ctx context.Context) (*Result, error) {
	log := s.log.WithValues("instance", s.config.InstanceName)
	log.V(1).Info("starting resource discovery")

	result := NewResult(s.config.InstanceName)

	// Check if adapter supports resource discovery
	discoverer, ok := s.adapter.(adapter.ResourceDiscovery)
	if !ok {
		return nil, fmt.Errorf("adapter does not support resource discovery")
	}

	// Get managed resources from Kubernetes
	managedDBs, err := s.getManagedDatabases(ctx)
	if err != nil {
		log.Error(err, "failed to get managed databases")
		return nil, fmt.Errorf("get managed databases: %w", err)
	}

	managedUsers, err := s.getManagedUsers(ctx)
	if err != nil {
		log.Error(err, "failed to get managed users")
		return nil, fmt.Errorf("get managed users: %w", err)
	}

	managedRoles, err := s.getManagedRoles(ctx)
	if err != nil {
		log.Error(err, "failed to get managed roles")
		return nil, fmt.Errorf("get managed roles: %w", err)
	}

	// Get actual resources from database (adapter filters system resources)
	actualDBs, err := discoverer.ListDatabases(ctx)
	if err != nil {
		log.Error(err, "failed to list databases from DB")
		return nil, fmt.Errorf("list databases: %w", err)
	}

	actualUsers, err := discoverer.ListUsers(ctx)
	if err != nil {
		log.Error(err, "failed to list users from DB")
		return nil, fmt.Errorf("list users: %w", err)
	}

	actualRoles, err := discoverer.ListRoles(ctx)
	if err != nil {
		log.Error(err, "failed to list roles from DB")
		return nil, fmt.Errorf("list roles: %w", err)
	}

	// Find orphans (in DB but not managed by K8s)
	result.Databases = s.findOrphans("database", actualDBs, managedDBs)
	result.Users = s.findOrphans("user", actualUsers, managedUsers)
	result.Roles = s.findOrphans("role", actualRoles, managedRoles)

	if result.HasDiscoveries() {
		log.Info("discovery completed",
			"databases", len(result.Databases),
			"users", len(result.Users),
			"roles", len(result.Roles))
	} else {
		log.V(1).Info("no unmanaged resources discovered")
	}

	return result, nil
}

// AdoptDatabases creates Database CRs for the specified discovered databases.
// Each CR is created with a reference to the source DatabaseInstance and
// with drift detection enabled in "detect" mode by default.
func (s *Service) AdoptDatabases(ctx context.Context, databases []string) (*AdoptionResult, error) {
	log := s.log.WithValues("instance", s.config.InstanceName)
	log.Info("adopting databases", "count", len(databases))

	result := NewAdoptionResult(s.config.InstanceName)

	for _, dbName := range databases {
		cr := s.buildDatabaseCR(dbName)
		if err := s.k8sClient.Create(ctx, cr); err != nil {
			result.AddFailed("database", dbName, err)
			log.Error(err, "failed to adopt database", "database", dbName)
			continue
		}
		result.AddAdopted("database", dbName, cr.Name)
		log.Info("adopted database", "database", dbName, "cr", cr.Name)
	}

	return result, nil
}

// AdoptUsers creates DatabaseUser CRs for the specified discovered users.
func (s *Service) AdoptUsers(ctx context.Context, users []string) (*AdoptionResult, error) {
	log := s.log.WithValues("instance", s.config.InstanceName)
	log.Info("adopting users", "count", len(users))

	result := NewAdoptionResult(s.config.InstanceName)

	for _, userName := range users {
		cr := s.buildUserCR(userName)
		if err := s.k8sClient.Create(ctx, cr); err != nil {
			result.AddFailed("user", userName, err)
			log.Error(err, "failed to adopt user", "user", userName)
			continue
		}
		result.AddAdopted("user", userName, cr.Name)
		log.Info("adopted user", "user", userName, "cr", cr.Name)
	}

	return result, nil
}

// AdoptRoles creates DatabaseRole CRs for the specified discovered roles.
func (s *Service) AdoptRoles(ctx context.Context, roles []string) (*AdoptionResult, error) {
	log := s.log.WithValues("instance", s.config.InstanceName)
	log.Info("adopting roles", "count", len(roles))

	result := NewAdoptionResult(s.config.InstanceName)

	for _, roleName := range roles {
		cr := s.buildRoleCR(roleName)
		if err := s.k8sClient.Create(ctx, cr); err != nil {
			result.AddFailed("role", roleName, err)
			log.Error(err, "failed to adopt role", "role", roleName)
			continue
		}
		result.AddAdopted("role", roleName, cr.Name)
		log.Info("adopted role", "role", roleName, "cr", cr.Name)
	}

	return result, nil
}

// getManagedDatabases returns database names managed by Database CRs for this instance.
func (s *Service) getManagedDatabases(ctx context.Context) (map[string]bool, error) {
	managed := make(map[string]bool)

	var dbList dbopsv1alpha1.DatabaseList
	if err := s.k8sClient.List(ctx, &dbList,
		client.InNamespace(s.config.InstanceNamespace)); err != nil {
		return nil, err
	}

	for _, db := range dbList.Items {
		// Only include databases referencing this instance
		if db.Spec.InstanceRef.Name == s.config.InstanceName {
			managed[db.Spec.Name] = true
		}
	}

	return managed, nil
}

// getManagedUsers returns usernames managed by DatabaseUser CRs for this instance.
func (s *Service) getManagedUsers(ctx context.Context) (map[string]bool, error) {
	managed := make(map[string]bool)

	var userList dbopsv1alpha1.DatabaseUserList
	if err := s.k8sClient.List(ctx, &userList,
		client.InNamespace(s.config.InstanceNamespace)); err != nil {
		return nil, err
	}

	for _, user := range userList.Items {
		// Only include users referencing this instance
		if user.Spec.InstanceRef.Name == s.config.InstanceName {
			managed[user.Spec.Username] = true
		}
	}

	return managed, nil
}

// getManagedRoles returns role names managed by DatabaseRole CRs for this instance.
func (s *Service) getManagedRoles(ctx context.Context) (map[string]bool, error) {
	managed := make(map[string]bool)

	var roleList dbopsv1alpha1.DatabaseRoleList
	if err := s.k8sClient.List(ctx, &roleList,
		client.InNamespace(s.config.InstanceNamespace)); err != nil {
		return nil, err
	}

	for _, role := range roleList.Items {
		// Only include roles referencing this instance
		if role.Spec.InstanceRef.Name == s.config.InstanceName {
			managed[role.Spec.RoleName] = true
		}
	}

	return managed, nil
}

// findOrphans returns resources that exist in the database but not in Kubernetes.
func (s *Service) findOrphans(resourceType string, actual []string, managed map[string]bool) []DiscoveredResource {
	var orphans []DiscoveredResource
	now := time.Now()

	for _, name := range actual {
		if !managed[name] {
			orphans = append(orphans, DiscoveredResource{
				Name:       name,
				Type:       resourceType,
				Discovered: now,
				Adopted:    false,
			})
		}
	}

	return orphans
}

// buildDatabaseCR creates a Database CR for adoption.
func (s *Service) buildDatabaseCR(dbName string) *dbopsv1alpha1.Database {
	// Use the database name as the CR name (sanitized for K8s)
	crName := sanitizeK8sName(dbName)

	db := &dbopsv1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crName,
			Namespace: s.config.InstanceNamespace,
			Labels: map[string]string{
				dbopsv1alpha1.LabelInstance: s.config.InstanceName,
				dbopsv1alpha1.LabelAdopted:  "true",
			},
		},
		Spec: dbopsv1alpha1.DatabaseSpec{
			Name: dbName,
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: s.config.InstanceName,
			},
			DriftPolicy: &dbopsv1alpha1.DriftPolicy{
				Mode: dbopsv1alpha1.DriftModeDetect, // Safe default for adopted resources
			},
		},
	}

	// Set engine-specific config based on instance engine
	switch s.config.Engine {
	case dbopsv1alpha1.EngineTypePostgres, dbopsv1alpha1.EngineTypeCockroachDB:
		db.Spec.Postgres = &dbopsv1alpha1.PostgresDatabaseConfig{}
	case dbopsv1alpha1.EngineTypeMySQL, dbopsv1alpha1.EngineTypeMariaDB:
		db.Spec.MySQL = &dbopsv1alpha1.MySQLDatabaseConfig{}
	}

	return db
}

// buildUserCR creates a DatabaseUser CR for adoption.
func (s *Service) buildUserCR(username string) *dbopsv1alpha1.DatabaseUser {
	crName := sanitizeK8sName(username)

	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crName,
			Namespace: s.config.InstanceNamespace,
			Labels: map[string]string{
				dbopsv1alpha1.LabelInstance: s.config.InstanceName,
				dbopsv1alpha1.LabelAdopted:  "true",
			},
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: username,
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: s.config.InstanceName,
			},
			DriftPolicy: &dbopsv1alpha1.DriftPolicy{
				Mode: dbopsv1alpha1.DriftModeDetect,
			},
		},
	}

	// Set engine-specific config
	switch s.config.Engine {
	case dbopsv1alpha1.EngineTypePostgres, dbopsv1alpha1.EngineTypeCockroachDB:
		user.Spec.Postgres = &dbopsv1alpha1.PostgresUserConfig{
			Inherit: true, // Default PostgreSQL behavior
		}
	case dbopsv1alpha1.EngineTypeMySQL, dbopsv1alpha1.EngineTypeMariaDB:
		user.Spec.MySQL = &dbopsv1alpha1.MySQLUserConfig{}
	}

	return user
}

// buildRoleCR creates a DatabaseRole CR for adoption.
func (s *Service) buildRoleCR(roleName string) *dbopsv1alpha1.DatabaseRole {
	crName := sanitizeK8sName(roleName)

	role := &dbopsv1alpha1.DatabaseRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crName,
			Namespace: s.config.InstanceNamespace,
			Labels: map[string]string{
				dbopsv1alpha1.LabelInstance: s.config.InstanceName,
				dbopsv1alpha1.LabelAdopted:  "true",
			},
		},
		Spec: dbopsv1alpha1.DatabaseRoleSpec{
			RoleName: roleName,
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: s.config.InstanceName,
			},
			DriftPolicy: &dbopsv1alpha1.DriftPolicy{
				Mode: dbopsv1alpha1.DriftModeDetect,
			},
		},
	}

	// Set engine-specific config
	switch s.config.Engine {
	case dbopsv1alpha1.EngineTypePostgres, dbopsv1alpha1.EngineTypeCockroachDB:
		role.Spec.Postgres = &dbopsv1alpha1.PostgresRoleConfig{
			Inherit: true, // Default PostgreSQL behavior
		}
	case dbopsv1alpha1.EngineTypeMySQL, dbopsv1alpha1.EngineTypeMariaDB:
		role.Spec.MySQL = &dbopsv1alpha1.MySQLRoleConfig{}
	}

	return role
}

// sanitizeK8sName converts a database resource name to a valid Kubernetes resource name.
// Kubernetes names must:
// - Be lowercase
// - Start with an alphanumeric character
// - Contain only alphanumeric characters, '-', or '.'
// - End with an alphanumeric character
// - Be at most 253 characters
func sanitizeK8sName(name string) string {
	result := make([]byte, 0, len(name))

	for i, c := range []byte(name) {
		switch {
		case c >= 'a' && c <= 'z':
			result = append(result, c)
		case c >= 'A' && c <= 'Z':
			result = append(result, c+32) // Convert to lowercase
		case c >= '0' && c <= '9':
			// Numbers can't be at the start
			if i == 0 {
				result = append(result, 'n', '-')
			}
			result = append(result, c)
		case c == '-' || c == '_' || c == '.':
			// Replace underscore with dash
			if c == '_' {
				c = '-'
			}
			// Don't start with dash or dot
			if i > 0 && len(result) > 0 {
				result = append(result, c)
			}
		default:
			// Skip other characters
		}
	}

	// Remove trailing dash or dot
	for len(result) > 0 && (result[len(result)-1] == '-' || result[len(result)-1] == '.') {
		result = result[:len(result)-1]
	}

	// Ensure we have at least one character
	if len(result) == 0 {
		result = []byte("unnamed")
	}

	// Truncate if too long
	if len(result) > 253 {
		result = result[:253]
	}

	return string(result)
}
