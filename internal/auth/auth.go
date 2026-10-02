// Package auth provides password checking, JWT issue/parse and bearer middleware.
package auth

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Issuer is the fixed `iss` claim.
const Issuer = "demo-golang-api-01"

// BcryptCost is the cost used for stored hashes.
const BcryptCost = 10

// dummyHash is compared against when the user is missing, so unknown-user and
// wrong-password take comparable time. It hashes a random-looking constant.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), BcryptCost)

// CheckPassword reports whether password matches hash. An empty hash (unknown
// user) is compared against a dummy hash and always returns false.
func CheckPassword(hash, password string) bool {
	if hash == "" {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// HashPassword returns a bcrypt hash.
func HashPassword(pw string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), BcryptCost)
	return string(h), err
}

// Claims are the JWT claims.
type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// Tokens issues and parses HS256 tokens.
type Tokens struct {
	Secret []byte
	TTL    time.Duration
	Now    func() time.Time // injectable for tests; defaults to time.Now
}

func (t Tokens) now() time.Time {
	if t.Now != nil {
		return t.Now()
	}
	return time.Now()
}

// Issue creates a signed token for the user.
func (t Tokens) Issue(userID int64, username string) (string, error) {
	n := t.now()
	c := Claims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(userID, 10),
			Issuer:    Issuer,
			IssuedAt:  jwt.NewNumericDate(n),
			ExpiresAt: jwt.NewNumericDate(n.Add(t.TTL)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(t.Secret)
}

// Parse validates a token: HS256 only, signature, exp and iss required.
func (t Tokens) Parse(raw string) (*Claims, error) {
	c := &Claims{}
	_, err := jwt.ParseWithClaims(raw, c, func(*jwt.Token) (any, error) { return t.Secret, nil },
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithExpirationRequired(),
		jwt.WithIssuer(Issuer),
		jwt.WithTimeFunc(t.now),
	)
	if err != nil {
		return nil, err
	}
	return c, nil
}

type ctxKey struct{}

// FromContext returns the authenticated claims set by Middleware.
func FromContext(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(ctxKey{}).(*Claims)
	return c, ok
}

// Middleware requires a valid bearer token, otherwise 401 with WWW-Authenticate.
func (t Tokens) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		scheme, tok, ok := strings.Cut(h, " ")
		tok = strings.TrimSpace(tok)
		if !ok || !strings.EqualFold(scheme, "Bearer") || tok == "" {
			unauthorized(w)
			return
		}
		c, err := t.Parse(tok)
		if err != nil {
			unauthorized(w)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, c)))
	})
}

var errUnauthorized = errors.New("unauthorized")

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"` + errUnauthorized.Error() + `"}` + "\n"))
}
