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

package testutil

import (
	"context"
	"fmt"
	"sync"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mariadb"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// DatabaseContainer wraps testcontainers for different database types
type DatabaseContainer struct {
	container testcontainers.Container
	engine    string
	host      string
	port      int
	user      string
	password  string
	database  string
	mu        sync.Mutex
}

// DatabaseContainerConfig holds configuration for starting a database container
type DatabaseContainerConfig struct {
	Engine   string // "postgresql", "mysql", "mariadb"
	Image    string // optional, uses default if empty
	User     string
	Password string
	Database string
}

// StartDatabaseContainer starts a database container based on engine type
func StartDatabaseContainer(ctx context.Context, cfg DatabaseContainerConfig) (*DatabaseContainer, error) {
	dc := &DatabaseContainer{
		engine:   cfg.Engine,
		user:     cfg.User,
		password: cfg.Password,
		database: cfg.Database,
	}

	switch cfg.Engine {
	case "postgresql", "postgres":
		return dc.startPostgres(ctx, cfg)
	case "mysql":
		return dc.startMySQL(ctx, cfg)
	case "mariadb":
		return dc.startMariaDB(ctx, cfg)
	default:
		return nil, fmt.Errorf("unsupported engine: %s", cfg.Engine)
	}
}

func (dc *DatabaseContainer) startPostgres(ctx context.Context, cfg DatabaseContainerConfig) (*DatabaseContainer, error) {
	image := cfg.Image
	if image == "" {
		image = "postgres:16-alpine"
	}

	container, err := postgres.Run(ctx,
		image,
		postgres.WithUsername(cfg.User),
		postgres.WithPassword(cfg.Password),
		postgres.WithDatabase(cfg.Database),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start postgres: %w", err)
	}

	dc.container = container
	host, err := container.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get postgres host: %w", err)
	}
	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		return nil, fmt.Errorf("failed to get postgres port: %w", err)
	}

	dc.host = host
	dc.port = port.Int()
	return dc, nil
}

func (dc *DatabaseContainer) startMySQL(ctx context.Context, cfg DatabaseContainerConfig) (*DatabaseContainer, error) {
	image := cfg.Image
	if image == "" {
		image = "mysql:8.0"
	}

	container, err := mysql.Run(ctx,
		image,
		mysql.WithUsername(cfg.User),
		mysql.WithPassword(cfg.Password),
		mysql.WithDatabase(cfg.Database),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start mysql: %w", err)
	}

	dc.container = container
	host, err := container.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get mysql host: %w", err)
	}
	port, err := container.MappedPort(ctx, "3306/tcp")
	if err != nil {
		return nil, fmt.Errorf("failed to get mysql port: %w", err)
	}

	dc.host = host
	dc.port = port.Int()
	return dc, nil
}

func (dc *DatabaseContainer) startMariaDB(ctx context.Context, cfg DatabaseContainerConfig) (*DatabaseContainer, error) {
	image := cfg.Image
	if image == "" {
		image = "mariadb:11.2"
	}

	container, err := mariadb.Run(ctx,
		image,
		mariadb.WithUsername(cfg.User),
		mariadb.WithPassword(cfg.Password),
		mariadb.WithDatabase(cfg.Database),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start mariadb: %w", err)
	}

	dc.container = container
	host, err := container.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get mariadb host: %w", err)
	}
	port, err := container.MappedPort(ctx, "3306/tcp")
	if err != nil {
		return nil, fmt.Errorf("failed to get mariadb port: %w", err)
	}

	dc.host = host
	dc.port = port.Int()
	return dc, nil
}

// Stop terminates the container
func (dc *DatabaseContainer) Stop(ctx context.Context) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	if dc.container != nil {
		return dc.container.Terminate(ctx)
	}
	return nil
}

// ConnectionInfo returns connection details
func (dc *DatabaseContainer) ConnectionInfo() (host string, port int, user, password, database string) {
	return dc.host, dc.port, dc.user, dc.password, dc.database
}

// Host returns the container host
func (dc *DatabaseContainer) Host() string {
	return dc.host
}

// Port returns the mapped container port
func (dc *DatabaseContainer) Port() int {
	return dc.port
}

// Engine returns the database engine type
func (dc *DatabaseContainer) Engine() string {
	return dc.engine
}
