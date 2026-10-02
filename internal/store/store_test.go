package store

import (
	"context"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/example/demo-golang-api-01/migrations"
)

// DB-backed tests run only when TEST_DATABASE_URL points at a disposable database.
func newStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	st, err := Connect(ctx, url, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	// Start from a clean schema in the disposable database.
	if _, err := st.Pool.Exec(ctx, `DROP TABLE IF EXISTS users, products, schema_migrations CASCADE`); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestMigrateIdempotent(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if err := st.Migrate(ctx, migrations.FS); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
	var n int
	if err := st.Pool.QueryRow(ctx, `SELECT count(*) FROM products`).Scan(&n); err != nil || n < 5 {
		t.Fatalf("products = %d err=%v", n, err)
	}
	if err := st.Pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("schema_migrations = %d err=%v", n, err)
	}
}

func TestMigrateConcurrent(t *testing.T) {
	st := newStore(t)
	errc := make(chan error, 4)
	for i := 0; i < 4; i++ {
		go func() { errc <- st.Migrate(context.Background(), migrations.FS) }()
	}
	for i := 0; i < 4; i++ {
		if err := <-errc; err != nil {
			t.Fatal(err)
		}
	}
}

func TestMigrateFailureRollsBack(t *testing.T) {
	st := newStore(t)
	bad := fstest.MapFS{"001_bad.sql": {Data: []byte(`CREATE TABLE users (id int); SELECT nope_not_a_column FROM users;`)}}
	if err := st.Migrate(context.Background(), bad); err == nil {
		t.Fatal("expected error")
	}
	var exists bool
	if err := st.Pool.QueryRow(context.Background(), `SELECT to_regclass('users') IS NOT NULL`).Scan(&exists); err != nil || exists {
		t.Fatalf("users table leaked after rollback: exists=%v err=%v", exists, err)
	}
}

func TestSeedAdminAndLookup(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx, migrations.FS); err != nil {
		t.Fatal(err)
	}
	const hash = "$2a$10$abcdefghijklmnopqrstuuabcdefghijklmnopqrstuvwxyz012345"
	if err := st.SeedAdmin(ctx, "admin", hash); err != nil {
		t.Fatal(err)
	}
	if err := st.SeedAdmin(ctx, "admin", "$2a$10$other"); err != nil { // DO NOTHING keeps the first
		t.Fatal(err)
	}
	u, err := st.UserByUsername(ctx, "admin")
	if err != nil || u.PasswordHash != hash || !strings.HasPrefix(u.PasswordHash, "$2") {
		t.Fatalf("user = %+v err=%v", u, err)
	}
	if _, err := st.UserByUsername(ctx, "ghost"); err != ErrNotFound {
		t.Fatalf("err = %v", err)
	}
	// Bound parameters: a quote-bearing username is data, not SQL.
	if _, err := st.UserByUsername(ctx, `' OR '1'='1`); err != ErrNotFound {
		t.Fatalf("injection string matched: %v", err)
	}
}

func TestListProductsLimitOffset(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx, migrations.FS); err != nil {
		t.Fatal(err)
	}
	all, err := st.ListProducts(ctx, 100, 0)
	if err != nil || len(all) < 5 {
		t.Fatalf("all = %d err=%v", len(all), err)
	}
	two, _ := st.ListProducts(ctx, 2, 1)
	if len(two) != 2 || two[0].ID != all[1].ID || two[1].ID != all[2].ID {
		t.Fatalf("limit/offset wrong: %+v", two)
	}
	none, err := st.ListProducts(ctx, 5, 1000)
	if err != nil || none == nil || len(none) != 0 {
		t.Fatalf("beyond end: %v %v", none, err)
	}
}
