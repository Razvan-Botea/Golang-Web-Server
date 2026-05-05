# Golang Web Server (Chirpy)

A small Go web server providing user auth, chirps (short posts), and refresh token handling. Uses PostgreSQL (sqlc-generated queries) and JWT for auth. Designed for local development and simple integrations (e.g., Polka webhooks).

## Features
- User signup, login (JWT access + refresh tokens)
- Create / list / delete chirps
- Polka webhook to upgrade users
- Admin endpoints for metrics and reset
- Password hashing with Argon2id

## Prerequisites
- Go 1.20+ (or recent stable)
- PostgreSQL
- (Optional) direnv or dotenv to load environment variables

## Environment
The service reads configuration from environment variables (godotenv is supported). Important variables:

- DB_URL - PostgreSQL connection string (e.g. postgres://user:pass@localhost:5432/dbname?sslmode=disable)
- JWT_SECRET - secret used to sign JWT access tokens
- POLKA_KEY - API key expected by `/api/polka/webhooks`
- PLATFORM - (optional) platform identifier used by the app

Create a `.env` file for local development with these values.

## Build & Run

Run directly (development):

  go run main.go

Or build a binary:

  go build -o chirpy ./
  ./chirpy

The server listens on :8080 by default.

## Database
This project uses sqlc-generated database code in `internal/database`. Ensure the PostgreSQL database in DB_URL is created and accessible. There are SQL files under `internal/database` (generated code present) — run your migrations prior to starting the server.

Example migration (psql):

  psql "$DB_URL" -f path/to/migrations/001_init.sql

(Adjust to your migration tooling.)

## HTTP API
Base URL: http://localhost:8080

Public endpoints:

- GET /api/healthz
  - health check

- POST /api/users
  - create user
  - body: { "email": "...", "password": "..." }

- POST /api/login
  - login user
  - body: { "email": "...", "password": "..." }
  - response: { user: {...}, token: "<jwt>", refresh_token: "<token>" }

- POST /api/refresh
  - exchange refresh token for new access token
  - Authorization: Bearer <refresh_token>

- POST /api/revoke
  - revoke a refresh token
  - Authorization: Bearer <refresh_token>

Authenticated endpoints (use `Authorization: Bearer <jwt>`):

- POST /api/chirps
  - create a chirp
  - body: { "body": "..." }

- GET /api/chirps
  - list chirps (optional ?author_id=UUID)

- GET /api/chirps/{chirpID}
  - get single chirp

- DELETE /api/chirps/{chirpID}
  - delete a chirp (must own the chirp)

- PUT /api/users
  - update user (password/email)
  - body: { "password":"...", "email":"..." }

Polka webhook:

- POST /api/polka/webhooks
  - Body: { "event":"user.upgraded", "data": { "user_id": "<uuid>" } }
  - Header: Authorization: ApiKey <POLKA_KEY>

Admin endpoints:

- GET /admin/metrics
- POST /admin/reset

Static files served under `/app/` (file server rooted at repo dir).

## Examples

Signup:

  curl -X POST http://localhost:8080/api/users \
    -H "Content-Type: application/json" \
    -d '{"email":"alice@example.com","password":"s3cr3t"}'

Login:

  curl -X POST http://localhost:8080/api/login \
    -H "Content-Type: application/json" \
    -d '{"email":"alice@example.com","password":"s3cr3t"}'

Create chirp (with JWT):

  curl -X POST http://localhost:8080/api/chirps \
    -H "Authorization: Bearer <jwt>" \
    -H "Content-Type: application/json" \
    -d '{"body":"Hello world"}'

## Tests
Run unit tests:

  go test ./...

There are tests under `internal/auth` as a starting point.

## Development notes
- The project uses `github.com/joho/godotenv` to load a `.env` file for local development; production deployments should set real environment variables.
- SQL access is implemented via sqlc-generated code in `internal/database`.
- Password hashing uses Argon2id (`github.com/alexedwards/argon2id`).
- JWTs are created with `github.com/golang-jwt/jwt/v5`.