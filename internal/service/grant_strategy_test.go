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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

func TestStrategyFor_RoutesByEngine(t *testing.T) {
	svc := &GrantService{}

	tests := []struct {
		engine    dbopsv1alpha1.EngineType
		want      string
		wantError bool
	}{
		{dbopsv1alpha1.EngineTypePostgres, "*service.postgresGrantStrategy", false},
		{dbopsv1alpha1.EngineTypeCockroachDB, "*service.postgresGrantStrategy", false},
		{dbopsv1alpha1.EngineTypeMySQL, "*service.mysqlGrantStrategy", false},
		{dbopsv1alpha1.EngineTypeMariaDB, "*service.mysqlGrantStrategy", false},
		{dbopsv1alpha1.EngineTypeClickHouse, "*service.clickhouseGrantStrategy", false},
		{dbopsv1alpha1.EngineType("bogus"), "", true},
	}

	for _, tt := range tests {
		t.Run(string(tt.engine), func(t *testing.T) {
			got, err := svc.strategyFor(tt.engine)
			if tt.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, got)
		})
	}
}

func TestBuildPostgresGrantOptions_PreservesAllFields(t *testing.T) {
	grants := []dbopsv1alpha1.PostgresGrant{
		{
			Database:        "appdb",
			Schema:          "public",
			Tables:          []string{"users", "orders"},
			Sequences:       []string{"order_id_seq"},
			Functions:       []string{"audit_log"},
			Privileges:      []string{"SELECT", "INSERT"},
			WithGrantOption: true,
		},
	}

	got := buildPostgresGrantOptions(grants)

	require.Len(t, got, 1)
	assert.Equal(t, "appdb", got[0].Database)
	assert.Equal(t, "public", got[0].Schema)
	assert.Equal(t, []string{"users", "orders"}, got[0].Tables)
	assert.Equal(t, []string{"order_id_seq"}, got[0].Sequences)
	assert.Equal(t, []string{"audit_log"}, got[0].Functions)
	assert.Equal(t, []string{"SELECT", "INSERT"}, got[0].Privileges)
	assert.True(t, got[0].WithGrantOption)
}

func TestBuildMySQLGrantOptions_PreservesLevelAsString(t *testing.T) {
	grants := []dbopsv1alpha1.MySQLGrant{
		{
			Level:           dbopsv1alpha1.MySQLGrantLevelTable,
			Database:        "appdb",
			Table:           "users",
			Columns:         []string{"id", "name"},
			Privileges:      []string{"SELECT"},
			WithGrantOption: false,
		},
	}

	got := buildMySQLGrantOptions(grants)

	require.Len(t, got, 1)
	assert.Equal(t, string(dbopsv1alpha1.MySQLGrantLevelTable), got[0].Level)
	assert.Equal(t, "appdb", got[0].Database)
	assert.Equal(t, "users", got[0].Table)
	assert.Equal(t, []string{"id", "name"}, got[0].Columns)
}

func TestBuildClickHouseGrantOptions_PreservesLevelAsString(t *testing.T) {
	grants := []dbopsv1alpha1.ClickHouseGrant{
		{
			Level:      dbopsv1alpha1.ClickHouseGrantLevelTable,
			Database:   "analytics",
			Table:      "events",
			Privileges: []string{"SELECT"},
		},
	}

	got := buildClickHouseGrantOptions(grants)

	require.Len(t, got, 1)
	assert.Equal(t, string(dbopsv1alpha1.ClickHouseGrantLevelTable), got[0].Level)
	assert.Equal(t, "analytics", got[0].Database)
	assert.Equal(t, "events", got[0].Table)
}

func TestBuildDefaultPrivilegeOptions_PreservesAllFields(t *testing.T) {
	defPrivs := []dbopsv1alpha1.PostgresDefaultPrivilegeGrant{
		{
			Database:   "appdb",
			Schema:     "public",
			GrantedBy:  "owner_role",
			ObjectType: "TABLES",
			Privileges: []string{"SELECT"},
		},
	}

	got := buildDefaultPrivilegeOptions(defPrivs)

	require.Len(t, got, 1)
	assert.Equal(t, "appdb", got[0].Database)
	assert.Equal(t, "public", got[0].Schema)
	assert.Equal(t, "owner_role", got[0].GrantedBy)
	assert.Equal(t, "TABLES", got[0].ObjectType)
}
