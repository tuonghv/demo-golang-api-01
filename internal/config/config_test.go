package config

import (
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

const good = "0123456789abcdef0123456789abcdef"

func TestLoad(t *testing.T) {
	base := func() map[string]string {
		return map[string]string{"DATABASE_URL": "postgres://x", "JWT_SECRET": good, "SEED_ADMIN_PASSWORD": "pw"}
	}
	tests := []struct {
		name    string
		mut     func(map[string]string)
		wantErr string
	}{
		{"ok defaults", func(map[string]string) {}, ""},
		{"missing jwt secret", func(m map[string]string) { delete(m, "JWT_SECRET") }, "JWT_SECRET is required"},
		{"short jwt secret", func(m map[string]string) { m["JWT_SECRET"] = "short" }, "at least 32"},
		{"boundary 31 bytes", func(m map[string]string) { m["JWT_SECRET"] = good[:31] }, "at least 32"},
		{"boundary 32 bytes", func(m map[string]string) { m["JWT_SECRET"] = good[:32] }, ""},
		{"missing admin pw", func(m map[string]string) { delete(m, "SEED_ADMIN_PASSWORD") }, "SEED_ADMIN_PASSWORD is required"},
		{"missing db url", func(m map[string]string) { delete(m, "DATABASE_URL") }, "DATABASE_URL is required"},
		{"bad ttl", func(m map[string]string) { m["JWT_TTL"] = "abc" }, "JWT_TTL"},
		{"zero ttl", func(m map[string]string) { m["JWT_TTL"] = "0s" }, "JWT_TTL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := base()
			tt.mut(m)
			_, err := Load(env(m))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want contains %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadDefaults(t *testing.T) {
	c, err := Load(env(map[string]string{"DATABASE_URL": "u", "JWT_SECRET": good, "SEED_ADMIN_PASSWORD": "p"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.HTTPAddr != ":8080" || c.AdminUsername != "admin" || c.JWTTTL != time.Hour {
		t.Fatalf("defaults wrong: %+v", c)
	}
}
