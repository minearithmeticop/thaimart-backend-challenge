# ThaiMart — User Management API

[![CI](https://github.com/minearithmeticop/thaimart-backend-challenge/actions/workflows/ci.yml/badge.svg)](https://github.com/minearithmeticop/thaimart-backend-challenge/actions/workflows/ci.yml)
[![Coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/minearithmeticop/thaimart-backend-challenge/coverage/coverage.json)](https://github.com/minearithmeticop/thaimart-backend-challenge/actions/workflows/ci.yml)

Backend coding challenge: a RESTful user management API in Go with gin,
MongoDB persistence and JWT authentication, built on a hexagonal
(ports & adapters) structure.

## Run locally

Prerequisites: Go 1.26+, Docker.

```bash
# 1. Start infrastructure (MongoDB) and wait until it reports healthy
docker compose -f deploy/compose.yaml up -d
docker compose -f deploy/compose.yaml ps

# 2. Run the API (connects to MongoDB, serves /healthz)
go run ./cmd/api

# 3. Smoke test — returns {"status":"ok"} when MongoDB is reachable
curl http://localhost:8080/healthz
```

Configuration via environment variables (all optional, defaults shown):

| Variable      | Default                     | Purpose                               |
| ------------- | --------------------------- | ------------------------------------- |
| `HTTP_ADDR`   | `:8080`                     | HTTP listen address                   |
| `MONGO_URI`   | `mongodb://localhost:27017` | MongoDB connection URI                |
| `MONGO_DB`    | `thaimart`                  | MongoDB database name                 |
| `JWT_SECRET`  | auto-generated              | HS256 signing key; set in production  |
| `JWT_TTL`     | `1h`                        | Token lifetime                        |
| `BCRYPT_COST` | `10`                        | bcrypt cost factor (4–15)             |
| `LOG_FORMAT`  | `text`                      | `text` or `json`                      |

Tear down:

```bash
docker compose -f deploy/compose.yaml down        # keep data
docker compose -f deploy/compose.yaml down -v     # wipe data volume too
```

## Try it

With the infrastructure up and the API running (`go run ./cmd/api`):

```shell
# Register (public)
curl -s -X POST http://localhost:8080/api/v1/auth/register -H "Content-Type: application/json" -d "{\"name\":\"Somchai\",\"email\":\"somchai@example.com\",\"password\":\"P@ssw0rd1\"}"

# Login — returns a bearer token (public)
curl -s -X POST http://localhost:8080/api/v1/auth/login -H "Content-Type: application/json" -d "{\"email\":\"somchai@example.com\",\"password\":\"P@ssw0rd1\"}"

# Use the token on a protected route (paste the token from the login response)
set TOKEN=<your-token>
curl -s http://localhost:8080/api/v1/users -H "Authorization: Bearer %TOKEN%"
```

## Browse the data with mongo-express (optional)

mongo-express is a web UI for MongoDB. It is not started by default; add
the `tools` profile when you want it:

```bash
docker compose -f deploy/compose.yaml --profile tools up -d
```

Then open <http://localhost:8081> and sign in with:

- Username: `admin`
- Password: `P@assw0rd1`

The API database is `thaimart` (collection `users`). Test leftovers live
in `thaimart_test_*` databases. To stop everything again, use the tear
down commands above.
