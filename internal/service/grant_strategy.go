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

// grantStrategy encapsulates engine-specific Apply/Revoke logic for grants.
// New engines plug in by adding a strategy and a factory entry — Apply/Revoke
// in GrantService never need to change.
type grantStrategy interface {
	apply(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseGrantSpec, op *operationLogger) (*GrantResult, error)
	revoke(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseGrantSpec, op *operationLogger) (int, error)
}

// strategyFor returns the strategy that handles the given engine. Engines that
// share wire protocol/grant syntax (Postgres↔CockroachDB, MySQL↔MariaDB) share
// a strategy.
func (s *GrantService) strategyFor(engine dbopsv1alpha1.EngineType) (grantStrategy, error) {
	switch engine {
	case dbopsv1alpha1.EngineTypePostgres, dbopsv1alpha1.EngineTypeCockroachDB:
		return &postgresGrantStrategy{svc: s}, nil
	case dbopsv1alpha1.EngineTypeMySQL, dbopsv1alpha1.EngineTypeMariaDB:
		return &mysqlGrantStrategy{svc: s}, nil
	case dbopsv1alpha1.EngineTypeClickHouse:
		return &clickhouseGrantStrategy{svc: s}, nil
	default:
		return nil, fmt.Errorf("unsupported engine for grants: %s", engine)
	}
}
