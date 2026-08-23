# syntax=docker/dockerfile:1

# ---- Build ----------------------------------------------------------------
FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /bin/api ./cmd/api

# ---- Run -------------------------------------------------------------------
FROM alpine:3.22
RUN adduser -D -u 10001 app \
    && apk add --no-cache ca-certificates tzdata
USER app
WORKDIR /app
COPY --from=build /bin/api /app/api

ENV HTTP_ADDR=:8080 GRPC_ADDR=:50051
EXPOSE 8080 50051

HEALTHCHECK --interval=5s --timeout=3s --start-period=5s --retries=12 \
    CMD wget -q -O /dev/null http://localhost:8080/healthz || exit 1

ENTRYPOINT ["/app/api"]
