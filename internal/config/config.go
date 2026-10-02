// Package config loads runtime configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// MinSecretBytes is the minimum JWT_SECRET length.
const MinSecretBytes = 32

// Config is the validated runtime configuration.
type Config struct {
	HTTPAddr      string
	DatabaseURL   string
	JWTSecret     []byte
	JWTTTL        time.Duration
	AdminUsername string
	AdminPassword string
}

// Load reads configuration using getenv (os.Getenv in production).
// It fails fast, naming each missing or invalid variable.
func Load(getenv func(string) string) (Config, error) {
	var errs []error
	get := func(k, def string) string {
		if v := strings.TrimSpace(getenv(k)); v != "" {
			return v
		}
		return def
	}
	need := func(k string) string {
		v := getenv(k)
		if v == "" {
			errs = append(errs, fmt.Errorf("%s is required", k))
		}
		return v
	}

	c := Config{
		HTTPAddr:      get("HTTP_ADDR", ":8080"),
		DatabaseURL:   need("DATABASE_URL"),
		AdminUsername: get("SEED_ADMIN_USERNAME", "admin"),
		AdminPassword: need("SEED_ADMIN_PASSWORD"),
		JWTTTL:        time.Hour,
	}
	secret := need("JWT_SECRET")
	if secret != "" && len(secret) < MinSecretBytes {
		errs = append(errs, fmt.Errorf("JWT_SECRET must be at least %d bytes", MinSecretBytes))
	}
	c.JWTSecret = []byte(secret)
	if v := get("JWT_TTL", ""); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			errs = append(errs, errors.New("JWT_TTL must be a positive duration such as 1h"))
		} else {
			c.JWTTTL = d
		}
	}
	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return c, nil
}

// FromEnv is Load(os.Getenv).
func FromEnv() (Config, error) { return Load(os.Getenv) }
