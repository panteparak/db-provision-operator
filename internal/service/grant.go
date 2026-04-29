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

package service

import (
	"context"
	"fmt"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/adapter"
	"github.com/db-provision-operator/internal/adapter/types"
)

// GrantService handles database grant operations using the appropriate adapter.
// It extracts business logic from controllers and can be used both by
// Kubernetes controllers and the CLI tool.
type GrantService struct {
	*ResourceService
}

// NewGrantService creates a new GrantService with the given configuration.
func NewGrantService(cfg *Config) (*GrantService, error) {
	rs, err := NewResourceService(cfg, "GrantService")
	if err != nil {
		return nil, err
	}
	return &GrantService{ResourceService: rs}, nil
}

// NewGrantServiceWithAdapter creates a GrantService with a pre-created adapter.
func NewGrantServiceWithAdapter(adp adapter.DatabaseAdapter, cfg *Config) *GrantService {
	return &GrantService{
		ResourceService: NewResourceServiceWithAdapter(adp, cfg, "GrantService"),
	}
}

// ApplyGrantServiceOptions contains options for applying grants.
type ApplyGrantServiceOptions struct {
	Username string
	Spec     *dbopsv1alpha1.DatabaseGrantSpec
}

// GrantResult contains detailed information about grant operations.
// This is returned by Apply/Revoke methods and includes breakdown of applied grants.
type GrantResult struct {
	AppliedRoles             []string
	AppliedDirectGrants      int
	AppliedDefaultPrivileges int
}

// Apply applies all grants from the spec to the specified user.
// This is the main method for the CLI's "create" command.
func (s *GrantService) Apply(ctx context.Context, opts ApplyGrantServiceOptions) (*GrantResult, error) {
	if opts.Username == "" {
		return nil, &ValidationError{Field: "username", Message: "username is required"}
	}
	if opts.Spec == nil {
		return nil, &ValidationError{Field: "spec", Message: "spec is required"}
	}

	op := s.startOp("Apply", opts.Username)

	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	strategy, err := s.strategyFor(s.config.GetEngineType())
	if err != nil {
		return nil, err
	}

	result, err := strategy.apply(ctx, opts.Username, opts.Spec, op)
	if err != nil {
		return nil, err
	}

	op.Success("grants applied successfully")
	return result, nil
}

// Revoke revokes all grants from the spec from the specified user.
// This is the main method for the CLI's "delete" command.
func (s *GrantService) Revoke(ctx context.Context, opts ApplyGrantServiceOptions) (*Result, error) {
	if opts.Username == "" {
		return nil, &ValidationError{Field: "username", Message: "username is required"}
	}
	if opts.Spec == nil {
		return nil, &ValidationError{Field: "spec", Message: "spec is required"}
	}

	op := s.startOp("Revoke", opts.Username)

	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	strategy, err := s.strategyFor(s.config.GetEngineType())
	if err != nil {
		return nil, err
	}

	revokedCount, err := strategy.revoke(ctx, opts.Username, opts.Spec, op)
	if err != nil {
		return nil, err
	}

	op.Success("grants revoked successfully")
	return NewSuccessResult(fmt.Sprintf("Revoked %d grants from user '%s'", revokedCount, opts.Username)), nil
}

// GrantRoles grants the specified roles to a user.
func (s *GrantService) GrantRoles(ctx context.Context, username string, roles []string) (*Result, error) {
	if username == "" {
		return nil, &ValidationError{Field: "username", Message: "username is required"}
	}
	if len(roles) == 0 {
		return nil, &ValidationError{Field: "roles", Message: "at least one role is required"}
	}

	op := s.startOp("GrantRoles", username)
	op.Debug("granting roles", "roles", roles)

	// Apply operation timeout
	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	if err := s.adapter.GrantRole(ctx, username, roles); err != nil {
		op.Error(err, "failed to grant roles")
		return nil, s.wrapError(ctx, s.config, "grant roles", username, err)
	}

	op.Success("roles granted successfully")
	return NewSuccessResult(fmt.Sprintf("Granted %d roles to user '%s'", len(roles), username)), nil
}

// RevokeRoles revokes the specified roles from a user.
func (s *GrantService) RevokeRoles(ctx context.Context, username string, roles []string) (*Result, error) {
	if username == "" {
		return nil, &ValidationError{Field: "username", Message: "username is required"}
	}
	if len(roles) == 0 {
		return nil, &ValidationError{Field: "roles", Message: "at least one role is required"}
	}

	op := s.startOp("RevokeRoles", username)
	op.Debug("revoking roles", "roles", roles)

	// Apply operation timeout
	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	if err := s.adapter.RevokeRole(ctx, username, roles); err != nil {
		op.Error(err, "failed to revoke roles")
		return nil, s.wrapError(ctx, s.config, "revoke roles", username, err)
	}

	op.Success("roles revoked successfully")
	return NewSuccessResult(fmt.Sprintf("Revoked %d roles from user '%s'", len(roles), username)), nil
}

// GetGrants retrieves the current grants for a user/role.
func (s *GrantService) GetGrants(ctx context.Context, grantee string) ([]types.GrantInfo, error) {
	if grantee == "" {
		return nil, &ValidationError{Field: "grantee", Message: "grantee is required"}
	}

	op := s.startOp("GetGrants", grantee)

	// Apply query timeout
	ctx, cancel := s.config.Timeouts.WithQueryTimeout(ctx)
	defer cancel()

	grants, err := s.adapter.GetGrants(ctx, grantee)
	if err != nil {
		op.Error(err, "failed to get grants")
		return nil, s.wrapError(ctx, s.config, "get grants", grantee, err)
	}

	op.Success("retrieved grants")
	return grants, nil
}
