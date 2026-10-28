# Transactional Notification Service

A backend service that processes transactional notifications (order placed, payment received, etc.) through a message queue pipeline. Built to handle real-world concerns like duplicate messages, user rate limiting, and a full audit trail of everything that was sent.

## What it does

When an event comes in (via HTTP), the service publishes it to RabbitMQ. A worker picks it up and runs it through a pipeline:

1. **Deduplication** — if the same message ID was already processed, skip it
2. **Rate limiting** — if the user hit their hourly limit, record it and move on
3. **Template rendering** — fetch the HTML template from MinIO, fill in the payload
4. **Send** — deliver the notification
5. **Audit log** — write the outcome (sent / duplicate / rate_limited / failed) to PostgreSQL

Every message is acknowledged manually — no message is lost silently.

## Stack

- **Go** — core service
- **RabbitMQ** — message broker with a dead-letter queue for failed messages
- **Redis** — deduplication (SetNX, 24h TTL) and rate limiting (10 requests/hour per user)
- **MinIO** — S3-compatible storage for notification templates
- **PostgreSQL** — audit log of every notification

## Getting started

**Prerequisites:** Docker, Go 1.25+

### 1. Start infrastructure

```bash
docker compose up -d
```

This starts RabbitMQ, Redis, PostgreSQL, and MinIO. PostgreSQL runs the migration automatically on first boot and is exposed on host port **5433** (so it doesn't clash with a locally installed Postgres on 5432).

The `minio-init` container creates the **notifications-templates** bucket and uploads every `templates/*.html` file into it. To change a template, edit the file and run `docker compose up minio-init`.

### 2. Set environment variables

```bash
cp .env.example .env
```

The defaults in `.env.example` match the Docker Compose setup, so no changes needed locally. Both binaries load `.env` from the working directory on startup; variables already set in the environment take precedence.

### 3. Templates

Each event type needs a `<event_type>.html` template, e.g. `templates/order.placed.html`:

```
Hello {{.UserName}},

Your order {{.OrderID}} has been placed.
Amount: {{.Amount}}
```

Template variables come from the `payload` field in the request and are case-sensitive.

### 4. Run the worker

```bash
go run ./cmd/worker
```

### 5. Run the producer

```bash
go run ./cmd/producer
```

### 6. Send a test notification 🚀

```bash
curl -X POST http://localhost:8080/publish \
  -H "Content-Type: application/json" \
  -d '{
    "event_type": "order.placed",
    "user_id": "u123",
    "payload": {
      "UserName": "Nurali",
      "OrderID": "ORD-42",
      "Amount": "4500"
    }
  }'
```

You should see the rendered notification printed in the worker terminal.

On Windows PowerShell use `curl.exe` (plain `curl` is an alias for `Invoke-WebRequest`), or import `postman_collection.json` into Postman.

### 7. Query the audit log

```bash
# All notifications for a user
curl "http://localhost:8080/notifications?user_id=u123"

# Filter by status
curl "http://localhost:8080/notifications?status=sent&limit=20"

# Filter by event type
curl "http://localhost:8080/notifications?event_type=order.placed"
```

## Running tests

Unit and integration tests are included. The integration tests need a running PostgreSQL instance.

```bash
# Unit tests only (no external dependencies)
go test ./internal/dedup/... ./internal/ratelimit/... ./internal/notification/...

# All tests including integration
POSTGRES_DSN="postgres://user:Qqwerty1!@localhost:5433/mydb?sslmode=disable" go test ./internal/...

# With benchmarks
go test ./internal/ratelimit/... -bench=. -benchtime=5s
```

## Project structure

```
cmd/
  producer/   — HTTP server (publish events, query audit log)
  worker/     — RabbitMQ consumer
internal/
  broker/     — RabbitMQ connection, exchange and queue setup
  config/     — environment variable loading
  consumer/   — message loop with manual ack/nack
  dedup/      — Redis-based deduplication
  notification/ — dispatcher, sender interface, log sender
  ratelimit/  — Redis-based per-user rate limiter
  store/      — PostgreSQL audit log
  template/   — MinIO template fetching and rendering
migrations/
  001_create_notifications.sql
```

## Docker build

The Dockerfile has separate targets for each binary:

```bash
# Build worker image
docker build --target worker -t notification-worker .

# Build producer image
docker build --target producer -t notification-producer .
```
