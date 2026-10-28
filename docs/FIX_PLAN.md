# Fix Plan: Transactional Notification Service

Step-by-step plan for this repo. It merges the "Fix Plan & Codebase Reading Guide" (items N1–N9, same numbers kept) with what a full read of the code found (N10–N13), and corrects three places where the original plan would not work as written (marked **Correction**).

The URL shortener part, the CV bullets and the interview talking points stay in the original document.

---

## The loop for every fix

1. Read the code path and write the current behavior in one sentence.
2. Write a test that **fails** because of the bug. Run it and see it fail.
3. Make the smallest change that turns it green.
4. Run everything:
   ```powershell
   go vet ./...
   go test -race ./...
   ```
   `-race` works on this machine (gcc from MSYS2 is on PATH).
5. One fix per branch, conventional commit, PR into `main`:
   ```powershell
   git checkout main; git pull
   git checkout -b fix/ratelimit-sliding-window
   # ...work...
   git commit -m "fix(ratelimit): replace fixed window with sliding window"
   ```
6. Update the README, `postman_collection.json` and this file if behavior changed. Tick the box.

Manual checks use the stack from `docker compose up -d` plus `go run ./cmd/worker` and `go run ./cmd/producer` (`.env` is loaded automatically). Useful side-effect viewers:

```powershell
docker compose exec redis redis-cli MONITOR
docker compose exec postgres psql -U user -d mydb -c "select message_id,user_id,status,error,sent_at from notifications order by sent_at desc limit 10"
# RabbitMQ UI: http://localhost:15672  (user / Qqwerty1!)
# MinIO UI:    http://localhost:9001   (minioadmin / minioadmin)
```

---

## Overview and order

| Step | Item | Priority | What |
|---|---|---|---|
| 0 | — | done | Local setup works (`chore/local-dev-setup`) |
| 1 | — | — | Map the codebase (no code changes) |
| 2 | N12 | enabler | Make producer, consumer and template code testable |
| 3 | N10 | P1 | Warm-up: dispatcher message ID + swallowed errors |
| 4 | N1 | P0 | Sliding-window rate limiter |
| 5 | N2 | P0 | Retries and dead-letter queue actually work |
| 6 | N3 | P1 | Client idempotency key |
| 7 | N4 | P1 | Publisher confirms |
| 8 | N5 | P1 | Worker concurrency |
| 9 | N11 | P1 | `/notifications` API input and output |
| 10 | N8 | P2 | Config hygiene (remainder) |
| 11 | N13 | P2 | Startup checks and graceful shutdown |
| — | N6, N7, N9 | stretch | Template cache, SMTP sender, reconnect |

Step 2 comes before the bug fixes because N2, N3 and N4 need tests on code that currently can't be tested without real RabbitMQ/MinIO. Step 3 is a deliberately small first TDD cycle.

---

## Step 0 — Local setup (done)

Committed on `chore/local-dev-setup`:

- [x] MinIO images replaced with `pgsty/minio` / `pgsty/mc` (official images were removed from Docker Hub)
- [x] `minio-init` creates the bucket and uploads `templates/*.html`
- [x] Postgres exposed on host port **5433** (a local Windows Postgres 18 owns 5432)
- [x] `.env` loaded automatically (`godotenv` in `internal/config/config.go`)
- [x] `.env.example` credentials match compose
- [x] Dockerfile builder on `golang:1.25-alpine` (was half of N8)
- [x] Postman collection

---

## Step 1 — Map the codebase (no code)

Do the Part 1 exercise from the original guide for this repo. Concretely:

- [ ] Write the call chain with files: `cmd/producer/main.go:publishHandler` → `internal/broker/publisher.go:Publish` → exchange `notifications` (routing key = `event_type`) → queue `notifications.email` (bound with `#`) → `internal/consumer/consumer.go:handle` → `internal/notification/dispatcher.go:Handle` → dedup → ratelimit → template → sender → `internal/store/notification_log.go:Insert`.
- [ ] Answer: where does each piece of state live? (Redis `dedup:<id>`, Redis `ratelimit:<user>`, MinIO `<event>.html`, Postgres `notifications`, RabbitMQ queues.)
- [ ] Answer: which errors are retried, which dropped? (Today: every error → `Nack(requeue=true)`, nothing ever reaches the DLQ.)
- [ ] Run the four experiments and explain each result:
  1. Postman "order.placed" → row `sent`.
  2. Postman "rate limit" ×11 → 11th row `rate_limited`.
  3. Postman "missing template" → worker logs `failed`, then `[DUPLICATE]`; the row stays `failed`. **This is N2 bug 1.**
  4. RabbitMQ UI → Queues → `notifications.email` → Publish message, payload `not json` → the worker loops forever. **This is N2 bug 3.** Stop it by purging the queue.
- [ ] Delve: `dlv test ./internal/notification -- -test.run TestDispatcher_HappyPath`, break on `dispatcher.go:67`, step to the end.

---

## Step 2 — N12: Make the untested code testable

**Problem.** Coverage is 0% for `cmd/producer`, `consumer`, `template` and `config`, because each one depends on a concrete type that needs a live server:

| Code | Depends on | Seam to add |
|---|---|---|
| `publishHandler(conn *broker.Connection)` | live RabbitMQ | `type publisher interface { Publish(ctx context.Context, e broker.Event) error }` |
| `listHandler(notifLog *store.NotificationLog)` | live Postgres | `type notificationLister interface { List(ctx, store.ListFilter) ([]store.NotificationRecord, error) }` |
| `consumer.New(..., dispatcher *notification.Dispatcher)` | the real dispatcher | `type handler interface { Handle(ctx context.Context, messageID string, body []byte) error }` |
| `template.Store.Render` | live MinIO | split into fetch + pure `render(name, raw string, data map[string]any) (string, error)` |

Define each interface **where it is used** (the same pattern `dispatcher.go:14-28` already follows).

**Tests to add.**

- [ ] `cmd/producer/main_test.go` (package `main`), using `httptest.NewRecorder`:
  - valid body → 200, JSON has `message_id`, fake publisher received the event
  - missing `user_id` → 400; invalid JSON → 400; `GET /publish` → 405
  - fake publisher returns an error → 500
- [ ] `internal/consumer/consumer_test.go` (package `consumer`, so it can call `handle` directly). `amqp.Delivery` has an `Acknowledger` field, so a fake records the call:
  ```go
  type fakeAck struct{ acked, nacked, requeue bool }

  func (f *fakeAck) Ack(uint64, bool) error            { f.acked = true; return nil }
  func (f *fakeAck) Nack(_ uint64, _ bool, rq bool) error { f.nacked, f.requeue = true, rq; return nil }
  func (f *fakeAck) Reject(_ uint64, rq bool) error     { f.nacked, f.requeue = true, rq; return nil }

  // msg := amqp.Delivery{Acknowledger: ack, MessageId: "m1", Body: body}
  ```
  - handler returns nil → `acked`
  - handler returns error → `nacked && requeue` (this documents today's behavior; N2 changes it)
- [ ] `internal/template/render_test.go`: payload fills variables; missing key renders `<no value>`; bad template syntax returns an error.
- [ ] `internal/config/config_test.go` with `t.Setenv`: all vars set → loaded; one missing → error naming the variable. (Also fix the typo `vairable` → `variable` in `config.go`.)

Commit: `refactor: add interfaces at producer, consumer and template seams for testing`.

---

## Step 3 — N10: Dispatcher warm-up fixes (P1)

Two small bugs in `internal/notification/dispatcher.go`, good for a first red-green cycle.

**Bug A — wrong ID on the rate-limited path.** Line 82 passes `event.MessageID` (from the JSON body). Every other path passes `messageID` (from the AMQP header). The existing rate-limit test never checks the ID, which is why it slipped through.

- [ ] Failing test (add to `dispatcher_test.go`):
  ```go
  func TestDispatcher_RateLimited_RecordsHeaderMessageID(t *testing.T) {
      logger := &fakeLogger{}
      d := newDispatcher(&fakeDedup{}, &fakeLimiter{allow: false}, &fakeTemplates{}, &fakeSender{}, logger)

      err := d.Handle(context.Background(), "header-id", makeBody(t, "body-id", "u1", "order.placed"))

      require.NoError(t, err)
      assert.Equal(t, "header-id", logger.last.MessageID)
  }
  ```
- [ ] Fix: `event.MessageID` → `messageID` on line 82.

**Bug B — audit-log errors swallowed.** Lines 87 and 97 ignore the return value of `writeRecord`. If Postgres is down, the failure is never recorded and nobody finds out.

- [ ] Failing test: `fakeTemplates{err: errors.New("render")}` + `fakeLogger{err: errors.New("db down")}` → assert the returned error contains both (`assert.ErrorContains` twice).
- [ ] Fix: `return errors.Join(err, d.writeRecord(...))` in both failure branches.

Commit: `fix(notification): use header message id consistently and surface audit log errors`.

---

## Step 4 — N1 (P0): Sliding-window rate limiter

**Problem.** `internal/ratelimit/limiter.go` uses `INCR` + `EXPIRE`: a fixed window that starts at the user's first request. The two commands are not atomic: a crash between them leaves a key with no TTL, which blocks that user forever.

**Fix.** Same as the original plan: a sorted set per user and a Lua script (`ZREMRANGEBYSCORE` → `ZCARD` → `ZADD` → `PEXPIRE`), loaded once with `redis.NewScript`, with a unique member (`fmt.Sprintf("%d-%s", nowMs, uuid.NewString())`). miniredis runs Lua, so the existing test setup still works.

Inject the clock so tests control time. One way that keeps `New(client)` working for `cmd/worker`:

```go
type Option func(*Limiter)

func WithClock(now func() time.Time) Option { return func(l *Limiter) { l.now = now } }

func New(client *redis.Client, opts ...Option) *Limiter {
    l := &Limiter{client: client, now: time.Now}
    for _, opt := range opts {
        opt(l)
    }
    return l
}
```

> **Correction.** The original plan's boundary test ("10 calls at 59m59s, then 1 at 60m01s → denied") passes on the **old** code too. The old window starts at the first call (59m59s), so 60m01s is still inside it and the call is denied either way. A test that really fails on the fixed window has to spread the calls out:

- [ ] Failing test:
  ```go
  func TestAllow_SlidingWindow_NoBurstAtWindowEdge(t *testing.T) {
      client := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
      now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
      l := ratelimit.New(client, ratelimit.WithClock(func() time.Time { return now }))
      ctx := context.Background()

      ok, _ := l.Allow(ctx, "u") // 1 call at t=0
      require.True(t, ok)

      now = now.Add(59 * time.Minute) // 9 calls at t=59m
      for range 9 {
          ok, _ = l.Allow(ctx, "u")
          require.True(t, ok)
      }

      // t=60m01s: only the t=0 call has left the window.
      // Old code fails here: its counter is already at 10, so it denies `first`.
      // (Don't FastForward miniredis: that would expire the new code's key too.)
      now = now.Add(time.Minute + time.Second)

      first, _ := l.Allow(ctx, "u")
      second, _ := l.Allow(ctx, "u")
      assert.True(t, first, "one slot freed by the call at t=0")
      assert.False(t, second, "the 9 calls from t=59m are still in the window")
  }
  ```
- [ ] 10 calls at t=0, 1 call at t=60m01s → allowed.
- [ ] Existing tests still pass, including `TestAllow_HighLoad` (200 concurrent → exactly 10). `TestAllow_ResetsAfterWindow` uses only `FastForward`, so give it the clock too.
- [ ] README: "10 requests/hour" → "10 requests per sliding hour".

Commit: `fix(ratelimit): replace fixed window with atomic sliding window`.

---

## Step 5 — N2 (P0): Retries and the dead-letter queue

**Problem.** Three bugs combine (you saw them in Step 1, experiments 3 and 4):

1. `dispatcher.go:67` marks the ID as seen *before* doing the work, so a failed message is treated as a duplicate on redelivery and never retried.
2. `notification_log.go:49` `ON CONFLICT DO NOTHING` keeps the first status forever (`failed`).
3. `consumer.go:56` always uses `Nack(requeue=true)`: the DLQ is unreachable and poison messages loop forever.

Do it as four separate commits, in this order.

### 5a. Dedup: claim / complete / release

> **Correction.** The original plan treats "Claim failed" as "duplicate". But a failed claim also happens when another worker is still processing, or crashed mid-message (the `processing` key lives 300 s). If that redelivery is acked as a duplicate, the message is lost. Only `done` means duplicate.

```go
type ClaimResult int

const (
    Claimed    ClaimResult = iota // we own it, go ahead
    Done                          // already sent: duplicate, ack and skip
    InProgress                    // someone else has it, or a crashed worker did: retry later
)
```

- `Claim(id)`: `SET dedup:<id> processing NX EX 300`. On success → `Claimed`. Otherwise `GET` it: `"done"` → `Done`; anything else (including the key expiring in between) → `InProgress`.
- `Complete(id)`: `SET dedup:<id> done EX 86400`.
- `Release(id)`: `DEL dedup:<id>`.

Dispatcher: `Done` → write `duplicate`, return nil. `InProgress` → return a transient error (requeue). After a send or render failure → `Release`, then return the error. After a successful send → `Complete`.

Rate-limited messages: call `Complete` (they're finished; a redelivery shouldn't count against the user again).

- [ ] Dedup tests (miniredis): claim → claimed; claim again → in progress; complete then claim → done; release then claim → claimed; `FastForward(301s)` on a processing key → claimable.
- [ ] Dispatcher test: a stateful fake dedup (map) plus a sender that fails twice and then succeeds. Call `Handle` 3 times with the same ID → 3 send calls, last status `sent`.

### 5b. Classify errors

- [ ] Add `var ErrPermanent = errors.New("permanent")` in `internal/notification`. Wrap invalid JSON and a missing template with `fmt.Errorf("%w: ...", ErrPermanent, err)`. Everything else is transient.
- [ ] A missing template from MinIO surfaces on `io.ReadAll` (not on `GetObject`, which is lazy). Check `minio.ToErrorResponse(err).Code == "NoSuchKey"` on the raw error in `template/store.go`, **before** wrapping it, because `ToErrorResponse` doesn't unwrap.
- [ ] Tests: invalid JSON → `errors.Is(err, ErrPermanent)`; template "not found" → permanent; sender error → not permanent.

### 5c. Consumer and quorum queue

- [ ] `consumer.handle`: `errors.Is(err, notification.ErrPermanent)` → `Nack(false, false)` (to the DLQ); other error → `Nack(false, true)`; nil → `Ack`. Extend the Step 2 consumer tests.
- [ ] `broker/connection.go`: add `"x-queue-type": "quorum"` and `"x-delivery-limit": 3` to the main queue's args, keeping the DLX args. RabbitMQ then dead-letters a message after 3 failed deliveries, so transient errors are bounded too.
- [ ] **Delete the old queue once** (RabbitMQ UI → Queues → `notifications.email` → Delete). Otherwise both binaries fail at startup with `PRECONDITION_FAILED - inequivalent arg 'x-queue-type'`, because a queue's type can't change in place. Note that `setup()` runs in **both** the producer and the worker.

### 5d. Audit log upsert

- [ ] `migrations/002_add_attempts.sql`: `ALTER TABLE notifications ADD COLUMN attempts INT NOT NULL DEFAULT 1;`. Mount it in `docker-compose.yml` next to 001. Init scripts only run on an **empty** volume, so for your existing DB either run it by hand with `psql` or `docker compose down -v` (this wipes the data).
- [ ] Change the insert to the upsert from the original plan (`... DO UPDATE ... WHERE notifications.status <> 'sent'`).
- [ ] Integration test (`$env:POSTGRES_DSN="postgres://user:Qqwerty1!@localhost:5433/mydb?sslmode=disable"`): insert `failed`, then `sent` → one row, status `sent`, attempts 2; then insert `failed` again → still `sent`.

**Manual check for all of N2.**
- [ ] Postman "missing template" → the message appears in `notifications.deadletter` (RabbitMQ UI), the row is `failed`, and the worker does not loop.
- [ ] Publish `not json` from the RabbitMQ UI → straight to the DLQ.
- [ ] Stop Redis (`docker compose stop redis`), publish, start Redis again → the message is retried and ends `sent`.

Commits: `fix(dedup): claim/complete/release so failed messages are retried`, `feat(notification): classify permanent errors`, `fix(consumer): route permanent failures to dlq with bounded retries`, `fix(store): upsert audit row and count attempts`.

---

## Step 6 — N3 (P1): Client idempotency key

**Problem.** `cmd/producer/main.go:81` makes a new UUID per HTTP call, so a client retry creates a real duplicate. (It's also why you can't trigger the `duplicate` path from Postman today.)

- [ ] Read the `Idempotency-Key` header; if present, require 1–128 printable characters (else 400) and use it as `MessageID`; otherwise fall back to a UUID.
- [ ] Unit test (Step 2's fake publisher): the header value becomes `event.MessageID`; a 200-char key → 400.
- [ ] Manual: add an `Idempotency-Key: order-42` header in Postman, send twice → one `sent` row, `[DUPLICATE]` in the worker log.
- [ ] Add the header to a request in `postman_collection.json`.

---

## Step 7 — N4 (P1): Publisher confirms

**Problem.** `PublishWithContext` returns once bytes hit the socket. If the broker drops the message, the client still gets 200.

- [ ] In `broker.Connect` (or a producer-only option), call `ch.Confirm(false)`. In `Publish`, use `PublishWithDeferredConfirmWithContext` and `conf.WaitContext(ctx)`; not acked → error. The producer returns **503** for that case.
- [ ] Also set `mandatory: true` and handle `NotifyReturn`, or document why not: with the `#` binding every message is routable today, but a future binding change would silently drop messages.
- [ ] Manual: `docker compose stop rabbitmq`, send from Postman → 5xx, not 200.

---

## Step 8 — N5 (P1): Worker concurrency

**Problem.** One goroutine handles one message at a time, and with no prefetch limit RabbitMQ pushes the whole backlog into one consumer's memory.

- [ ] `ch.Qos(20, 0, false)` before `Consume`.
- [ ] `WORKER_CONCURRENCY` (default 8, optional in `config.Load`) goroutines ranging over the same `msgs` channel, joined with a `sync.WaitGroup` on shutdown.
- [ ] Dedup and the rate limiter are already atomic in Redis, so they're safe across goroutines. The fakes in `dispatcher_test.go` are not (`fakeLogger.last`). Use a mutex if a concurrent test needs them.
- [ ] Measure: a small `cmd/loadgen` that publishes 10,000 events, then msgs/sec before and after. Use different `user_id`s or the rate limiter will absorb the test.
- [ ] `go test -race ./...`

---

## Step 9 — N11 (P1): `/notifications` API input and output (new)

Found in `cmd/producer/main.go:99-131` and `internal/store/notification_log.go`:

| Input | Today | Should be |
|---|---|---|
| `?offset=-1` | Postgres error → **500** | 400 |
| `?status=bogus` | enum cast error → **500** | 400 (allow only `sent`, `failed`, `rate_limited`, `duplicate`) |
| `?limit=abc` | silently 50 | 400, or keep 50 but document it |
| no rows | `"data": null` | `"data": []` |
| JSON keys | `MessageID`, `SentAt` (no `json` tags) | `message_id`, `sent_at` |

- [ ] Handler tests (Step 2's fake lister) for each row.
- [ ] Add `json:"..."` tags to `store.NotificationRecord`, `Error` with `omitempty`. This changes the API: update the README examples.
- [ ] Initialize `records := []NotificationRecord{}` in `List`.

---

## Step 10 — N8 (P2): Config hygiene (remainder)

- [x] Dockerfile on `golang:1.25-alpine`
- [x] `.env.example` matches compose
- [ ] Remove the real-looking password `Qqwerty1!` from `docker-compose.yml`: use `${POSTGRES_PASSWORD}` / `${RABBITMQ_PASSWORD}` from `.env` (compose reads `.env` automatically), with placeholder values in `.env.example`.
- [ ] Add `producer` and `worker` services to compose (build targets already exist). Inside the compose network the hosts are `rabbitmq:5672`, `redis:6379`, `postgres:5432`, `minio:9000`, **not** `localhost`, so set those in an `environment:` block (it overrides `env_file`). Use `depends_on` with `condition: service_healthy` for rabbitmq and postgres.
- [ ] Verify: `docker compose up -d --build` then Postman → `sent`, with no `go run`.

---

## Step 11 — N13 (P2): Startup checks and shutdown (new)

- [ ] `cmd/worker/main.go:35`: Redis is never pinged, so a bad `REDIS_ADDR` only shows up on the first message. Add `redisClient.Ping(ctx)` and fail fast like the other dependencies.
- [ ] `cmd/producer/main.go:54`: `srv.Shutdown(context.Background())` can wait forever. Use `context.WithTimeout(..., 10*time.Second)`.
- [ ] `cmd/worker`: on SIGTERM the same `ctx` is cancelled and passed into `Handle`, so an in-flight send fails, gets released and is requeued. That's safe after N2, but cleaner is to stop *consuming* on the signal, and let in-flight messages finish with their own context (pairs with the WaitGroup from N5).
- [ ] Use `signal.NotifyContext` instead of the manual channel in both mains.

---

## Stretch

Unchanged from the original plan:

- [ ] **N6** Template cache (RWMutex, 5-min TTL) behind a small fetcher interface, so a test can count MinIO calls. Switch to `html/template` so `<script>` in a payload is escaped.
- [ ] **N7** `SMTPSender` + Mailpit container (`axllent/mailpit`, UI on 8025), selected by `SENDER=log|smtp`. The payload needs an `email` field.
- [ ] **N9** Reconnect on `conn.NotifyClose` with exponential backoff, redeclaring the topology.
- [ ] **N2 extra** Backoff retries via a `notifications.retry` queue with `x-message-ttl`, dead-lettering back to `notifications`.

---

## Checklist

**Week 1**
- [ ] Step 1 map + experiments
- [ ] Step 2 (N12) testability
- [ ] Step 3 (N10) warm-up
- [ ] Step 4 (N1) sliding window
- [ ] Step 5 (N2) retries + DLQ

**Week 2**
- [ ] Step 6 (N3) idempotency key
- [ ] Step 7 (N4) publisher confirms
- [ ] Step 8 (N5) worker pool + msgs/sec number
- [ ] Step 9 (N11) API fixes
- [ ] Step 10 (N8) one-command `docker compose up`
- [ ] Step 11 (N13) startup and shutdown
- [ ] README + CV bullets updated (original guide, Part 4)
