package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var secret = []byte("0123456789abcdef0123456789abcdef")

func mw(t Tokens) http.Handler {
	return t.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := FromContext(r.Context())
		if !ok || c.Username != "admin" {
			w.WriteHeader(http.StatusTeapot)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
}

func sign(t *testing.T, m jwt.SigningMethod, key any, c Claims) string {
	t.Helper()
	s, err := jwt.NewWithClaims(m, c).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestMiddleware(t *testing.T) {
	now := time.Now()
	tk := Tokens{Secret: secret, TTL: time.Hour}
	good, err := tk.Issue(1, "admin")
	if err != nil {
		t.Fatal(err)
	}
	expired, _ := Tokens{Secret: secret, TTL: time.Hour, Now: func() time.Time { return now.Add(-2 * time.Hour) }}.Issue(1, "admin")
	wrongSecret, _ := Tokens{Secret: []byte("ffffffffffffffffffffffffffffffff"), TTL: time.Hour}.Issue(1, "admin")
	claims := func(iss string, exp *jwt.NumericDate) Claims {
		return Claims{Username: "admin", RegisteredClaims: jwt.RegisteredClaims{Subject: "1", Issuer: iss, ExpiresAt: exp}}
	}
	none := sign(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, claims(Issuer, jwt.NewNumericDate(now.Add(time.Hour))))
	hs512 := sign(t, jwt.SigningMethodHS512, secret, claims(Issuer, jwt.NewNumericDate(now.Add(time.Hour))))
	noExp := sign(t, jwt.SigningMethodHS256, secret, claims(Issuer, nil))
	badIss := sign(t, jwt.SigningMethodHS256, secret, claims("evil", jwt.NewNumericDate(now.Add(time.Hour))))
	parts := strings.Split(good, ".")
	tampered := parts[0] + "." + parts[1] + "x." + parts[2]

	tests := []struct {
		name   string
		header string
		want   int
	}{
		{"valid", "Bearer " + good, 200},
		{"valid lowercase scheme", "bearer " + good, 200},
		{"missing header", "", 401},
		{"bearer without token", "Bearer", 401},
		{"bearer empty token", "Bearer  ", 401},
		{"basic scheme", "Basic " + good, 401},
		{"garbage", "Bearer not.a.jwt", 401},
		{"expired", "Bearer " + expired, 401},
		{"tampered payload", "Bearer " + tampered, 401},
		{"alg none", "Bearer " + none, 401},
		{"alg HS512", "Bearer " + hs512, 401},
		{"wrong secret", "Bearer " + wrongSecret, 401},
		{"no exp", "Bearer " + noExp, 401},
		{"wrong issuer", "Bearer " + badIss, 401},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()
			mw(tk).ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("code = %d, want %d", rec.Code, tt.want)
			}
			if tt.want == 401 && rec.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Fatalf("missing WWW-Authenticate")
			}
		})
	}
}

func TestIssueClaims(t *testing.T) {
	tk := Tokens{Secret: secret, TTL: time.Hour}
	raw, _ := tk.Issue(42, "admin")
	c, err := tk.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if c.Subject != "42" || c.Username != "admin" || c.Issuer != Issuer {
		t.Fatalf("claims: %+v", c)
	}
	if d := c.ExpiresAt.Sub(c.IssuedAt.Time); d != time.Hour {
		t.Fatalf("exp-iat = %v", d)
	}
}

func TestCheckPassword(t *testing.T) {
	h, err := HashPassword("s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if h == "s3cret" || !strings.HasPrefix(h, "$2") {
		t.Fatalf("not a bcrypt hash: %q", h)
	}
	for _, tt := range []struct {
		name, hash, pw string
		want           bool
	}{
		{"match", h, "s3cret", true},
		{"wrong", h, "nope", false},
		{"unknown user (empty hash)", "", "s3cret", false},
		{"garbage hash", "garbage", "s3cret", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := CheckPassword(tt.hash, tt.pw); got != tt.want {
				t.Fatalf("got %v", got)
			}
		})
	}
}
