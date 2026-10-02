# demo-golang-api-01

Small Go REST API: `POST /api/v1/login` issues a JWT, `GET /api/v1/products` returns products from PostgreSQL for authenticated callers. Runs with Docker Compose (API + PostgreSQL 16).

## Prerequisites
- Docker with the Compose plugin (v2+) for the stack; Go 1.24+ for tests and local builds.

## Run
```bash
cp .env.example .env          # then edit .env: replace every change-me value
# JWT_SECRET must be >= 32 bytes, e.g.: openssl rand -hex 32
docker compose up -d --build --wait
```
Compose refuses to start if `POSTGRES_PASSWORD`, `JWT_SECRET` or `SEED_ADMIN_PASSWORD` is unset. The API applies the schema and seed products (6 rows) on startup and creates the admin user (`SEED_ADMIN_USERNAME`, default `admin`) with a bcrypt hash of `SEED_ADMIN_PASSWORD`. Ports are bound to 127.0.0.1 only.

Note: the admin user is created only if it does not exist; changing `SEED_ADMIN_PASSWORD` later does not rotate the stored password.

## Try it
```bash
curl -s localhost:8080/healthz

TOKEN=$(curl -s -X POST localhost:8080/api/v1/login -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"<your SEED_ADMIN_PASSWORD>"}' | sed -E 's/.*"token":"([^"]+)".*/\1/')

curl -s -H "Authorization: Bearer $TOKEN" 'localhost:8080/api/v1/products?limit=5&offset=0'
curl -i localhost:8080/api/v1/products     # 401 without a token
```
Responses: login 200 `{"token","expires_in":3600}` / 401 `{"error":"invalid credentials"}` / 400 bad body (max 1 KiB); products 200 `{"items":[{"id","name","price_cents","created_at"}]}`, `limit` 1-100 (default 20), `offset` >= 0, else 400; 401 with `WWW-Authenticate: Bearer` for missing, invalid or expired tokens.

## Configuration
| Variable | Default | Notes |
|---|---|---|
| `POSTGRES_PASSWORD` | none, required | |
| `POSTGRES_USER` / `POSTGRES_DB` | `app` / `app` | |
| `JWT_SECRET` | none, required | >= 32 bytes |
| `JWT_TTL` | `1h` | Go duration |
| `SEED_ADMIN_USERNAME` | `admin` | |
| `SEED_ADMIN_PASSWORD` | none, required | |
| `HTTP_ADDR` | `:8080` | API listen address |
| `DATABASE_URL` | composed by compose | required when running the binary directly |

## Tests
```bash
make check                    # gofmt, go vet, go build, go test -race
TEST_DATABASE_URL='postgres://user:pw@127.0.0.1:5432/scratchdb?sslmode=disable' go test -race -count=1 ./internal/store/
```
DB-backed tests are skipped without `TEST_DATABASE_URL`. They DROP the `users`, `products` and `schema_migrations` tables, so point them only at a disposable database.

## Teardown
```bash
docker compose down           # keeps data in the pgdata volume
# docker compose down -v      # DELETES the database volume; only if you mean it
```

## Known gaps
Login requires `Content-Type: application/json` (415 otherwise). Unknown query parameters and malformed query strings (e.g. containing `;`) are ignored rather than rejected; a JWT_SECRET of 32+ whitespace bytes is accepted. No rate limiting on login, no TLS, no refresh tokens or token revocation (local demo scope).

## Development

This section is the project context for engineers and AI agents; keep it factual and update it when it changes.

- **Layout:** `cmd/api` (wiring only), `internal/{config,auth,products,httpapi,store}`, `migrations/*.sql`, `Dockerfile`, `docker-compose.yml`, `.env.example`, `Makefile`.
- **Dependencies:** stdlib first; only `github.com/jackc/pgx/v5`, `golang.org/x/crypto/bcrypt`, `github.com/golang-jwt/jwt/v5` (pinned to versions that fit `go 1.24`; build images with `GOTOOLCHAIN=local`).
- **Checks** (from the repo root): `test -z "$(gofmt -l .)" && go vet ./... && go build ./... && go test -race -count=1 ./...`. DB-backed tests need `TEST_DATABASE_URL` and are skipped without it.
- **Stack:** `docker compose up -d --build --wait`; stop with `docker compose down` (`-v` deletes the database volume, ask first). Config check without a daemon: `docker compose --env-file .env.example config -q`.
- **Never break:** passwords only as bcrypt hashes; no secrets in git (`.env` is ignored, `.env.example` holds placeholders); SQL only with bound parameters; `JWT_SECRET` and DB password have no default.
- **Unproven:** the Compose runtime (Dockerfile build, healthchecks, `depends_on`) was only config-validated in the first build, not run; see Known gaps above for the rest.
