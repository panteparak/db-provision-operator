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

package postgres

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/db-provision-operator/internal/adapter/postgres/testutil"
	"github.com/db-provision-operator/internal/adapter/types"
)

var _ = Describe("PostgreSQL Adapter", func() {
	Describe("NewAdapter", func() {
		It("should create adapter with config", func() {
			config := testutil.NewBasicConnectionConfig()

			adapter := NewAdapter(config)

			Expect(adapter).NotTo(BeNil())
			Expect(adapter.config.Host).To(Equal(testutil.TestHost))
			Expect(adapter.config.Port).To(Equal(int32(testutil.TestPort)))
			Expect(adapter.config.Database).To(Equal(testutil.TestDatabase))
			Expect(adapter.config.Username).To(Equal(testutil.TestUsername))
			Expect(adapter.config.Password).To(Equal(testutil.TestPassword))
			Expect(adapter.pool).To(BeNil())
		})

		It("should create adapter with TLS config", func() {
			config := testutil.NewConnectionConfigWithTLS("verify-full")

			adapter := NewAdapter(config)

			Expect(adapter).NotTo(BeNil())
			Expect(adapter.config.TLSEnabled).To(BeTrue())
			Expect(adapter.config.TLSMode).To(Equal("verify-full"))
		})
	})

	Describe("buildConnectionString", func() {
		It("should build basic connection string", func() {
			config := testutil.NewBasicConnectionConfig()
			adapter := NewAdapter(config)

			connStr, err := adapter.buildConnectionString()

			Expect(err).NotTo(HaveOccurred())
			Expect(connStr).To(ContainSubstring("host=localhost"))
			Expect(connStr).To(ContainSubstring("port=5432"))
			Expect(connStr).To(ContainSubstring("dbname=testdb"))
			Expect(connStr).To(ContainSubstring("user=testuser"))
			Expect(connStr).To(ContainSubstring("password=testpassword123"))
			Expect(connStr).To(ContainSubstring("sslmode=disable"))
		})

		It("should include SSL mode when TLS enabled", func() {
			config := testutil.NewConnectionConfigWithTLS("verify-full")
			adapter := NewAdapter(config)

			connStr, err := adapter.buildConnectionString()

			Expect(err).NotTo(HaveOccurred())
			Expect(connStr).To(ContainSubstring("sslmode=verify-full"))
		})

		It("should prefer SSLMode over TLSMode when both are set", func() {
			config := testutil.NewConnectionConfigWithTLS("verify-full")
			config.SSLMode = "require"
			adapter := NewAdapter(config)

			connStr, err := adapter.buildConnectionString()

			Expect(err).NotTo(HaveOccurred())
			Expect(connStr).To(ContainSubstring("sslmode=require"))
		})

		It("should include application name", func() {
			config := testutil.NewConnectionConfigWithApplicationName(testutil.TestApplicationName)
			adapter := NewAdapter(config)

			connStr, err := adapter.buildConnectionString()

			Expect(err).NotTo(HaveOccurred())
			Expect(connStr).To(ContainSubstring("application_name=db-provision-operator-test"))
		})

		It("should include connect timeout", func() {
			config := testutil.NewConnectionConfigWithTimeout(30)
			adapter := NewAdapter(config)

			connStr, err := adapter.buildConnectionString()

			Expect(err).NotTo(HaveOccurred())
			Expect(connStr).To(ContainSubstring("connect_timeout=30"))
		})

		It("should not include connect timeout when zero", func() {
			config := testutil.NewBasicConnectionConfig()
			adapter := NewAdapter(config)

			connStr, err := adapter.buildConnectionString()

			Expect(err).NotTo(HaveOccurred())
			Expect(connStr).NotTo(ContainSubstring("connect_timeout"))
		})

		It("should not include application name when empty", func() {
			config := testutil.NewBasicConnectionConfig()
			adapter := NewAdapter(config)

			connStr, err := adapter.buildConnectionString()

			Expect(err).NotTo(HaveOccurred())
			Expect(connStr).NotTo(ContainSubstring("application_name"))
		})
	})

	Describe("buildTLSConfig", func() {
		It("should return nil for disable mode", func() {
			config := testutil.NewConnectionConfigWithTLS("disable")
			adapter := NewAdapter(config)

			tlsConfig, err := adapter.buildTLSConfig()

			Expect(err).NotTo(HaveOccurred())
			Expect(tlsConfig).To(BeNil())
		})

		It("should set InsecureSkipVerify for require mode", func() {
			config := testutil.NewConnectionConfigWithTLS("require")
			adapter := NewAdapter(config)

			tlsConfig, err := adapter.buildTLSConfig()

			Expect(err).NotTo(HaveOccurred())
			Expect(tlsConfig).NotTo(BeNil())
			Expect(tlsConfig.InsecureSkipVerify).To(BeTrue())
		})

		It("should set InsecureSkipVerify to false for verify-ca mode", func() {
			config := testutil.NewConnectionConfigWithTLS("verify-ca")
			adapter := NewAdapter(config)

			tlsConfig, err := adapter.buildTLSConfig()

			Expect(err).NotTo(HaveOccurred())
			Expect(tlsConfig).NotTo(BeNil())
			Expect(tlsConfig.InsecureSkipVerify).To(BeFalse())
		})

		It("should set InsecureSkipVerify to false for verify-full mode", func() {
			config := testutil.NewConnectionConfigWithTLS("verify-full")
			adapter := NewAdapter(config)

			tlsConfig, err := adapter.buildTLSConfig()

			Expect(err).NotTo(HaveOccurred())
			Expect(tlsConfig).NotTo(BeNil())
			Expect(tlsConfig.InsecureSkipVerify).To(BeFalse())
			Expect(tlsConfig.ServerName).To(Equal(testutil.TestHost))
		})

		It("should add CA certificate", func() {
			config := testutil.NewConnectionConfigWithTLSCerts(
				"verify-full",
				[]byte(testutil.TestCACert),
				nil,
				nil,
			)
			adapter := NewAdapter(config)

			tlsConfig, err := adapter.buildTLSConfig()

			Expect(err).NotTo(HaveOccurred())
			Expect(tlsConfig).NotTo(BeNil())
			Expect(tlsConfig.RootCAs).NotTo(BeNil())
		})

		It("should add client certificate for mTLS", func() {
			config := testutil.NewConnectionConfigWithTLSCerts(
				"verify-full",
				[]byte(testutil.TestCACert),
				[]byte(testutil.TestClientCert),
				[]byte(testutil.TestClientKey),
			)
			adapter := NewAdapter(config)

			tlsConfig, err := adapter.buildTLSConfig()

			Expect(err).NotTo(HaveOccurred())
			Expect(tlsConfig).NotTo(BeNil())
			Expect(tlsConfig.Certificates).To(HaveLen(1))
		})

		It("should return error for invalid CA", func() {
			config := testutil.NewConnectionConfigWithTLSCerts(
				"verify-full",
				[]byte(testutil.InvalidCACert),
				nil,
				nil,
			)
			adapter := NewAdapter(config)

			tlsConfig, err := adapter.buildTLSConfig()

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse CA certificate"))
			Expect(tlsConfig).To(BeNil())
		})

		It("should return error for invalid client cert", func() {
			config := testutil.NewConnectionConfigWithTLSCerts(
				"verify-full",
				[]byte(testutil.TestCACert),
				[]byte(testutil.InvalidClientCert),
				[]byte(testutil.InvalidClientKey),
			)
			adapter := NewAdapter(config)

			tlsConfig, err := adapter.buildTLSConfig()

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse client certificate"))
			Expect(tlsConfig).To(BeNil())
		})

		It("should set minimum TLS version to 1.2", func() {
			config := testutil.NewConnectionConfigWithTLS("require")
			adapter := NewAdapter(config)

			tlsConfig, err := adapter.buildTLSConfig()

			Expect(err).NotTo(HaveOccurred())
			Expect(tlsConfig).NotTo(BeNil())
			Expect(tlsConfig.MinVersion).To(Equal(uint16(0x0303))) // TLS 1.2
		})

		It("should default to InsecureSkipVerify for unknown mode", func() {
			config := testutil.NewConnectionConfigWithTLS("unknown-mode")
			adapter := NewAdapter(config)

			tlsConfig, err := adapter.buildTLSConfig()

			Expect(err).NotTo(HaveOccurred())
			Expect(tlsConfig).NotTo(BeNil())
			Expect(tlsConfig.InsecureSkipVerify).To(BeTrue())
		})
	})

	Describe("escapeIdentifier", func() {
		It("should quote identifiers", func() {
			result := escapeIdentifier("my_table")

			Expect(result).To(Equal(`"my_table"`))
		})

		It("should double existing quotes", func() {
			result := escapeIdentifier(`my"table`)

			Expect(result).To(Equal(`"my""table"`))
		})

		It("should handle empty string", func() {
			result := escapeIdentifier("")

			Expect(result).To(Equal(`""`))
		})

		It("should handle multiple quotes", func() {
			result := escapeIdentifier(`a"b"c`)

			Expect(result).To(Equal(`"a""b""c"`))
		})

		It("should handle identifier with spaces", func() {
			result := escapeIdentifier("my table")

			Expect(result).To(Equal(`"my table"`))
		})

		It("should handle special characters", func() {
			result := escapeIdentifier("table$name")

			Expect(result).To(Equal(`"table$name"`))
		})
	})

	Describe("escapeLiteral", func() {
		It("should quote literals", func() {
			result := escapeLiteral("my_value")

			Expect(result).To(Equal(`'my_value'`))
		})

		It("should double existing single quotes", func() {
			result := escapeLiteral(`my'value`)

			Expect(result).To(Equal(`'my''value'`))
		})

		It("should handle empty string", func() {
			result := escapeLiteral("")

			Expect(result).To(Equal(`''`))
		})

		It("should handle multiple single quotes", func() {
			result := escapeLiteral(`a'b'c`)

			Expect(result).To(Equal(`'a''b''c'`))
		})

		It("should handle literal with spaces", func() {
			result := escapeLiteral("my value")

			Expect(result).To(Equal(`'my value'`))
		})

		It("should not escape double quotes", func() {
			result := escapeLiteral(`my"value`)

			Expect(result).To(Equal(`'my"value'`))
		})
	})

	Describe("Ping", func() {
		Context("when not connected", func() {
			It("should return error", func() {
				config := testutil.NewBasicConnectionConfig()
				adapter := NewAdapter(config)

				err := adapter.Ping(context.Background())

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("not connected"))
			})
		})
	})

	Describe("GetVersion", func() {
		Context("when not connected", func() {
			It("should return error", func() {
				config := testutil.NewBasicConnectionConfig()
				adapter := NewAdapter(config)

				version, err := adapter.GetVersion(context.Background())

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("not connected"))
				Expect(version).To(BeEmpty())
			})
		})
	})

	Describe("Close", func() {
		It("should be safe to call when not connected", func() {
			config := testutil.NewBasicConnectionConfig()
			adapter := NewAdapter(config)

			err := adapter.Close()

			Expect(err).NotTo(HaveOccurred())
		})

		It("should be idempotent", func() {
			config := testutil.NewBasicConnectionConfig()
			adapter := NewAdapter(config)

			err := adapter.Close()
			Expect(err).NotTo(HaveOccurred())

			err = adapter.Close()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("getPool", func() {
		Context("when not connected", func() {
			It("should return error", func() {
				config := testutil.NewBasicConnectionConfig()
				adapter := NewAdapter(config)

				pool, err := adapter.getPool()

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("not connected"))
				Expect(pool).To(BeNil())
			})
		})
	})

	Describe("Connection Config Variations", func() {
		It("should handle config with all fields set", func() {
			config := types.ConnectionConfig{
				Host:            "db.example.com",
				Port:            5433,
				Database:        "production",
				Username:        "admin",
				Password:        "secret123",
				TLSEnabled:      true,
				TLSMode:         "verify-full",
				TLSCA:           []byte(testutil.TestCACert),
				SSLMode:         "",
				ConnectTimeout:  60,
				ApplicationName: "my-app",
			}
			adapter := NewAdapter(config)

			connStr, err := adapter.buildConnectionString()

			Expect(err).NotTo(HaveOccurred())
			Expect(connStr).To(ContainSubstring("host=db.example.com"))
			Expect(connStr).To(ContainSubstring("port=5433"))
			Expect(connStr).To(ContainSubstring("dbname=production"))
			Expect(connStr).To(ContainSubstring("user=admin"))
			Expect(connStr).To(ContainSubstring("password=secret123"))
			Expect(connStr).To(ContainSubstring("sslmode=verify-full"))
			Expect(connStr).To(ContainSubstring("connect_timeout=60"))
			Expect(connStr).To(ContainSubstring("application_name=my-app"))
		})

		It("should handle config with only required fields", func() {
			config := types.ConnectionConfig{
				Host:     "localhost",
				Port:     5432,
				Database: "test",
				Username: "user",
				Password: "pass",
			}
			adapter := NewAdapter(config)

			connStr, err := adapter.buildConnectionString()

			Expect(err).NotTo(HaveOccurred())
			Expect(connStr).To(ContainSubstring("sslmode=disable"))
			Expect(connStr).NotTo(ContainSubstring("connect_timeout"))
			Expect(connStr).NotTo(ContainSubstring("application_name"))
		})
	})
})
