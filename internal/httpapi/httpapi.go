// Package httpapi wires the HTTP router, login and health handlers.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/example/demo-golang-api-01/internal/auth"
	"github.com/example/demo-golang-api-01/internal/products"
	"github.com/example/demo-golang-api-01/internal/store"
)

// MaxBodyBytes caps request bodies.
const MaxBodyBytes = 1024

// Backend is what the API needs from the data layer.
type Backend interface {
	UserByUsername(ctx context.Context, username string) (store.User, error)
	ListProducts(ctx context.Context, limit, offset int) ([]store.Product, error)
	Ping(ctx context.Context) error
}

// NewRouter builds the API routes.
func NewRouter(b Backend, tokens auth.Tokens) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := b.Ping(r.Context()); err != nil {
			slog.Error("healthz", "err", err)
			WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.Handle("POST /api/v1/login", loginHandler(b, tokens))
	mux.Handle("GET /api/v1/products", tokens.Middleware(products.Handler(b)))
	return mux
}

// WriteJSON writes v as JSON with the status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func loginHandler(b Backend, tokens auth.Tokens) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		var req loginRequest
		if err := dec.Decode(&req); err != nil || req.Username == "" || req.Password == "" {
			WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		// Reject trailing data after the JSON value.
		if _, err := dec.Token(); !errors.Is(err, io.EOF) {
			WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		u, err := b.UserByUsername(r.Context(), req.Username)
		hash := ""
		switch {
		case err == nil:
			hash = u.PasswordHash
		case errors.Is(err, store.ErrNotFound):
		default:
			slog.Error("login lookup", "err", err)
			WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		if !auth.CheckPassword(hash, req.Password) {
			WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
			return
		}
		tok, err := tokens.Issue(u.ID, u.Username)
		if err != nil {
			slog.Error("issue token", "err", err)
			WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"token": tok, "expires_in": int(tokens.TTL.Seconds())})
	})
}
