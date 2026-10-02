// Package products serves the products listing endpoint.
package products

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/example/demo-golang-api-01/internal/store"
)

// Lister reads products.
type Lister interface {
	ListProducts(ctx context.Context, limit, offset int) ([]store.Product, error)
}

const (
	defaultLimit = 20
	maxLimit     = 100
)

// Handler returns the GET /api/v1/products handler.
func Handler(l Lister) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit, offset := defaultLimit, 0
		q := r.URL.Query()
		if v := q.Get("limit"); q.Has("limit") {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > maxLimit {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be an integer between 1 and 100"})
				return
			}
			limit = n
		}
		if v := q.Get("offset"); q.Has("offset") {
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "offset must be an integer >= 0"})
				return
			}
			offset = n
		}
		items, err := l.ListProducts(r.Context(), limit, offset)
		if err != nil {
			slog.Error("list products", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		if items == nil {
			items = []store.Product{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
