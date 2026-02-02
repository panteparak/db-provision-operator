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

package adapter

import (
	"fmt"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/adapter/cockroachdb"
	"github.com/db-provision-operator/internal/adapter/mysql"
	"github.com/db-provision-operator/internal/adapter/postgres"
)

// NewAdapter creates a new database adapter based on the engine type
func NewAdapter(engine dbopsv1alpha1.EngineType, config ConnectionConfig) (DatabaseAdapter, error) {
	switch engine {
	case dbopsv1alpha1.EngineTypePostgres:
		return postgres.NewAdapter(config), nil
	case dbopsv1alpha1.EngineTypeMySQL, dbopsv1alpha1.EngineTypeMariaDB:
		// MariaDB is MySQL-compatible and uses the same adapter
		return mysql.NewAdapter(config), nil
	case dbopsv1alpha1.EngineTypeCockroachDB:
		return cockroachdb.NewAdapter(config), nil
	default:
		return nil, fmt.Errorf("unsupported engine type: %s", engine)
	}
}

// BuildConnectionConfig builds a ConnectionConfig from DatabaseInstance spec and credentials
func BuildConnectionConfig(
	spec *dbopsv1alpha1.DatabaseInstanceSpec,
	username, password string,
	tlsCA, tlsCert, tlsKey []byte,
) ConnectionConfig {
	config := ConnectionConfig{
		Host:     spec.Connection.Host,
		Port:     spec.Connection.Port,
		Database: spec.Connection.Database,
		Username: username,
		Password: password,
	}

	// TLS configuration
	if spec.TLS != nil && spec.TLS.Enabled {
		config.TLSEnabled = true
		config.TLSMode = spec.TLS.Mode
		config.TLSCA = tlsCA
		config.TLSCert = tlsCert
		config.TLSKey = tlsKey
	}

	// PostgreSQL-specific configuration
	if spec.Postgres != nil {
		config.SSLMode = string(spec.Postgres.SSLMode)
		config.ConnectTimeout = spec.Postgres.ConnectTimeout
		config.StatementTimeout = spec.Postgres.StatementTimeout
		config.ApplicationName = spec.Postgres.ApplicationName
	}

	// MySQL-specific configuration
	if spec.MySQL != nil {
		config.Charset = spec.MySQL.Charset
		config.Collation = spec.MySQL.Collation
		config.ParseTime = spec.MySQL.ParseTime
		config.Timeout = spec.MySQL.Timeout
		config.ReadTimeout = spec.MySQL.ReadTimeout
		config.WriteTimeout = spec.MySQL.WriteTimeout
	}

	return config
}
