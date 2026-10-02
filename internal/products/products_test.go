package products

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/demo-golang-api-01/internal/store"
)

type fake struct {
	limit, offset int
	items         []store.Product
	err           error
	called        bool
}

func (f *fake) ListProducts(_ context.Context, l, o int) ([]store.Product, error) {
	f.called, f.limit, f.offset = true, l, o
	return f.items, f.err
}

func TestHandler(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		want       int
		wantLimit  int
		wantOffset int
	}{
		{"defaults", "", 200, 20, 0},
		{"explicit", "?limit=5&offset=10", 200, 5, 10},
		{"limit min boundary", "?limit=1", 200, 1, 0},
		{"limit max boundary", "?limit=100", 200, 100, 0},
		{"limit over max", "?limit=101", 400, 0, 0},
		{"limit zero", "?limit=0", 400, 0, 0},
		{"limit negative", "?limit=-1", 400, 0, 0},
		{"limit non-int", "?limit=abc", 400, 0, 0},
		{"limit empty", "?limit=", 400, 0, 0},
		{"offset negative", "?offset=-1", 400, 0, 0},
		{"offset non-int", "?offset=x", 400, 0, 0},
		{"offset zero", "?offset=0", 200, 20, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fake{items: []store.Product{{ID: 1, Name: "a", PriceCents: 1}}}
			rec := httptest.NewRecorder()
			Handler(f).ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/products"+tt.query, nil))
			if rec.Code != tt.want {
				t.Fatalf("code = %d, want %d (%s)", rec.Code, tt.want, rec.Body)
			}
			if tt.want == 400 && f.called {
				t.Fatal("store called on invalid input")
			}
			if tt.want == 200 && (f.limit != tt.wantLimit || f.offset != tt.wantOffset) {
				t.Fatalf("limit/offset = %d/%d", f.limit, f.offset)
			}
		})
	}
}

func TestEmptyListIsArray(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler(&fake{}).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if got := strings.TrimSpace(rec.Body.String()); got != `{"items":[]}` {
		t.Fatalf("body = %s", got)
	}
}

func TestStoreErrorIsGeneric500(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler(&fake{err: errors.New("pq: secret internals")}).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
}
