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
)

// ownershipProvisionPlan captures the state and pipeline for provisioning a
// database with auto-ownership. Wrapping the closure-captured create result
// keeps the pipeline declaration in one place and out of CreateWithOwnership.
type ownershipProvisionPlan struct {
	svc          *DatabaseService
	spec         *dbopsv1alpha1.DatabaseSpec
	ownerSvc     *OwnershipService
	roleName     string
	userName     string
	schemaNames  []string
	createResult *Result
	pipeline     *ProvisionPipeline
}

// newOwnershipProvisionPlan prepares the spec for ownership-driven provisioning
// and assembles the pipeline of steps to execute. Mutates spec in place to
// default schema owners and override spec.Owner — preserving the pre-refactor
// behavior of CreateWithOwnership.
func newOwnershipProvisionPlan(s *DatabaseService, spec *dbopsv1alpha1.DatabaseSpec) *ownershipProvisionPlan {
	ownershipCfg := spec.Postgres.Ownership
	roleName := DeriveRoleName(ownershipCfg, spec.Name)
	userName := DeriveUserName(ownershipCfg, spec.Name)

	for i := range spec.Postgres.Schemas {
		if spec.Postgres.Schemas[i].Owner == "" {
			spec.Postgres.Schemas[i].Owner = roleName
		}
	}
	spec.Owner = roleName

	plan := &ownershipProvisionPlan{
		svc:         s,
		spec:        spec,
		ownerSvc:    NewOwnershipService(s.ResourceService),
		roleName:    roleName,
		userName:    userName,
		schemaNames: collectSchemaNames(spec.Postgres.Schemas),
	}
	plan.pipeline = plan.buildPipeline()
	return plan
}

func collectSchemaNames(schemas []dbopsv1alpha1.PostgresSchema) []string {
	names := make([]string, 0, len(schemas))
	for _, schema := range schemas {
		names = append(names, schema.Name)
	}
	return names
}

func (p *ownershipProvisionPlan) buildPipeline() *ProvisionPipeline {
	steps := []ProvisionStep{
		{Name: "create-owner-role", Execute: p.stepCreateOwnerRole},
		{Name: "create-database", Execute: p.stepCreateDatabase},
		{Name: "verify-access", Execute: p.stepVerifyAccess},
		{Name: "transfer-ownership", Execute: p.stepTransferOwnership},
		{Name: "create-app-user", Execute: p.stepCreateAppUser},
	}

	if p.spec.Postgres.Ownership.ShouldSetDefaultPrivileges() {
		steps = append(steps, ProvisionStep{
			Name:    "set-default-privileges",
			Execute: p.stepSetDefaultPrivileges,
		})
	}

	steps = append(steps, ProvisionStep{
		Name:    "update-database",
		Execute: p.stepUpdateDatabase,
	})

	return &ProvisionPipeline{Steps: steps}
}

func (p *ownershipProvisionPlan) stepCreateOwnerRole(ctx context.Context) error {
	return p.ownerSvc.EnsureOwnerRole(ctx, p.roleName)
}

func (p *ownershipProvisionPlan) stepCreateDatabase(ctx context.Context) error {
	result, err := p.svc.CreateOnly(ctx, p.spec)
	if err != nil {
		return err
	}
	p.createResult = result
	return nil
}

func (p *ownershipProvisionPlan) stepVerifyAccess(ctx context.Context) error {
	return p.svc.adapter.VerifyDatabaseAccess(ctx, p.spec.Name)
}

func (p *ownershipProvisionPlan) stepTransferOwnership(ctx context.Context) error {
	return p.ownerSvc.TransferOwnership(ctx, p.spec.Name, p.roleName)
}

func (p *ownershipProvisionPlan) stepCreateAppUser(ctx context.Context) error {
	return p.ownerSvc.EnsureOwnerUser(ctx, p.userName, p.roleName)
}

func (p *ownershipProvisionPlan) stepSetDefaultPrivileges(ctx context.Context) error {
	return p.ownerSvc.SetDefaultPrivileges(ctx, p.spec.Name, p.roleName, p.userName, p.schemaNames)
}

func (p *ownershipProvisionPlan) stepUpdateDatabase(ctx context.Context) error {
	opts := p.svc.specBuilder.BuildDatabaseUpdateOptions(p.spec)
	if len(opts.Extensions) == 0 && len(opts.Schemas) == 0 && len(opts.DefaultPrivileges) == 0 {
		return nil
	}
	return p.svc.adapter.UpdateDatabase(ctx, p.spec.Name, opts)
}

// run executes the pipeline and assembles the final results. The first return
// value is the create result (possibly nil if create-database hadn't run yet
// when an earlier step failed), the second is the ownership result on success,
// and the third is the failure error if any step failed.
func (p *ownershipProvisionPlan) run(ctx context.Context) (*Result, *OwnershipResult, error) {
	_, err := p.pipeline.Run(ctx, p.svc.ResourceService)
	if err != nil {
		return p.createResult, nil, err
	}

	createResult := p.createResult
	if createResult == nil {
		createResult = NewCreatedResult(fmt.Sprintf("Database '%s' created with ownership", p.spec.Name))
	}

	return createResult, &OwnershipResult{RoleName: p.roleName, UserName: p.userName}, nil
}
