package storage

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

var ErrNotFound = errors.New("storage: not found")

type Storage struct {
	pool *pgxpool.Pool
}

// New создаёт пул соединений и сразу проверяет, что БД доступна (Ping).
func New(ctx context.Context, dsn string) (*Storage, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("storage: create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("storage: ping db: %w", err)
	}

	return &Storage{pool: pool}, nil
}

func (s *Storage) Close() {
	s.pool.Close()
}

// RunMigrations прогоняет все .up.sql миграции, которых ещё нет в БД.
func RunMigrations(dsn string) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("storage: init migrations source: %w", err)
	}

	migrationDSN := "pgx5://" + strings.TrimPrefix(dsn, "postgres://")

	m, err := migrate.NewWithSourceInstance("iofs", src, migrationDSN)
	if err != nil {
		return fmt.Errorf("storage: init migrator: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("storage: apply migrations: %w", err)
	}
	return nil
}
