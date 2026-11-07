package database

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver used by the migrator

	"github.com/iriscript/url-shortener/migrations"
)

func Migrate(dsn string) error {
	if dsn == "" {
		return ErrNotConfigured
	}

	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("failed to read migrations: %w", err)
	}
	defer func() { _ = source.Close() }()

	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("failed to open migration connection: %w", err)
	}
	defer func() { _ = sqlDB.Close() }()

	driver, err := migratepgx.WithInstance(sqlDB, &migratepgx.Config{})
	if err != nil {
		return fmt.Errorf("failed to init migration driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "pgx5", driver)
	if err != nil {
		return fmt.Errorf("failed to init migrator: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	return nil
}
