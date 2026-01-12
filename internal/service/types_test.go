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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/util"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "config is nil",
		},
		{
			name:    "missing engine",
			config:  &Config{Host: "localhost", Port: 5432, Username: "user"},
			wantErr: true,
			errMsg:  "engine is required",
		},
		{
			name:    "invalid engine",
			config:  &Config{Engine: "oracle", Host: "localhost", Port: 5432, Username: "user"},
			wantErr: true,
			errMsg:  "engine must be 'postgres' or 'mysql'",
		},
		{
			name:    "missing host",
			config:  &Config{Engine: "postgres", Port: 5432, Username: "user"},
			wantErr: true,
			errMsg:  "host is required",
		},
		{
			name:    "invalid port zero",
			config:  &Config{Engine: "postgres", Host: "localhost", Port: 0, Username: "user"},
			wantErr: true,
			errMsg:  "port must be between 1 and 65535",
		},
		{
			name:    "invalid port negative",
			config:  &Config{Engine: "postgres", Host: "localhost", Port: -1, Username: "user"},
			wantErr: true,
			errMsg:  "port must be between 1 and 65535",
		},
		{
			name:    "invalid port too high",
			config:  &Config{Engine: "postgres", Host: "localhost", Port: 70000, Username: "user"},
			wantErr: true,
			errMsg:  "port must be between 1 and 65535",
		},
		{
			name:    "missing username",
			config:  &Config{Engine: "postgres", Host: "localhost", Port: 5432},
			wantErr: true,
			errMsg:  "username is required",
		},
		{
			name:    "invalid ssl mode",
			config:  &Config{Engine: "postgres", Host: "localhost", Port: 5432, Username: "user", SSLMode: "invalid"},
			wantErr: true,
			errMsg:  "invalid SSL mode",
		},
		{
			name:    "valid postgres config",
			config:  &Config{Engine: "postgres", Host: "localhost", Port: 5432, Username: "user"},
			wantErr: false,
		},
		{
			name:    "valid mysql config",
			config:  &Config{Engine: "mysql", Host: "localhost", Port: 3306, Username: "user"},
			wantErr: false,
		},
		{
			name:    "valid with ssl mode require",
			config:  &Config{Engine: "postgres", Host: "localhost", Port: 5432, Username: "user", SSLMode: "require"},
			wantErr: false,
		},
		{
			name:    "valid with ssl mode verify-full",
			config:  &Config{Engine: "postgres", Host: "localhost", Port: 5432, Username: "user", SSLMode: "verify-full"},
			wantErr: false,
		},
		{
			name:    "valid with empty ssl mode",
			config:  &Config{Engine: "postgres", Host: "localhost", Port: 5432, Username: "user", SSLMode: ""},
			wantErr: false,
		},
		{
			name:    "valid without password (cert auth)",
			config:  &Config{Engine: "postgres", Host: "localhost", Port: 5432, Username: "user"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_Clone(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		var cfg *Config
		clone := cfg.Clone()
		assert.Nil(t, clone)
	})

	t.Run("basic config clone", func(t *testing.T) {
		cfg := &Config{
			Engine:   "postgres",
			Host:     "localhost",
			Port:     5432,
			Database: "testdb",
			Username: "user",
			Password: "secret",
		}

		clone := cfg.Clone()
		require.NotNil(t, clone)
		assert.Equal(t, cfg.Engine, clone.Engine)
		assert.Equal(t, cfg.Host, clone.Host)
		assert.Equal(t, cfg.Port, clone.Port)
		assert.Equal(t, cfg.Database, clone.Database)
		assert.Equal(t, cfg.Username, clone.Username)
		assert.Equal(t, cfg.Password, clone.Password)

		// Modify original, ensure clone is unchanged
		cfg.Host = "modified"
		assert.Equal(t, "localhost", clone.Host)
	})

	t.Run("clone with TLS credentials", func(t *testing.T) {
		cfg := &Config{
			Engine:   "postgres",
			Host:     "localhost",
			Port:     5432,
			Username: "user",
			TLSCA:    []byte("ca-cert"),
			TLSCert:  []byte("client-cert"),
			TLSKey:   []byte("client-key"),
		}

		clone := cfg.Clone()
		require.NotNil(t, clone)
		assert.Equal(t, cfg.TLSCA, clone.TLSCA)
		assert.Equal(t, cfg.TLSCert, clone.TLSCert)
		assert.Equal(t, cfg.TLSKey, clone.TLSKey)

		// Modify original byte slices, ensure clone is unchanged
		cfg.TLSCA[0] = 'X'
		assert.Equal(t, byte('c'), clone.TLSCA[0])
	})
}

func TestConfig_WithTimeouts(t *testing.T) {
	cfg := &Config{
		Engine:   "postgres",
		Host:     "localhost",
		Port:     5432,
		Username: "user",
		Timeouts: util.DefaultTimeoutConfig(),
	}

	fastTimeouts := util.FastTimeoutConfig()
	newCfg := cfg.WithTimeouts(fastTimeouts)

	// Original should be unchanged
	assert.Equal(t, util.DefaultTimeoutConfig().ConnectTimeout, cfg.Timeouts.ConnectTimeout)

	// New config should have fast timeouts
	assert.Equal(t, fastTimeouts.ConnectTimeout, newCfg.Timeouts.ConnectTimeout)
	assert.Equal(t, fastTimeouts.OperationTimeout, newCfg.Timeouts.OperationTimeout)
}

func TestConfig_GetEngineType(t *testing.T) {
	tests := []struct {
		engine   string
		expected dbopsv1alpha1.EngineType
	}{
		{"postgres", dbopsv1alpha1.EngineTypePostgres},
		{"mysql", dbopsv1alpha1.EngineTypeMySQL},
		{"unknown", dbopsv1alpha1.EngineType("unknown")},
	}

	for _, tt := range tests {
		t.Run(tt.engine, func(t *testing.T) {
			cfg := &Config{Engine: tt.engine}
			assert.Equal(t, tt.expected, cfg.GetEngineType())
		})
	}
}

func TestConfigBuilder(t *testing.T) {
	t.Run("build valid postgres config", func(t *testing.T) {
		cfg, err := NewConfigBuilder().
			WithEngine("postgres").
			WithHost("localhost").
			WithCredentials("admin", "secret").
			Build()

		require.NoError(t, err)
		assert.Equal(t, "postgres", cfg.Engine)
		assert.Equal(t, "localhost", cfg.Host)
		assert.Equal(t, int32(5432), cfg.Port)    // Default port
		assert.Equal(t, "postgres", cfg.Database) // Default database
		assert.Equal(t, "admin", cfg.Username)
		assert.Equal(t, "secret", cfg.Password)
	})

	t.Run("build valid mysql config", func(t *testing.T) {
		cfg, err := NewConfigBuilder().
			WithEngine("mysql").
			WithHost("db.example.com").
			WithCredentials("root", "password").
			Build()

		require.NoError(t, err)
		assert.Equal(t, "mysql", cfg.Engine)
		assert.Equal(t, int32(3306), cfg.Port) // Default port
		assert.Equal(t, "mysql", cfg.Database) // Default database
	})

	t.Run("build with custom port and database", func(t *testing.T) {
		cfg, err := NewConfigBuilder().
			WithEngine("postgres").
			WithHost("localhost").
			WithPort(5433).
			WithDatabase("mydb").
			WithCredentials("user", "pass").
			Build()

		require.NoError(t, err)
		assert.Equal(t, int32(5433), cfg.Port)
		assert.Equal(t, "mydb", cfg.Database)
	})

	t.Run("build with SSL", func(t *testing.T) {
		ca := []byte("ca-cert")
		cert := []byte("client-cert")
		key := []byte("client-key")

		cfg, err := NewConfigBuilder().
			WithEngine("postgres").
			WithHost("secure.db.com").
			WithCredentials("user", "pass").
			WithSSL("verify-full", ca, cert, key).
			Build()

		require.NoError(t, err)
		assert.True(t, cfg.TLSEnabled)
		assert.Equal(t, "verify-full", cfg.SSLMode)
		assert.Equal(t, ca, cfg.TLSCA)
		assert.Equal(t, cert, cfg.TLSCert)
		assert.Equal(t, key, cfg.TLSKey)
	})

	t.Run("build with custom timeouts", func(t *testing.T) {
		customTimeouts := util.TimeoutConfig{
			ConnectTimeout:       10 * time.Second,
			OperationTimeout:     20 * time.Second,
			QueryTimeout:         5 * time.Second,
			LongOperationTimeout: 5 * time.Minute,
		}

		cfg, err := NewConfigBuilder().
			WithEngine("postgres").
			WithHost("localhost").
			WithCredentials("user", "pass").
			WithTimeouts(customTimeouts).
			Build()

		require.NoError(t, err)
		assert.Equal(t, customTimeouts.ConnectTimeout, cfg.Timeouts.ConnectTimeout)
		assert.Equal(t, customTimeouts.OperationTimeout, cfg.Timeouts.OperationTimeout)
	})

	t.Run("build with fast timeouts", func(t *testing.T) {
		cfg, err := NewConfigBuilder().
			WithEngine("postgres").
			WithHost("localhost").
			WithCredentials("user", "pass").
			WithFastTimeouts().
			Build()

		require.NoError(t, err)
		assert.Equal(t, util.FastTimeoutConfig().ConnectTimeout, cfg.Timeouts.ConnectTimeout)
	})

	t.Run("build with no timeouts", func(t *testing.T) {
		cfg, err := NewConfigBuilder().
			WithEngine("postgres").
			WithHost("localhost").
			WithCredentials("user", "pass").
			WithNoTimeouts().
			Build()

		require.NoError(t, err)
		assert.Equal(t, util.NoTimeoutConfig().ConnectTimeout, cfg.Timeouts.ConnectTimeout)
	})

	t.Run("build with postgres options", func(t *testing.T) {
		cfg, err := NewConfigBuilder().
			WithEngine("postgres").
			WithHost("localhost").
			WithCredentials("user", "pass").
			WithPostgresOptions(30, "30s", "myapp").
			Build()

		require.NoError(t, err)
		assert.Equal(t, int32(30), cfg.ConnectTimeout)
		assert.Equal(t, "30s", cfg.StatementTimeout)
		assert.Equal(t, "myapp", cfg.ApplicationName)
	})

	t.Run("build with mysql options", func(t *testing.T) {
		cfg, err := NewConfigBuilder().
			WithEngine("mysql").
			WithHost("localhost").
			WithCredentials("user", "pass").
			WithMySQLOptions("utf8mb4", "utf8mb4_unicode_ci", true).
			Build()

		require.NoError(t, err)
		assert.Equal(t, "utf8mb4", cfg.Charset)
		assert.Equal(t, "utf8mb4_unicode_ci", cfg.Collation)
		assert.True(t, cfg.ParseTime)
	})

	t.Run("build fails with missing engine", func(t *testing.T) {
		_, err := NewConfigBuilder().
			WithHost("localhost").
			WithCredentials("user", "pass").
			Build()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "engine is required")
	})

	t.Run("build fails with missing host", func(t *testing.T) {
		_, err := NewConfigBuilder().
			WithEngine("postgres").
			WithCredentials("user", "pass").
			Build()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "host is required")
	})

	t.Run("build fails with missing credentials", func(t *testing.T) {
		_, err := NewConfigBuilder().
			WithEngine("postgres").
			WithHost("localhost").
			Build()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "username is required")
	})

	t.Run("MustBuild succeeds with valid config", func(t *testing.T) {
		cfg := NewConfigBuilder().
			WithEngine("postgres").
			WithHost("localhost").
			WithCredentials("user", "pass").
			MustBuild()

		assert.NotNil(t, cfg)
		assert.Equal(t, "postgres", cfg.Engine)
	})

	t.Run("MustBuild panics with invalid config", func(t *testing.T) {
		assert.Panics(t, func() {
			NewConfigBuilder().
				WithHost("localhost"). // Missing engine
				MustBuild()
		})
	})
}

func TestConfigFromEnv(t *testing.T) {
	t.Run("valid postgres config", func(t *testing.T) {
		env := map[string]string{
			"DBCTL_ENGINE":   "postgres",
			"DBCTL_HOST":     "localhost",
			"DBCTL_PORT":     "5432",
			"DBCTL_USERNAME": "admin",
			"DBCTL_PASSWORD": "secret",
		}
		getEnv := func(key string) string { return env[key] }

		cfg, err := ConfigFromEnv(getEnv)
		require.NoError(t, err)
		assert.Equal(t, "postgres", cfg.Engine)
		assert.Equal(t, "localhost", cfg.Host)
		assert.Equal(t, int32(5432), cfg.Port)
		assert.Equal(t, "admin", cfg.Username)
		assert.Equal(t, "secret", cfg.Password)
		assert.Equal(t, "postgres", cfg.Database) // Default
	})

	t.Run("default port for postgres", func(t *testing.T) {
		env := map[string]string{
			"DBCTL_ENGINE":   "postgres",
			"DBCTL_HOST":     "localhost",
			"DBCTL_USERNAME": "admin",
		}
		getEnv := func(key string) string { return env[key] }

		cfg, err := ConfigFromEnv(getEnv)
		require.NoError(t, err)
		assert.Equal(t, int32(5432), cfg.Port)
	})

	t.Run("default port for mysql", func(t *testing.T) {
		env := map[string]string{
			"DBCTL_ENGINE":   "mysql",
			"DBCTL_HOST":     "localhost",
			"DBCTL_USERNAME": "admin",
		}
		getEnv := func(key string) string { return env[key] }

		cfg, err := ConfigFromEnv(getEnv)
		require.NoError(t, err)
		assert.Equal(t, int32(3306), cfg.Port)
	})

	t.Run("custom timeouts", func(t *testing.T) {
		env := map[string]string{
			"DBCTL_ENGINE":                 "postgres",
			"DBCTL_HOST":                   "localhost",
			"DBCTL_USERNAME":               "admin",
			"DBCTL_CONNECT_TIMEOUT":        "10s",
			"DBCTL_OPERATION_TIMEOUT":      "30s",
			"DBCTL_QUERY_TIMEOUT":          "15s",
			"DBCTL_LONG_OPERATION_TIMEOUT": "5m",
		}
		getEnv := func(key string) string { return env[key] }

		cfg, err := ConfigFromEnv(getEnv)
		require.NoError(t, err)
		assert.Equal(t, 10*time.Second, cfg.Timeouts.ConnectTimeout)
		assert.Equal(t, 30*time.Second, cfg.Timeouts.OperationTimeout)
		assert.Equal(t, 15*time.Second, cfg.Timeouts.QueryTimeout)
		assert.Equal(t, 5*time.Minute, cfg.Timeouts.LongOperationTimeout)
	})

	t.Run("invalid port", func(t *testing.T) {
		env := map[string]string{
			"DBCTL_ENGINE":   "postgres",
			"DBCTL_HOST":     "localhost",
			"DBCTL_PORT":     "invalid",
			"DBCTL_USERNAME": "admin",
		}
		getEnv := func(key string) string { return env[key] }

		_, err := ConfigFromEnv(getEnv)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "DBCTL_PORT")
	})

	t.Run("invalid timeout format", func(t *testing.T) {
		env := map[string]string{
			"DBCTL_ENGINE":          "postgres",
			"DBCTL_HOST":            "localhost",
			"DBCTL_USERNAME":        "admin",
			"DBCTL_CONNECT_TIMEOUT": "invalid",
		}
		getEnv := func(key string) string { return env[key] }

		_, err := ConfigFromEnv(getEnv)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "DBCTL_CONNECT_TIMEOUT")
	})

	t.Run("missing engine", func(t *testing.T) {
		env := map[string]string{
			"DBCTL_HOST":     "localhost",
			"DBCTL_USERNAME": "admin",
		}
		getEnv := func(key string) string { return env[key] }

		_, err := ConfigFromEnv(getEnv)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "DBCTL_ENGINE")
	})

	t.Run("missing host", func(t *testing.T) {
		env := map[string]string{
			"DBCTL_ENGINE":   "postgres",
			"DBCTL_USERNAME": "admin",
		}
		getEnv := func(key string) string { return env[key] }

		_, err := ConfigFromEnv(getEnv)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "DBCTL_HOST")
	})

	t.Run("missing username", func(t *testing.T) {
		env := map[string]string{
			"DBCTL_ENGINE": "postgres",
			"DBCTL_HOST":   "localhost",
		}
		getEnv := func(key string) string { return env[key] }

		_, err := ConfigFromEnv(getEnv)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "DBCTL_USERNAME")
	})
}

func TestConfigFromInstance(t *testing.T) {
	t.Run("basic postgres instance", func(t *testing.T) {
		spec := &dbopsv1alpha1.DatabaseInstanceSpec{
			Engine: dbopsv1alpha1.EngineTypePostgres,
			Connection: dbopsv1alpha1.ConnectionConfig{
				Host: "db.example.com",
				Port: 5432,
			},
		}

		cfg := ConfigFromInstance(spec, "admin", "secret", nil, nil, nil)
		assert.Equal(t, "postgres", cfg.Engine)
		assert.Equal(t, "db.example.com", cfg.Host)
		assert.Equal(t, int32(5432), cfg.Port)
		assert.Equal(t, "postgres", cfg.Database) // Default
		assert.Equal(t, "admin", cfg.Username)
		assert.Equal(t, "secret", cfg.Password)
		assert.NotNil(t, cfg.Timeouts) // Should have default timeouts
	})

	t.Run("mysql instance with defaults", func(t *testing.T) {
		spec := &dbopsv1alpha1.DatabaseInstanceSpec{
			Engine: dbopsv1alpha1.EngineTypeMySQL,
			Connection: dbopsv1alpha1.ConnectionConfig{
				Host: "mysql.example.com",
			},
		}

		cfg := ConfigFromInstance(spec, "root", "pass", nil, nil, nil)
		assert.Equal(t, "mysql", cfg.Engine)
		assert.Equal(t, int32(3306), cfg.Port) // Default port
		assert.Equal(t, "mysql", cfg.Database) // Default database
	})

	t.Run("with TLS credentials", func(t *testing.T) {
		spec := &dbopsv1alpha1.DatabaseInstanceSpec{
			Engine: dbopsv1alpha1.EngineTypePostgres,
			Connection: dbopsv1alpha1.ConnectionConfig{
				Host: "secure.db.com",
				Port: 5432,
			},
		}

		ca := []byte("ca-cert")
		cert := []byte("client-cert")
		key := []byte("client-key")

		cfg := ConfigFromInstance(spec, "user", "pass", ca, cert, key)
		assert.True(t, cfg.TLSEnabled)
		assert.Equal(t, ca, cfg.TLSCA)
		assert.Equal(t, cert, cfg.TLSCert)
		assert.Equal(t, key, cfg.TLSKey)
	})

	t.Run("with postgres options", func(t *testing.T) {
		spec := &dbopsv1alpha1.DatabaseInstanceSpec{
			Engine: dbopsv1alpha1.EngineTypePostgres,
			Connection: dbopsv1alpha1.ConnectionConfig{
				Host: "db.example.com",
				Port: 5432,
			},
			Postgres: &dbopsv1alpha1.PostgresInstanceConfig{
				SSLMode:          dbopsv1alpha1.PostgresSSLModeRequire,
				ConnectTimeout:   30,
				StatementTimeout: "30s",
				ApplicationName:  "myapp",
			},
		}

		cfg := ConfigFromInstance(spec, "user", "pass", nil, nil, nil)
		assert.Equal(t, "require", cfg.SSLMode)
		assert.True(t, cfg.TLSEnabled)
		assert.Equal(t, int32(30), cfg.ConnectTimeout)
		assert.Equal(t, "30s", cfg.StatementTimeout)
		assert.Equal(t, "myapp", cfg.ApplicationName)
	})

	t.Run("with mysql options", func(t *testing.T) {
		spec := &dbopsv1alpha1.DatabaseInstanceSpec{
			Engine: dbopsv1alpha1.EngineTypeMySQL,
			Connection: dbopsv1alpha1.ConnectionConfig{
				Host: "db.example.com",
				Port: 3306,
			},
			MySQL: &dbopsv1alpha1.MySQLInstanceConfig{
				Charset:   "utf8mb4",
				Collation: "utf8mb4_unicode_ci",
				ParseTime: true,
			},
		}

		cfg := ConfigFromInstance(spec, "user", "pass", nil, nil, nil)
		assert.Equal(t, "utf8mb4", cfg.Charset)
		assert.Equal(t, "utf8mb4_unicode_ci", cfg.Collation)
		assert.True(t, cfg.ParseTime)
	})
}

func TestResult(t *testing.T) {
	t.Run("NewSuccessResult", func(t *testing.T) {
		result := NewSuccessResult("Operation completed")
		assert.True(t, result.Success)
		assert.Equal(t, "Operation completed", result.Message)
		assert.False(t, result.Created)
		assert.False(t, result.Updated)
	})

	t.Run("NewCreatedResult", func(t *testing.T) {
		result := NewCreatedResult("Resource created")
		assert.True(t, result.Success)
		assert.Equal(t, "Resource created", result.Message)
		assert.True(t, result.Created)
		assert.False(t, result.Updated)
	})

	t.Run("NewUpdatedResult", func(t *testing.T) {
		result := NewUpdatedResult("Resource updated")
		assert.True(t, result.Success)
		assert.Equal(t, "Resource updated", result.Message)
		assert.False(t, result.Created)
		assert.True(t, result.Updated)
	})

	t.Run("NewExistsResult", func(t *testing.T) {
		result := NewExistsResult("Resource already exists")
		assert.True(t, result.Success)
		assert.Equal(t, "Resource already exists", result.Message)
		assert.False(t, result.Created)
		assert.False(t, result.Updated)
	})

	t.Run("WithData", func(t *testing.T) {
		data := map[string]string{"key": "value"}
		result := NewSuccessResult("Success").WithData(data)
		assert.Equal(t, data, result.Data)
	})
}
