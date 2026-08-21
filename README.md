# ThaiMart — User Management API

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

| Variable    | Default                    | Purpose                |
| ----------- | -------------------------- | ---------------------- |
| `HTTP_ADDR` | `:8080`                    | HTTP listen address    |
| `MONGO_URI` | `mongodb://localhost:27017` | MongoDB connection URI |

Tear down:

```bash
docker compose -f deploy/compose.yaml down        # keep data
docker compose -f deploy/compose.yaml down -v     # wipe data volume too
```
