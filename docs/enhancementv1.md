# NotifyQ — Enhancement Plan v1

## Overview

This document captures the current architectural issues identified in NotifyQ and the target design to make it
decoupled, adapter-based, scalable, and ready for future tooling (Kafka, stronger auth, Docker/K8s, etc.).

---

## Current State: What Is Wrong

### The Core Problem

Every HTTP handler reaches directly into GORM, builds SQL queries inline, and handles HTTP concerns, business
logic, and persistence all in the same function. There are no interfaces — GORM is the only "abstraction."
This makes it impossible to swap the database, add a message broker, or unit-test handlers without a live database.

---

## Issue Catalogue

### 1. No Repository Pattern — Handlers Directly Call GORM (Critical)

**Location:** all files under `handlers/`

Every handler calls `database.Where(...).Find(...)`, `database.Create(...)`, etc. directly. Business logic,
query building, and HTTP response writing are mixed in the same function body.

**What this blocks:**
- Swapping SQLite → Postgres → DynamoDB requires rewriting every handler
- Adding Kafka as a notification sink requires modifying `SendNotification`
- Unit testing handlers requires a real database

**Fix — define storage interfaces (ports), implement adapters:**

```
storage/
  port.go           ← interfaces: UserRepo, AppRepo, NotificationRepo, NotificationPublisher
  sqlite/
    user.go         ← GORM implementation of UserRepo
    app.go
    notification.go
  kafka/
    publisher.go    ← Kafka implementation of NotificationPublisher (plug in later)
  memory/
    user.go         ← in-memory implementation for tests
```

```go
// storage/port.go
type UserRepository interface {
    Create(ctx context.Context, u *models.User) error
    FindByUsername(ctx context.Context, username string) (*models.User, error)
    FindByID(ctx context.Context, id uint) (*models.User, error)
    Delete(ctx context.Context, id uint) error
}

type NotificationPublisher interface {
    Publish(ctx context.Context, n *models.Notification) error
}
```

Handlers receive these interfaces — not `*gorm.DB`. Swapping the database or adding Kafka means writing a new
adapter; zero handler changes required.

---

### 2. No Dependency Injection — Handlers Are Hard-Wired (Critical)

**Location:** `router/router.go`, all handler constructors

Handlers are plain functions that close over concrete `*gorm.DB` and `*hub.Hub`. There is no service layer
and no composition root beyond `main.go`.

**Fix — introduce a service struct per domain:**

```go
// service/notification.go
type NotificationService struct {
    repo      storage.NotificationRepository
    publisher storage.NotificationPublisher  // Hub today, Kafka tomorrow
    apps      storage.AppRepository
}

func (s *NotificationService) Send(ctx context.Context, appID uint, req SendRequest) (*models.Notification, error) {
    // business logic lives here, not in the handler
}
```

Handlers become thin transport adapters:

```go
func (h *NotificationHandler) Send(c *gin.Context) {
    notif, err := h.svc.Send(c.Request.Context(), appID, req)
    // marshal response, done
}
```

`main.go` becomes the sole composition root — wire interfaces to implementations there.

---

### 3. Goroutine Leaks (Medium)

**Locations:**
- `hub/ticket.go` — `go ts.cleanupLoop()` uses `time.Tick()`, runs forever, no stop mechanism
- `middleware/rate_limit.go` — `go rl.cleanupLoop()` same pattern, one goroutine spawned per route registration

**Fix — accept a context, use `time.NewTicker` with `defer t.Stop()`:**

```go
func NewTicketStore(ctx context.Context) *TicketStore {
    ts := &TicketStore{tickets: make(map[string]ticketEntry)}
    go ts.cleanupLoop(ctx)
    return ts
}

func (ts *TicketStore) cleanupLoop(ctx context.Context) {
    t := time.NewTicker(time.Minute)
    defer t.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-t.C:
            ts.cleanup()
        }
    }
}
```

Pass the root context from `main()` so all loops stop cleanly on shutdown.

---

### 4. No Context Propagation Into DB Queries (Critical)

**Location:** every `database.Where(...).Find(...)` call across all handlers

Every database call ignores the request context. If a client disconnects mid-request, the query keeps running,
wasting connections and CPU.

**Fix — one-line change per query, enforced at the repository interface boundary:**

```go
// Before
database.Where("user_id = ?", uid).Find(&notifs)

// After
database.WithContext(c.Request.Context()).Where("user_id = ?", uid).Find(&notifs)
```

With a proper repository layer the interface signature forces a `ctx` argument, so this cannot be forgotten.

---

### 5. Missing Transaction Boundaries — DeleteUser Is Not Atomic (Medium)

**Location:** `handlers/user_handler.go` — `DeleteUser`

Three separate deletes run with no transaction and no error checks. If any step fails the handler still
returns `200 OK` with orphaned rows in the database.

```go
// Current — not atomic, ignores errors
database.Pluck("id", &appIDs)
database.Delete(notifications)
database.Delete(apps)
database.Delete(&user)
```

**Fix — single transaction in the repository layer:**

```go
func (r *userRepo) Delete(ctx context.Context, id uint) error {
    return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        if err := tx.Unscoped().Where("app_id IN (?)",
            tx.Model(&models.App{}).Select("id").Where("user_id = ?", id),
        ).Delete(&models.Notification{}).Error; err != nil {
            return err
        }
        if err := tx.Where("user_id = ?", id).Delete(&models.App{}).Error; err != nil {
            return err
        }
        return tx.Delete(&models.User{}, id).Error
    })
}
```

---

### 6. Hub — Unbounded Goroutine Spawning for Slow Clients (Medium)

**Location:** `hub/hub.go` broadcast loop

When a client's send channel is full the hub spawns an inline goroutine to unregister it:

```go
go func() { h.unregister <- c }()
```

Under heavy load this creates an unbounded number of goroutines, one per slow client per broadcast.

**Fix — bounded eviction queue drained by a single worker:**

```go
type Hub struct {
    evictQueue chan *Client  // sized, not unbounded
    // ...
}

func (h *Hub) run() {
    go h.evictWorker()
    for {
        select {
        case c := <-h.register:   // ...
        case c := <-h.unregister: // ...
        case b := <-h.broadcast:
            for _, c := range h.clients[b.UserID] {
                select {
                case c.send <- b.Payload:
                default:
                    select {
                    case h.evictQueue <- c:
                    default: // evict queue full — drop silently, log
                    }
                }
            }
        }
    }
}

func (h *Hub) evictWorker() {
    for c := range h.evictQueue {
        h.unregister <- c
    }
}
```

---

### 7. Webhooks Fire Synchronously in Request Goroutine (Medium)

**Location:** `handlers/notification_handler.go` — `fireWebhook` called with `go fireWebhook(...)`

Each `SendNotification` request spawns a raw goroutine to call an external URL. Under load this accumulates
thousands of goroutines with no back-pressure, no retry logic, and no queue depth visibility.

**Fix — bounded webhook worker pool:**

```go
// worker/webhook.go
type WebhookDispatcher struct {
    queue   chan webhookJob
    workers int
}

func NewWebhookDispatcher(workers, queueDepth int) *WebhookDispatcher {
    return &WebhookDispatcher{
        queue:   make(chan webhookJob, queueDepth),
        workers: workers,
    }
}

func (d *WebhookDispatcher) Start(ctx context.Context) {
    for i := 0; i < d.workers; i++ {
        go d.worker(ctx)
    }
}

func (d *WebhookDispatcher) Enqueue(url string, n *models.Notification) {
    select {
    case d.queue <- webhookJob{url: url, notification: n}:
    default:
        logger.L.Warn("webhook queue full, dropping delivery", zap.String("url", url))
    }
}

func (d *WebhookDispatcher) worker(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        case job := <-d.queue:
            d.deliver(ctx, job)
        }
    }
}
```

This gives back-pressure, a bounded goroutine count, and a natural swap point to route into Kafka later.

---

### 8. Auth Has No Interface — No Swap Point for Future Auth Service (Medium)

**Location:** `auth/jwt.go`, `auth/token.go`, `middleware/jwt_auth.go`

JWT and app-token auth are concrete functions. When you introduce OAuth2, OIDC, or an external auth service
there is no seam to cut without modifying every middleware and handler that calls `auth.ParseToken`.

**Fix — define auth ports:**

```go
// auth/port.go
type TokenIssuer interface {
    Issue(userID uint, isAdmin bool) (string, error)
}

type TokenVerifier interface {
    Verify(token string) (*Claims, error)
}
```

The JWT implementation satisfies these today. An external auth service satisfies them tomorrow. Middleware
depends on the interface, not the concrete implementation.

---

### 9. Config Not Validated at Startup (Low)

**Location:** `config/config.go`

`config.Load()` silently uses the default `"change-me-in-production-please"` JWT secret if `JWT_SECRET` is
not set. No validation runs at startup — the error surfaces only at runtime.

**Fix:**

```go
func (c *Config) Validate() error {
    if c.Env == "production" && c.JWTSecret == "change-me-in-production-please" {
        return fmt.Errorf("JWT_SECRET must be set in production")
    }
    if c.ListenAddr == "" {
        return fmt.Errorf("LISTEN_ADDR must not be empty")
    }
    return nil
}
```

Call `cfg.Validate()` in `main()` before anything else starts.

---

### 10. Global Logger Without Interface (Low)

**Location:** `logger/logger.go` — `var L *zap.Logger`

A package-level global works, but it cannot be swapped in tests or mocked. If observability requirements
change (e.g., OpenTelemetry), every call site must change.

**Mitigation:** pass a `*zap.Logger` into service constructors. The global can remain as a convenience
default, but it should not be the only way to get a logger.

---

## Target Architecture

```
notification-queue/
├── cmd/
│   └── server/
│       └── main.go               ← composition root only: wire interfaces → implementations
├── config/                       ← load + validate env vars
├── storage/
│   ├── port.go                   ← UserRepo, AppRepo, NotificationRepo, NotificationPublisher interfaces
│   ├── sqlite/                   ← GORM implementations (current logic moved here)
│   └── memory/                   ← test doubles
├── service/
│   ├── auth.go                   ← AuthService: issue/verify tokens
│   ├── notification.go           ← NotificationService: send, list, mark-read
│   ├── app.go                    ← AppService: CRUD, token rotation
│   └── user.go                   ← UserService: CRUD, admin guard
├── transport/
│   ├── http/
│   │   ├── handlers/             ← thin HTTP handlers, call service methods
│   │   ├── middleware/           ← JWT, AppToken, RateLimit, RequestID, ZapLogger
│   │   └── router.go
│   └── ws/
│       └── hub.go                ← WebSocket hub (implements NotificationPublisher)
├── worker/
│   ├── webhook.go                ← bounded dispatcher (workers + queue channel)
│   └── retention.go              ← retention job (already context-aware, keep as-is)
├── auth/
│   ├── port.go                   ← TokenIssuer, TokenVerifier interfaces
│   └── jwt/                      ← JWT implementation
├── logger/
├── models/
└── docs/
    └── enhancementv1.md          ← this document
```

### Data Flow After Refactor

```
HTTP Request
  → Gin middleware (RequestID, ZapLogger, Auth)
  → Handler (parse + validate input, call service)
  → Service (business logic, calls repository interface)
  → Repository Adapter (GORM/Postgres/DynamoDB — swappable)
  → Service (builds Notification, calls publisher interface)
  → Publisher Adapter (Hub WebSocket today / Kafka tomorrow — swappable)
  → WebhookDispatcher.Enqueue() (bounded worker pool, fire-and-forget)
```

---

## Implementation Priority

| # | Issue | Effort | Impact | Do First? |
|---|-------|--------|--------|-----------|
| 1 | `storage/port.go` — define repo interfaces + sqlite adapters | Medium | Unblocks all other work | Yes |
| 2 | `WithContext` on all DB queries | Low | Correctness under load | Yes |
| 3 | Fix goroutine leaks — context in cleanup loops | Low | Stability | Yes |
| 4 | Wrap `DeleteUser` in a transaction | Low | Data integrity | Yes |
| 5 | Service layer (NotificationService, AppService, etc.) | Medium | Testability, DI | Next |
| 6 | Bounded webhook worker pool | Medium | Concurrency safety | Next |
| 7 | Auth interfaces (`auth/port.go`) | Low | Swap point for future auth | Next |
| 8 | Config `Validate()` at startup | Low | Production safety | Next |
| 9 | Hub eviction — bounded queue instead of inline goroutines | Low | Stability at scale | Later |
| 10 | Logger as constructor argument (not global only) | Low | Testability | Later |

---

## Future Integration Points

Once the port-adapter structure is in place, each integration below requires **only new adapter files**.
Services and handlers remain untouched — they depend on interfaces, not concrete drivers.

---

### Kafka — Notification Event Bus

**Role:** Replace the direct Hub.Broadcast call with a durable, replayable event stream. The notification
service publishes to Kafka; one consumer fans out to WebSocket clients, another handles webhooks. This
decouples write throughput from delivery latency and enables multiple independent consumers.

**Adapter to write:** `storage/kafka/publisher.go`

```go
// storage/kafka/publisher.go
type KafkaPublisher struct {
    producer sarama.SyncProducer
    topic    string
}

func (p *KafkaPublisher) Publish(ctx context.Context, n *models.Notification) error {
    payload, err := json.Marshal(n)
    if err != nil {
        return err
    }
    msg := &sarama.ProducerMessage{
        Topic: p.topic,
        Key:   sarama.StringEncoder(fmt.Sprintf("%d", n.AppID)),
        Value: sarama.ByteEncoder(payload),
    }
    _, _, err = p.producer.SendMessage(msg)
    return err
}
```

**Consumer side** — a worker that reads Kafka and pushes to the WebSocket hub:

```go
// worker/kafka_consumer.go
type NotificationConsumer struct {
    consumer sarama.ConsumerGroup
    hub      *ws.Hub
    topic    string
}

func (c *NotificationConsumer) Run(ctx context.Context) error {
    handler := &consumerGroupHandler{hub: c.hub}
    for {
        if err := c.consumer.Consume(ctx, []string{c.topic}, handler); err != nil {
            return err
        }
        if ctx.Err() != nil {
            return nil
        }
    }
}
```

**Config additions needed:**

```
KAFKA_BROKERS=broker1:9092,broker2:9092
KAFKA_TOPIC=notifications
KAFKA_GROUP_ID=notifyq-ws-consumer
```

**Wire-up in `main.go`:** swap `hub` as the `NotificationPublisher` for `KafkaPublisher` — the
`NotificationService` constructor call is the only line that changes.

---

### Postgres — Relational Database Adapter

**Role:** Replace SQLite for production deployments that need connection pooling, concurrent writes,
horizontal read replicas, and managed hosting (RDS, Cloud SQL, Neon, Supabase).

**Adapter to write:** `storage/postgres/` — same interfaces, different GORM driver.

```go
// storage/postgres/db.go
func Open(dsn string) (*gorm.DB, error) {
    return gorm.Open(postgres.Open(dsn), &gorm.Config{
        Logger: logger.Default.LogMode(logger.Warn),
    })
}
```

```go
// storage/postgres/notification.go
type NotificationRepo struct{ db *gorm.DB }

func (r *NotificationRepo) Create(ctx context.Context, n *models.Notification) error {
    return r.db.WithContext(ctx).Create(n).Error
}
// ... same interface, Postgres-specific tuning (e.g. RETURNING clauses, advisory locks)
```

**Key differences from SQLite adapter:**
- Connection pool: `sql.SetMaxOpenConns`, `sql.SetMaxIdleConns`, `sql.SetConnMaxLifetime`
- Migrations: use `golang-migrate` or Atlas instead of `AutoMigrate` in production
- Soft deletes: Postgres partial indexes on `deleted_at IS NULL` for performance
- Transactions: use `pgx` advisory locks for concurrent token rotation

**Config additions needed:**

```
DATABASE_DRIVER=postgres
DATABASE_DSN=postgres://user:pass@host:5432/notifyq?sslmode=require
DATABASE_MAX_OPEN_CONNS=25
DATABASE_MAX_IDLE_CONNS=5
```

**Wire-up in `main.go`:** `config.DatabaseDriver` selects which `db.Open()` to call — services never know
which driver is underneath.

```go
// cmd/server/main.go
var database *gorm.DB
switch cfg.DatabaseDriver {
case "postgres":
    database, err = postgres.Open(cfg.DatabaseDSN)
default:
    database, err = sqlite.Open(cfg.DatabasePath)
}
```

---

### Redis — Cache and Rate Limiting

**Role:** Redis serves two distinct purposes in this stack:

1. **Rate limiting** — replace the current in-memory per-process limiter with a shared Redis counter so
   rate limits work correctly across multiple server instances.
2. **Notification cache** — cache the last N notifications per user so list reads skip the database on
   hot paths.

#### Rate Limiting Adapter

```go
// middleware/redis_rate_limit.go
import "github.com/go-redis/redis_rate/v10"

func RedisRateLimit(limiter *redis_rate.Limiter, key string, rps int) gin.HandlerFunc {
    return func(c *gin.Context) {
        res, err := limiter.Allow(c.Request.Context(),
            fmt.Sprintf("rate:%s:%s", key, c.ClientIP()),
            redis_rate.PerMinute(rps),
        )
        if err != nil || res.Remaining == 0 {
            c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
            return
        }
        c.Next()
    }
}
```

#### Notification Cache Adapter

```go
// storage/redis/notification_cache.go
type NotificationCache struct {
    client *redis.Client
    ttl    time.Duration
}

// Wraps NotificationRepository — read-through, write-through
func (c *NotificationCache) ListByUser(ctx context.Context, userID uint, limit, offset int) ([]*models.Notification, error) {
    key := fmt.Sprintf("notifs:user:%d:%d:%d", userID, limit, offset)
    cached, err := c.client.Get(ctx, key).Bytes()
    if err == nil {
        var result []*models.Notification
        if json.Unmarshal(cached, &result) == nil {
            return result, nil
        }
    }
    // cache miss — hit the repo, populate cache
    result, err := c.repo.ListByUser(ctx, userID, limit, offset)
    if err == nil {
        if b, err := json.Marshal(result); err == nil {
            c.client.Set(ctx, key, b, c.ttl)
        }
    }
    return result, err
}
```

**Cache invalidation:** on `MarkRead`, `DeleteNotification`, or `SendNotification` — call
`cache.Invalidate(ctx, userID)` which does `DEL notifs:user:<id>:*`.

**Config additions needed:**

```
REDIS_ADDR=redis:6379
REDIS_PASSWORD=
REDIS_DB=0
REDIS_CACHE_TTL=60s
```

**Wire-up in `main.go`:**

```go
rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
notifRepo := &storage.sqlite.NotificationRepo{DB: database}
cachedNotifRepo := &rediscache.NotificationCache{Repo: notifRepo, Client: rdb, TTL: cfg.RedisCacheTTL}
// pass cachedNotifRepo to NotificationService — service sees only the interface
```

---

### Updated Target Structure (with all three integrations)

```
notification-queue/
├── cmd/
│   └── server/
│       └── main.go
├── config/
├── storage/
│   ├── port.go                        ← interfaces (unchanged regardless of driver)
│   ├── sqlite/                        ← current driver
│   ├── postgres/                      ← NEW: Postgres adapter
│   ├── redis/                         ← NEW: cache adapter (wraps repo interface)
│   └── memory/                        ← test doubles
├── service/
│   ├── notification.go
│   ├── app.go
│   ├── user.go
│   └── auth.go
├── transport/
│   ├── http/
│   │   ├── handlers/
│   │   ├── middleware/                ← redis_rate_limit.go added here
│   │   └── router.go
│   └── ws/
│       └── hub.go
├── worker/
│   ├── webhook.go
│   ├── kafka_consumer.go              ← NEW: Kafka → Hub fan-out
│   └── retention.go
├── storage/
│   └── kafka/
│       └── publisher.go               ← NEW: NotificationPublisher via Kafka
├── auth/
│   ├── port.go
│   └── jwt/
├── logger/
├── models/
└── docs/
    └── enhancementv1.md
```

### Full Data Flow (v2 with all integrations)

```
HTTP POST /message  (app token auth)
  → NotificationService.Send()
  → storage/postgres: persist Notification
  → redis/cache: invalidate user cache
  → kafka/publisher: Publish(notification)       ← async, durable
        │
        ├─ worker/kafka_consumer → ws/hub → WebSocket clients
        └─ worker/kafka_consumer → worker/webhook → external URLs (bounded pool)
```

---

## Implementation Priority (updated)

| # | Issue | Effort | When |
|---|-------|--------|------|
| 1 | `storage/port.go` — repo interfaces + sqlite adapters | Medium | Now |
| 2 | `WithContext` on all DB queries | Low | Now |
| 3 | Fix goroutine leaks — context in cleanup loops | Low | Now |
| 4 | Wrap `DeleteUser` in a transaction | Low | Now |
| 5 | Service layer (NotificationService, AppService, etc.) | Medium | Next sprint |
| 6 | Bounded webhook worker pool | Medium | Next sprint |
| 7 | Auth interfaces (`auth/port.go`) | Low | Next sprint |
| 8 | Config `Validate()` at startup | Low | Next sprint |
| 9 | Hub eviction — bounded queue | Low | Next sprint |
| 10 | Postgres adapter (`storage/postgres/`) | Medium | When scaling SQLite |
| 11 | Redis rate limiting + notification cache | Medium | When multi-instance |
| 12 | Kafka publisher + consumer workers | High | When async delivery needed |
