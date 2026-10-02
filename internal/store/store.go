// Package store implements PostgreSQL access using pgx with bound parameters only.
package store

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Product is a row of the products table.
type Product struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	PriceCents int       `json:"price_cents"`
	CreatedAt  time.Time `json:"created_at"`
}

// User is a row of the users table.
type User struct {
	ID           int64
	Username     string
	PasswordHash string
}

// ErrNotFound is returned when a user does not exist.
var ErrNotFound = errors.New("not found")

// Store wraps a pgx pool.
type Store struct{ Pool *pgxpool.Pool }

// Connect opens a pool and waits for the database, retrying with backoff up to wait.
func Connect(ctx context.Context, url string, wait time.Duration) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	deadline := time.Now().Add(wait)
	delay := 500 * time.Millisecond
	for {
		pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err = pool.Ping(pctx)
		cancel()
		if err == nil {
			return &Store{Pool: pool}, nil
		}
		slog.Warn("database not ready", "err", err)
		if time.Now().Add(delay).After(deadline) || ctx.Err() != nil {
			pool.Close()
			return nil, fmt.Errorf("database not reachable: %w", err)
		}
		select {
		case <-ctx.Done():
			pool.Close()
			return nil, ctx.Err()
		case <-time.After(delay):
		}
		if delay < 4*time.Second {
			delay *= 2
		}
	}
}

// Close releases the pool.
func (s *Store) Close() { s.Pool.Close() }

// Ping checks reachability within 2s.
func (s *Store) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return s.Pool.Ping(ctx)
}

// migrationLockID is the pg_advisory_lock key serializing concurrent migrators.
const migrationLockID int64 = 726150001

// Migrate applies the *.sql files of fsys in filename order, each in its own
// transaction, tracked in schema_migrations. Safe to run repeatedly and concurrently.
func (s *Store) Migrate(ctx context.Context, fsys fs.FS) error {
	conn, err := s.Pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationLockID); err != nil {
		return fmt.Errorf("advisory lock: %w", err)
	}
	defer func() {
		// Use a fresh context so the unlock happens even if ctx was cancelled.
		uctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.Exec(uctx, `SELECT pg_advisory_unlock($1)`, migrationLockID)
	}()

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	names, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		var done bool
		if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, name).Scan(&done); err != nil {
			return fmt.Errorf("check %s: %w", name, err)
		}
		if done {
			continue
		}
		sqlText, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(sqlText)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, name); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record %s: %w", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit %s: %w", name, err)
		}
	}
	return nil
}

// SeedAdmin inserts the admin user with the given bcrypt hash if absent.
func (s *Store) SeedAdmin(ctx context.Context, username, passwordHash string) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO users (username, password_hash) VALUES ($1, $2) ON CONFLICT (username) DO NOTHING`,
		username, passwordHash)
	return err
}

// UserByUsername returns ErrNotFound when the user does not exist.
func (s *Store) UserByUsername(ctx context.Context, username string) (User, error) {
	var u User
	err := s.Pool.QueryRow(ctx,
		`SELECT id, username, password_hash FROM users WHERE username = $1`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

// ListProducts returns products ordered by id.
func (s *Store) ListProducts(ctx context.Context, limit, offset int) ([]Product, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id, name, price_cents, created_at FROM products ORDER BY id LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Product{}
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.Name, &p.PriceCents, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
