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

package cmd

import (
	"context"
	"fmt"

	"github.com/db-provision-operator/cmd/dbctl/internal"
	"github.com/db-provision-operator/internal/service"
)

// executeGrant is a shared helper for applying grants.
// It handles the common logic between create and apply commands.
func executeGrant(
	ctx context.Context,
	cfg *service.Config,
	res internal.Resource,
	username string,
	operation string,
) (*service.Result, error) {
	spec, ok := res.GetDatabaseGrantSpec()
	if !ok {
		return nil, fmt.Errorf("invalid grant spec")
	}

	// Username is required for grants
	if username == "" {
		return nil, fmt.Errorf("--for-user flag is required for grant %s", operation)
	}

	svc, err := service.NewGrantService(cfg)
	if err != nil {
		return nil, err
	}
	defer func() { _ = svc.Close() }()

	if err := svc.Connect(ctx); err != nil {
		return nil, err
	}

	grantResult, err := svc.Apply(ctx, service.ApplyGrantServiceOptions{
		Username: username,
		Spec:     spec,
	})
	if err != nil {
		return nil, err
	}

	// Convert GrantResult to Result
	totalGrants := len(grantResult.AppliedRoles) + grantResult.AppliedDirectGrants + grantResult.AppliedDefaultPrivileges
	return service.NewSuccessResult(fmt.Sprintf("Applied %d grants to user '%s'", totalGrants, username)), nil
}
