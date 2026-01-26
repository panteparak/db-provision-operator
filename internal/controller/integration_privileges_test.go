//go:build integration

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

package controller

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// These tests verify that the least-privilege admin account has the correct
// permissions and is NOT a superuser.
var _ = Describe("Least-Privilege Admin Tests", func() {
	var db *sql.DB

	BeforeEach(func() {
		var err error
		if intDBConfig.Database == "postgresql" || intDBConfig.Database == "postgres" {
			dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
				intDBConfig.Host, intDBConfig.Port, intDBConfig.User, intDBConfig.Password, intDBConfig.DBName)
			db, err = sql.Open("pgx", dsn)
		} else {
			dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
				intDBConfig.User, intDBConfig.Password, intDBConfig.Host, intDBConfig.Port, intDBConfig.DBName)
			db, err = sql.Open("mysql", dsn)
		}
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Context("PostgreSQL Privileges", func() {
		BeforeEach(func() {
			if intDBConfig.Database != "postgresql" && intDBConfig.Database != "postgres" {
				Skip("PostgreSQL-specific test")
			}
		})

		It("should NOT be a superuser", func() {
			var isSuperuser bool
			err := db.QueryRow("SELECT rolsuper FROM pg_roles WHERE rolname = current_user").Scan(&isSuperuser)
			Expect(err).NotTo(HaveOccurred())
			Expect(isSuperuser).To(BeFalse(), "Admin account should NOT be a superuser")
		})

		It("should have CREATEDB privilege", func() {
			var canCreateDB bool
			err := db.QueryRow("SELECT rolcreatedb FROM pg_roles WHERE rolname = current_user").Scan(&canCreateDB)
			Expect(err).NotTo(HaveOccurred())
			Expect(canCreateDB).To(BeTrue(), "Admin account should have CREATEDB privilege")
		})

		It("should have CREATEROLE privilege", func() {
			var canCreateRole bool
			err := db.QueryRow("SELECT rolcreaterole FROM pg_roles WHERE rolname = current_user").Scan(&canCreateRole)
			Expect(err).NotTo(HaveOccurred())
			Expect(canCreateRole).To(BeTrue(), "Admin account should have CREATEROLE privilege")
		})

		It("should be a member of pg_signal_backend", func() {
			var hasRole bool
			err := db.QueryRow("SELECT pg_has_role(current_user, 'pg_signal_backend', 'MEMBER')").Scan(&hasRole)
			Expect(err).NotTo(HaveOccurred())
			Expect(hasRole).To(BeTrue(), "Admin account should be a member of pg_signal_backend")
		})

		It("should be able to create a database", func() {
			testDBName := "privilege_test_create_db"

			// Create database
			_, err := db.Exec(fmt.Sprintf("CREATE DATABASE %s", testDBName))
			Expect(err).NotTo(HaveOccurred(), "Admin should be able to create database")

			// Cleanup
			_, err = db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", testDBName))
			Expect(err).NotTo(HaveOccurred())
		})

		It("should be able to create a role/user", func() {
			testRoleName := "privilege_test_role"

			// Create role
			_, err := db.Exec(fmt.Sprintf("CREATE ROLE %s WITH LOGIN PASSWORD 'test123'", testRoleName))
			Expect(err).NotTo(HaveOccurred(), "Admin should be able to create role")

			// Cleanup
			_, err = db.Exec(fmt.Sprintf("DROP ROLE IF EXISTS %s", testRoleName))
			Expect(err).NotTo(HaveOccurred())
		})

		It("should be able to grant privileges on created objects", func() {
			testRoleName := "privilege_test_grant_role"

			// Create a test role
			_, err := db.Exec(fmt.Sprintf("CREATE ROLE %s WITH LOGIN PASSWORD 'test123'", testRoleName))
			Expect(err).NotTo(HaveOccurred())

			// Grant CONNECT privilege
			_, err = db.Exec(fmt.Sprintf("GRANT CONNECT ON DATABASE %s TO %s", intDBConfig.DBName, testRoleName))
			Expect(err).NotTo(HaveOccurred(), "Admin should be able to grant privileges")

			// Cleanup
			_, err = db.Exec(fmt.Sprintf("REVOKE CONNECT ON DATABASE %s FROM %s", intDBConfig.DBName, testRoleName))
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(fmt.Sprintf("DROP ROLE IF EXISTS %s", testRoleName))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("MySQL/MariaDB Privileges", func() {
		BeforeEach(func() {
			if intDBConfig.Database != "mysql" && intDBConfig.Database != "mariadb" {
				Skip("MySQL/MariaDB-specific test")
			}
		})

		It("should NOT have ALL PRIVILEGES", func() {
			rows, err := db.Query(fmt.Sprintf("SHOW GRANTS FOR '%s'@'%%'", intDBConfig.User))
			Expect(err).NotTo(HaveOccurred())
			defer rows.Close()

			hasAllPrivileges := false
			for rows.Next() {
				var grant string
				err := rows.Scan(&grant)
				Expect(err).NotTo(HaveOccurred())

				// Check if grant contains "ALL PRIVILEGES"
				if strings.Contains(strings.ToUpper(grant), "ALL PRIVILEGES") {
					hasAllPrivileges = true
					break
				}
			}

			Expect(hasAllPrivileges).To(BeFalse(), "Admin account should NOT have ALL PRIVILEGES")
		})

		It("should have CREATE privilege", func() {
			rows, err := db.Query(fmt.Sprintf("SHOW GRANTS FOR '%s'@'%%'", intDBConfig.User))
			Expect(err).NotTo(HaveOccurred())
			defer rows.Close()

			hasCreate := false
			for rows.Next() {
				var grant string
				err := rows.Scan(&grant)
				Expect(err).NotTo(HaveOccurred())
				if strings.Contains(strings.ToUpper(grant), "CREATE") {
					hasCreate = true
					break
				}
			}

			Expect(hasCreate).To(BeTrue(), "Admin account should have CREATE privilege")
		})

		It("should have CREATE USER privilege", func() {
			rows, err := db.Query(fmt.Sprintf("SHOW GRANTS FOR '%s'@'%%'", intDBConfig.User))
			Expect(err).NotTo(HaveOccurred())
			defer rows.Close()

			hasCreateUser := false
			for rows.Next() {
				var grant string
				err := rows.Scan(&grant)
				Expect(err).NotTo(HaveOccurred())
				if strings.Contains(strings.ToUpper(grant), "CREATE USER") {
					hasCreateUser = true
					break
				}
			}

			Expect(hasCreateUser).To(BeTrue(), "Admin account should have CREATE USER privilege")
		})

		It("should be able to create a database", func() {
			testDBName := "privilege_test_create_db"

			// Create database
			_, err := db.Exec(fmt.Sprintf("CREATE DATABASE %s", testDBName))
			Expect(err).NotTo(HaveOccurred(), "Admin should be able to create database")

			// Cleanup
			_, err = db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", testDBName))
			Expect(err).NotTo(HaveOccurred())
		})

		It("should be able to create a user", func() {
			testUserName := "privilege_test_user"

			// Create user
			_, err := db.Exec(fmt.Sprintf("CREATE USER '%s'@'%%' IDENTIFIED BY 'test123'", testUserName))
			Expect(err).NotTo(HaveOccurred(), "Admin should be able to create user")

			// Cleanup
			_, err = db.Exec(fmt.Sprintf("DROP USER IF EXISTS '%s'@'%%'", testUserName))
			Expect(err).NotTo(HaveOccurred())
		})

		It("should be able to grant privileges", func() {
			testUserName := "privilege_test_grant_user"
			testDBName := "privilege_test_grant_db"

			// Create test database and user
			_, err := db.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", testDBName))
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(fmt.Sprintf("CREATE USER '%s'@'%%' IDENTIFIED BY 'test123'", testUserName))
			Expect(err).NotTo(HaveOccurred())

			// Grant privileges
			_, err = db.Exec(fmt.Sprintf("GRANT SELECT, INSERT ON %s.* TO '%s'@'%%'", testDBName, testUserName))
			Expect(err).NotTo(HaveOccurred(), "Admin should be able to grant privileges")

			// Cleanup
			_, err = db.Exec(fmt.Sprintf("DROP USER IF EXISTS '%s'@'%%'", testUserName))
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", testDBName))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Cross-Database Security", func() {
		It("should verify operator uses least-privilege account", func() {
			// This test confirms the integration test suite is using the admin account
			Expect(intDBConfig.User).To(Equal("dbprovision_admin"),
				"Integration tests should be using the least-privilege admin account")
		})
	})
})
