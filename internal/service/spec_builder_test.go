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
	"testing"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
)

func TestGetSpecBuilder(t *testing.T) {
	tests := []struct {
		name         string
		engine       dbopsv1alpha1.EngineType
		expectedType string
	}{
		{
			name:         "postgres returns PostgresSpecBuilder",
			engine:       dbopsv1alpha1.EngineTypePostgres,
			expectedType: "*service.postgresSpecBuilder",
		},
		{
			name:         "mysql returns MySQLSpecBuilder",
			engine:       dbopsv1alpha1.EngineTypeMySQL,
			expectedType: "*service.mysqlSpecBuilder",
		},
		{
			name:         "mariadb returns MySQLSpecBuilder (MySQL-compatible)",
			engine:       dbopsv1alpha1.EngineTypeMariaDB,
			expectedType: "*service.mysqlSpecBuilder",
		},
		{
			name:         "cockroachdb returns PostgresSpecBuilder (PG wire-compatible)",
			engine:       dbopsv1alpha1.EngineTypeCockroachDB,
			expectedType: "*service.postgresSpecBuilder",
		},
		{
			name:         "unknown engine returns emptySpecBuilder",
			engine:       dbopsv1alpha1.EngineType("unknown"),
			expectedType: "*service.emptySpecBuilder",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := GetSpecBuilder(tt.engine)
			assert.NotNil(t, builder)
			assert.IsType(t, builder, builder, "expected type %s", tt.expectedType)

			// Verify by checking concrete type via type assertion
			switch tt.expectedType {
			case "*service.postgresSpecBuilder":
				_, ok := builder.(*postgresSpecBuilder)
				assert.True(t, ok, "expected *postgresSpecBuilder, got %T", builder)
			case "*service.mysqlSpecBuilder":
				_, ok := builder.(*mysqlSpecBuilder)
				assert.True(t, ok, "expected *mysqlSpecBuilder, got %T", builder)
			case "*service.emptySpecBuilder":
				_, ok := builder.(*emptySpecBuilder)
				assert.True(t, ok, "expected *emptySpecBuilder, got %T", builder)
			}
		})
	}
}
