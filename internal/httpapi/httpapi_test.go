package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/example/demo-golang-api-01/internal/auth"
	"github.com/example/demo-golang-api-01/internal/store"
)

type fake struct {
	users   map[string]store.User
	pingErr error
	listErr error
}

func (f *fake) UserByUsername(_ context.Context, n string) (store.User, error) {
	u, ok := f.users[n]
	if !ok {
		return store.User{}, store.ErrNotFound
	}
	return u, nil
}
func (f *fake) ListProducts(context.Context, int, int) ([]store.Product, error) {
	return []store.Product{{ID: 1, Name: "p", PriceCents: 5}, {ID: 2, Name: "q", PriceCents: 6}}, f.listErr
}
func (f *fake) Ping(context.Context) error { return f.pingErr }

var tokens = auth.Tokens{Secret: []byte("0123456789abcdef0123456789abcdef"), TTL: time.Hour}

func newFake(t *testing.T) *fake {
	h, err := auth.HashPassword("pw-ok")
	if err != nil {
		t.Fatal(err)
	}
	return &fake{users: map[string]store.User{"admin": {ID: 7, Username: "admin", PasswordHash: h}}}
}

func do(h http.Handler, method, path, body, authz string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestLogin(t *testing.T) {
	h := NewRouter(newFake(t), tokens)
	tests := []struct {
		name, body string
		want       int
	}{
		{"valid", `{"username":"admin","password":"pw-ok"}`, 200},
		{"wrong password", `{"username":"admin","password":"bad"}`, 401},
		{"unknown user", `{"username":"ghost","password":"pw-ok"}`, 401},
		{"malformed json", `{bad`, 400},
		{"empty body", ``, 400},
		{"empty username", `{"username":"","password":"x"}`, 400},
		{"empty password", `{"username":"admin","password":""}`, 400},
		{"unknown field", `{"username":"admin","password":"pw-ok","x":1}`, 400},
		{"trailing data", `{"username":"admin","password":"pw-ok"}{}`, 400},
		{"wrong types", `{"username":1,"password":2}`, 400},
		{"oversized body", `{"username":"admin","password":"` + strings.Repeat("a", 2000) + `"}`, 400},
		{"body at limit boundary ok", `{"username":"admin","password":"pw-ok"}` + strings.Repeat(" ", 900), 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(h, "POST", "/api/v1/login", tt.body, "")
			if rec.Code != tt.want {
				t.Fatalf("code = %d, want %d (%s)", rec.Code, tt.want, rec.Body)
			}
			if tt.want == 200 {
				var out struct {
					Token     string `json:"token"`
					ExpiresIn int    `json:"expires_in"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(strings.Split(out.Token, ".")) != 3 || out.ExpiresIn != 3600 {
					t.Fatalf("bad body %s", rec.Body)
				}
				if c, err := tokens.Parse(out.Token); err != nil || c.Subject != "7" {
					t.Fatalf("token claims: %v %v", c, err)
				}
			}
		})
	}
}

func TestLoginIdentical401Bodies(t *testing.T) {
	h := NewRouter(newFake(t), tokens)
	a := do(h, "POST", "/api/v1/login", `{"username":"admin","password":"bad"}`, "")
	b := do(h, "POST", "/api/v1/login", `{"username":"ghost","password":"bad"}`, "")
	if a.Code != 401 || b.Code != 401 || a.Body.String() != b.Body.String() {
		t.Fatalf("a=%d %q b=%d %q", a.Code, a.Body, b.Code, b.Body)
	}
	if strings.TrimSpace(a.Body.String()) != `{"error":"invalid credentials"}` {
		t.Fatalf("body = %s", a.Body)
	}
}

type errBackend struct{ *fake }

func (errBackend) UserByUsername(context.Context, string) (store.User, error) {
	return store.User{}, errors.New("db down: internals")
}

func TestLoginDBErrorIsGeneric500(t *testing.T) {
	rec := do(NewRouter(errBackend{newFake(t)}, tokens), "POST", "/api/v1/login", `{"username":"admin","password":"pw-ok"}`, "")
	if rec.Code != 500 || strings.Contains(rec.Body.String(), "internals") {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
}

func TestProductsRequiresAuth(t *testing.T) {
	h := NewRouter(newFake(t), tokens)
	tok, _ := tokens.Issue(7, "admin")
	if rec := do(h, "GET", "/api/v1/products", "", ""); rec.Code != 401 {
		t.Fatalf("no token: %d", rec.Code)
	}
	if rec := do(h, "GET", "/api/v1/products", "", "Bearer "+tok+"x"); rec.Code != 401 {
		t.Fatalf("bad token: %d", rec.Code)
	}
	rec := do(h, "GET", "/api/v1/products", "", "Bearer "+tok)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"price_cents":5`) {
		t.Fatalf("valid: %d %s", rec.Code, rec.Body)
	}
}

func TestMethodsAndHealth(t *testing.T) {
	f := newFake(t)
	h := NewRouter(f, tokens)
	if rec := do(h, "GET", "/api/v1/login", "", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET login: %d", rec.Code)
	}
	if rec := do(h, "GET", "/healthz", "", ""); rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"status":"ok"}` {
		t.Fatalf("health: %d %s", rec.Code, rec.Body)
	}
	f.pingErr = errors.New("down")
	if rec := do(h, "GET", "/healthz", "", ""); rec.Code != 503 {
		t.Fatalf("health down: %d", rec.Code)
	}
}
