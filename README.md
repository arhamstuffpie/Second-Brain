# AI Second Brain API

Production-oriented Go REST API skeleton using Gin, PostgreSQL, sqlc, Goose,
Zerolog, constructor dependency injection, and graceful shutdown. The complete
example endpoint is `GET /health`, which runs a sqlc-generated PostgreSQL probe.

## Architecture

`cmd/server/main.go` is the only composition root. It creates each dependency
exactly once and passes it downward:

```text
PostgreSQL -> repositories -> services -> handlers -> router
```

- HTTP handlers only translate HTTP input/output and invoke one use case.
- Services own business logic, authorization, transactions, and orchestration.
- Repositories only perform database operations through generated sqlc queries.
- Services depend on small interfaces declared in the service package.
- Request contexts flow through every layer and JWT claims are attached to the
  standard `context.Context` as an authenticated principal.

The directory name `db/quries` intentionally follows the spelling requested in
the project structure. sqlc outputs generated Go code to `internal/db/sqlc`.

## Configuration

Configuration is read from `APP_*` environment variables through `GetEnv`,
`GetEnvInt`, and `GetEnvBool`. Non-secret operational settings have fallback
values. `APP_DATABASE_URL` and `APP_JWT_SECRET` are required; the JWT secret must
contain at least 32 characters. Copy `.env.example` for local development, but
do not commit `.env`.

Development logging is colored and console-friendly by default. Production
logging defaults to structured JSON. `APP_LOG_PRETTY` can explicitly override
either behavior, and `NO_COLOR=1` disables ANSI colors.

```bash
cp .env.example .env
set -a; source .env; set +a
```

## Database and server

Install sqlc v1.30.0 and Goose v3.25.0, then run:

```bash
make generate
make migrate-up DATABASE_URL="$APP_DATABASE_URL"
make run
```

Example response:

```json
{
  "data": {
    "status": "ok",
    "database": "up",
    "checked_at": "2026-07-22T12:00:00Z"
  },
  "error": "",
  "code": "",
  "message": "service is healthy",
  "paging": null
}
```

If PostgreSQL is unavailable, the endpoint returns HTTP 503 with code
`SERVICE_UNAVAILABLE`.

## Verification

```bash
make test
make vet
make build
```

The test suite covers container dependency contracts, configuration fallbacks,
the health use case and response envelope, request IDs, and JWT enforcement.
