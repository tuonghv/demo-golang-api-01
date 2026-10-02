# Architecture and repo review

> There is no `platform/` directory in this repo. This review covers every top-level directory and each package under `internal/`. Keep it in sync when the layout changes.

## Request flow

```
client ──HTTP──▶ cmd/api (main.go: config, store, migrate, seed, http.Server, graceful shutdown)
                   └─▶ internal/httpapi  NewRouter  (ServeMux, Go 1.22+ "METHOD /path" patterns)
                          ├─ GET  /healthz            ─▶ Backend.Ping
                          ├─ POST /api/v1/login       ─▶ loginHandler ─▶ Backend.UserByUsername ─▶ auth.CheckPassword ─▶ auth.Tokens.Issue
                          └─ GET  /api/v1/products    ─▶ auth.Tokens.Middleware ─▶ products.Handler ─▶ Lister.ListProducts
                   internal/store ─▶ PostgreSQL 16 (pgx pool, schema from migrations/*.sql)
```

## Dependency direction

```
cmd/api ─▶ config, auth, httpapi, store, migrations
httpapi ─▶ auth, products, store (types only)
products ─▶ store (types only)
store ─▶ auth (HashPassword, used only by SeedAdmin)
auth, config, migrations ─▶ no internal imports
```

Consumers define small interfaces (`httpapi.Backend`, `products.Lister`), so handlers are tested with fakes and only `store` touches SQL.

## Directory inventory

| Path | Purpose | In use? | Verdict |
|---|---|---|---|
| `cmd/api` | Entry point: wiring, startup order, shutdown | Yes | Good. Wiring only, no logic. |
| `internal/config` | Env parsing and validation (`Load(getenv)` is injectable for tests) | Yes | Good. See note C1. |
| `internal/auth` | bcrypt, HS256 JWT issue/parse, bearer middleware | Yes | Good. See notes A1, A2. |
| `internal/httpapi` | Router, login, health, JSON helper | Yes | Good. See notes H1, H2. |
| `internal/products` | Products list handler (query validation) | Yes | Good. See note H1. |
| `internal/store` | pgx pool, migration runner, queries, row types | Yes | Good. See notes S1, S2. |
| `migrations` | Embedded `*.sql` (applied in filename order, tracked in `schema_migrations`) | Yes | Good. Add new files as `00N_name.sql`; never edit an applied one. |
| `docs` | This file | n/a | New. |
| root files | `Dockerfile`, `docker-compose.yml`, `Makefile`, `.env.example`, `.dockerignore`, `.gitignore` | Yes | See notes R1-R4. |

No dead packages or unused files were found. `go build ./...` and `go vet ./...` pass.

## Consistency notes and suggested cleanups

Nothing below is a bug; these are ordered by value.

- **H1 Duplicate JSON writer.** `httpapi.WriteJSON` and `products.writeJSON` are identical, and `auth.unauthorized` hand-builds JSON. `WriteJSON` is exported but only used inside `httpapi`. Suggest one small shared helper (for example `internal/httpx` with `JSON` and `Error`) used by all three, and `{"error": ...}` bodies built in one place.
- **H2 Exported but internal-only.** `httpapi.MaxBodyBytes`, `auth.BcryptCost`, `auth.FromContext` (used only by a test; nothing reads claims yet) and `store.Store.Pool` (used by tests) widen the API surface. Unexport the constants. Keep `FromContext` only if a handler will need the caller identity.
- **S1 Layering.** `store` imports `auth` just to hash the seed password. Cleaner: `main` hashes and `SeedAdmin(ctx, username, hash)` takes the hash, which makes `store` a leaf package.
- **S2 Types.** `store.Product` carries JSON tags, so the DB row type is also the API response. Fine at this size; introduce a response type in `products` when the two diverge.
- **A1 Naming.** `auth` mixes passwords, tokens and middleware. Acceptable now; split into `auth` (tokens, middleware) and a password helper only if it grows.
- **A2 Timing.** `dummyHash` is computed at package init (about 50 ms at cost 10); fine, but note it when profiling startup.
- **C1 Trimming.** `config.get` trims whitespace but `need` does not, so `JWT_SECRET` and the admin password keep surrounding whitespace by design while other values are trimmed. Document this or make it explicit (see README Known gaps).
- **R1 `.env.example` secret.** The placeholder `JWT_SECRET` is 43 bytes, so copying it unchanged passes validation. Consider making the placeholder shorter than 32 bytes so the API refuses to start until it is replaced.
- **R2 Compose ports.** Postgres is published on `127.0.0.1:5432`; drop it if the host does not need direct DB access.
- **R3 No CI.** There is no `.github/workflows`; `make check` is the only gate. Add a workflow running `make check` (plus the DB tests against a Postgres service).
- **R4 `.dockerignore`** excludes `*.md` and `docker-compose.yml`, which is fine, but `docs/` will be copied into the build context; add `docs` if desired.
- **T1 Tests.** Each package has `_test.go` files in the same directory and style (table tests, fakes). `internal/store` tests skip without `TEST_DATABASE_URL`, so they do not run by default.

## Conventions to keep

- Layout: `cmd/<binary>` for wiring, `internal/<domain>` for logic, one package per concern, tests beside code.
- Errors: log details server-side, return generic `{"error": "..."}` JSON to clients.
- SQL only with bound parameters; passwords only as bcrypt hashes; no secrets in git (see README "Never break").
- Config only through `internal/config`; no `os.Getenv` elsewhere.
