package db

import (
	"database/sql"
	"errors"
	"fmt"
	"log"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"

	"va_visionai_server/conf"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// MigrateDB handles database migrations
func MigrateDB(db *sql.DB, dbName string) error {
	driver, err := mysql.WithInstance(db, &mysql.Config{
		MigrationsTable: "va_schema_migrations",
	})
	if err != nil {
		return fmt.Errorf("could not create mysql driver: %w", err)
	}
	filePath := "file://" + conf.GlobalConfig.Mysql.MigrationsDir
	m, err := migrate.NewWithDatabaseInstance(
		filePath,
		dbName,
		driver,
	)
	if err != nil {
		return fmt.Errorf("could not create migration instance: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("could not run migrations: %w", err)
	}

	version, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return fmt.Errorf("could not get migration version: %w", err)
	}

	log.Printf("migrations completed. Current version: %d, dirty: %v", version, dirty)
	return nil
}

// RollbackDB rolls back the last migration
func RollbackDB(db *sql.DB, dbName string) error {
	driver, err := mysql.WithInstance(db, &mysql.Config{})
	if err != nil {
		return fmt.Errorf("could not create mysql driver: %w", err)
	}
	filePath := "file://" + conf.GlobalConfig.Mysql.MigrationsDir
	m, err := migrate.NewWithDatabaseInstance(
		filePath,
		"mysql",
		driver,
	)
	if err != nil {
		return fmt.Errorf("could not create migration instance: %w", err)
	}

	if err := m.Steps(-1); err != nil {
		return fmt.Errorf("could not rollback migration: %w", err)
	}

	version, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return fmt.Errorf("could not get migration version: %w", err)
	}

	log.Printf("rollback completed. Current version: %d, dirty: %v", version, dirty)
	return nil
}
