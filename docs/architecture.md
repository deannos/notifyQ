# NotifyQ — Architecture Document

**Version:** 1.0  
**Module:** `github.com/deannos/notification-queue`  
**Language:** Go 1.25 (CGO required for SQLite)  
**Status:** Active development

---

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Current Architecture](#2-current-architecture)
   - [Directory Structure](#21-directory-structure)
   - [Package Responsibilities](#22-package-responsibilities)
   - [Data Models](#23-data-models)
   - [Request Lifecycle](#24-request-lifecycle)
   - [Authentication Model](#25-authentication-model)
   - [WebSocket Architecture](#26-websocket-architecture)
   - [Concurrency Model](#27-concurrency-model)
   - [Storage Layer](#28-storage-layer)
3. [API Reference](#3-api-reference)
4. [Configuration](#4-configuration)
5. [Dependency Graph](#5-dependency-graph)
6. [Design Decisions](#6-design-decisions)
7. [Known Limitations](#7-known-limitations)
8. [Roadmap](#8-roadmap)
   - [Phase 1 — Service Layer & DI](#phase-1--service-layer--di-issue-21)
   - [Phase 2 — Concurrency & Reliability](#phase-2--concurrency--reliability-issues-25-26)
   - [Phase 3 — Auth Hardening](#phase-3--auth-hardening-issue-27)
   - [Phase 4 — Postgres](#phase-4--postgres-issue-30)
   - [Phase 5 — Redis](#phase-5--redis-issue-32)
   - [Phase 6 — Kafka](#phase-6--kafka-issue-31)
   - [Phase 7 — Observability](#phase-7--observability)
   - [Phase 8 — Multi-tenancy & Enterprise](#phase-8--multi-tenancy--enterprise)
9. [Target Architecture (v2)](#9-target-architecture-v2)
10. [Deployment](#10-deployment)

---

## 1. Project Overview

NotifyQ is a self-hosted push notification server inspired by Gotify. It lets applications send
notifications to users over WebSockets via a simple REST API, with JWT-based user authentication
and per-application token-based publishing.

**Core capabilities:**
- Real-time notification delivery over WebSockets
- REST API for sending, listing, reading, and deleting notifications
- Per-application token authentication (send path)
- JWT-based user authentication (read/manage path)
- Webhook fan-out per application
- Notification retention with configurable TTL
- Embedded web UI (no separate frontend server)
- Single binary, single SQLite file — zero external dependencies to run

---

## 2. Current Architecture

### 2.1 Directory Structure

```
notification-queue/
├── main.go                    ← composition root: wire all deps, start server
├── config/
│   └── config.go              ← env-var loading + startup validation
├── models/
│   ├── user.go                ← User GORM model
│   ├── app.go                 ← App GORM model (token stored as SHA-256 hash)
│   └── notification.go        ← Notification GORM model (soft-delete)
├── storage/
│   ├── port.go                ← repository interfaces (UserRepository, AppRepository,
│   │                             NotificationRepository, NotificationPublisher)
│   └── sqlite/
│       ├── user.go            ← SQLite/GORM implementation of UserRepository
│       ├── app.go             ← SQLite/GORM implementation of AppRepository
│       └── notification.go    ← SQLite/GORM implementation of NotificationRepository
├── auth/
│   ├── jwt.go                 ← JWT generation and parsing (HS256)
│   └── token.go               ← App token generation, SHA-256 hashing, prefix
├── db/
│   ├── db.go                  ← SQLite open (WAL mode, FK enforcement, GORM init)
│   └── retention.go           ← background worker: purge old notifications
├── hub/
│   ├── hub.go                 ← WebSocket hub: register/unregister/broadcast event loop
│   └── ticket.go              ← Short-lived WS auth tickets (30s expiry)
├── handlers/
│   ├── auth_handler.go        ← Login, Register, EnsureAdminUser
│   ├── user_handler.go        ← ListUsers, CreateUser, DeleteUser, ChangePassword
│   ├── app_handler.go         ← ListApps, CreateApp, UpdateApp, DeleteApp, RotateToken
│   ├── notification_handler.go← SendNotification, List, Get, MarkRead, Delete, DeleteAll
│   └── ws_handler.go          ← WebSocketHandler, IssueWSTicket, HealthHandler
├── middleware/
│   ├── jwt_auth.go            ← JWTAuth, AdminOnly, WSJWTAuth
│   ├── app_token_auth.go      ← AppTokenAuth (X-App-Token / ?token=)
│   ├── rate_limit.go          ← per-IP token-bucket rate limiter
│   └── logger.go              ← RequestID injection + Zap request logger
├── router/
│   └── router.go              ← Gin route registration, middleware chains
├── logger/
│   └── logger.go              ← Zap logger init (dev: colored console, prod: JSON)
├── web/
│   └── embed.go               ← embedded static assets (HTML/JS/CSS)
└── docs/
    ├── architecture.md        ← this document
    └── enhancementv1.md       ← issue-level improvement catalogue
```

---

### 2.2 Package Responsibilities

| Package | Responsibility | Depends On |
|---------|---------------|------------|
| `config` | Load and validate env vars | stdlib only |
| `models` | GORM struct definitions (no logic) | gorm |
| `storage` | Repository interfaces (port.go) | models |
| `storage/sqlite` | GORM implementations of all repo interfaces | storage, models, gorm |
| `auth` | JWT issue/parse, app token generate/hash | golang-jwt, stdlib |
| `db` | Open SQLite, run AutoMigrate, retention worker | gorm, models, logger |
| `hub` | WebSocket client registry, broadcast event loop, WS tickets | websocket, logger |
| `middleware` | Gin middleware: auth, rate-limit, logging | auth, storage, config, logger |
| `handlers` | HTTP request parsing → repo calls → response | storage, auth, hub, models |
| `router` | Route registration, middleware chains | handlers, middleware, hub, config |
| `logger` | Zap singleton, dev/prod config | zap |
| `main` | Composition root only — wire deps, start server | all packages |

**Dependency rule:** `storage/port.go` is the seam. Packages above it (handlers, middleware,
router) depend on interfaces. Packages below it (storage/sqlite) depend on the concrete driver.
`*gorm.DB` never crosses this boundary outward.

---

### 2.3 Data Models

```
┌─────────────┐       ┌────────────────────┐       ┌──────────────────────────┐
│    User     │       │       App          │       │      Notification        │
│─────────────│       │────────────────────│       │──────────────────────────│
│ id (PK)     │──1:N──│ id (PK)            │──1:N──│ id (PK)                  │
│ username    │       │ user_id (FK→User)  │       │ app_id (FK→App)          │
│ password    │       │ name               │       │ title                    │
│ is_admin    │       │ description        │       │ message                  │
│ created_at  │       │ webhook_url        │       │ priority (0–10, def 5)   │
└─────────────┘       │ token_prefix (idx) │       │ read (bool)              │
                      │ token (SHA-256,uniq│       │ created_at               │
                      │ created_at         │       │ deleted_at (soft-delete) │
                      └────────────────────┘       └──────────────────────────┘
```

**Key design choices:**
- `User.password` — bcrypt hash, excluded from all JSON serialisation (`json:"-"`)
- `App.token` — stored as SHA-256(plaintext); plaintext returned once at creation only
- `App.token_prefix` — first 8 chars indexed for fast `WHERE token_prefix = ? AND token = ?`
  lookup without scanning the full hash column
- `Notification.deleted_at` — GORM soft-delete; hard-delete only on app/user cascade

---

### 2.4 Request Lifecycle

#### Send Notification (app-token path)

```
POST /message
  │
  ├─ AppTokenAuth middleware
  │    ├─ extract X-App-Token header (or ?token= query)
  │    ├─ compute prefix + SHA-256 hash
  │    └─ AppRepository.FindByToken(ctx, prefix, hash) → sets CtxApp
  │
  ├─ RateLimit middleware (1 req/s burst 60 per IP)
  │
  └─ handlers.SendNotification
       ├─ bind + validate JSON body
       ├─ NotificationRepository.Create(ctx, notif)
       ├─ NotificationPublisher.Publish(ctx, userID, payload) → Hub.Send
       └─ go fireWebhook(url, notif)   [raw goroutine — see issue #26]
```

#### List Notifications (JWT path)

```
GET /api/v1/notification?limit=20&offset=0&read=false&priority=5&q=keyword
  │
  ├─ JWTAuth middleware
  │    ├─ extract Bearer token from Authorization header
  │    └─ auth.ParseToken(token, secret) → sets CtxUserID, CtxIsAdmin
  │
  └─ handlers.ListNotifications
       ├─ parse query params into storage.NotificationFilter
       └─ NotificationRepository.List(ctx, userID, filter)
            ├─ JOIN apps ON apps.id = notifications.app_id WHERE apps.user_id = ?
            ├─ apply optional filters (app_id, read, priority, LIKE query)
            ├─ COUNT(*) for pagination total
            └─ SELECT with ORDER BY created_at DESC, LIMIT, OFFSET
```

---

### 2.5 Authentication Model

NotifyQ uses two parallel authentication schemes:

#### JWT (user sessions)

```
Client                          Server
  │── POST /auth/login ────────► bcrypt.CompareHashAndPassword
  │◄─ { token, expires_in } ─── auth.GenerateToken (HS256, configurable TTL)
  │
  │── GET /api/v1/notification ─► JWTAuth middleware
  │   Authorization: Bearer <jwt>  └─ auth.ParseToken → UserID, IsAdmin
```

- Algorithm: HS256
- Claims: `user_id`, `is_admin`, `exp`, `iat`
- No refresh tokens (planned — see Phase 3)
- No token blacklist (planned — see Phase 3)
- WebSocket falls back to `?token=` query param via `WSJWTAuth` (short-lived tickets preferred)

#### App Token (machine-to-machine send path)

```
Client (application)            Server
  │── POST /message ───────────► AppTokenAuth middleware
  │   X-App-Token: <64-char hex>  ├─ prefix = token[:8]
  │                               ├─ hash   = SHA-256(token)
  │                               └─ SELECT * FROM apps WHERE token_prefix=? AND token=?
```

- Token: 32 random bytes → 64-char hex string
- Stored: SHA-256 hash (high-entropy random string, not a password → SHA-256 is appropriate)
- Prefix index avoids full table scan on every inbound message
- Rotatable via `POST /api/v1/application/:id/token`

#### WebSocket Ticket Auth

```
Client                          Server
  │── GET /api/v1/ws/ticket ───► IssueWSTicket (requires valid JWT)
  │◄─ { ticket: "<32-byte hex>" } └─ TicketStore.Issue(userID) — 30s TTL
  │
  │── GET /ws?ticket=<ticket> ──► WebSocketHandler
  │                               └─ TicketStore.Consume(ticket) → userID
```

Tickets prevent long-lived JWTs from appearing in browser URL bars / server logs.

---

### 2.6 WebSocket Architecture

```
                   Hub (single goroutine event loop)
                   ┌─────────────────────────────────┐
  register ──256──►│                                 │
  unregister ─256─►│  clients: map[userID][]*Client  │
  broadcast ─1024─►│                                 │
                   └─────────────────────────────────┘
                          │ send  │ send  │ send
                          ▼       ▼       ▼
                       Client  Client  Client
                    (per conn) (per conn) (per conn)
                       │           │           │
                    WritePump   WritePump   WritePump   ← goroutine each
                    ReadPump    ReadPump    ReadPump    ← goroutine each

Client.send channel: 256-buffered
Ping interval: 54s (= 9/10 × pongWait 60s)
Read limit: 4096 bytes
```

**Slow client handling (current):** if `Client.send` is full, the hub spawns a goroutine to
unregister. This creates an unbounded goroutine pool under load — see issue #25.

---

### 2.7 Concurrency Model

| Component | Goroutines | Synchronisation | Context-aware |
|-----------|-----------|-----------------|---------------|
| Hub event loop | 1 (started in main) | channels | No (runs until process exits) |
| Per-client WritePump | 1 per connection | channel | No |
| Per-client ReadPump | 1 per connection (blocking) | none | No |
| Retention worker | 1 | ticker + ctx.Done | Yes |
| TicketStore cleanup | 1 | mutex + ticker + ctx.Done | Yes |
| Rate limiter cleanup | 1 per route | mutex + ticker + ctx.Done | Yes |
| Webhook delivery | 1 per notification (unbounded) | none | No — issue #26 |

**Root context:** A single `rootCtx` is created in `main()` and cancelled before the HTTP server
shuts down. All background goroutines that accept a context stop cleanly on `rootCtx.Done()`.

---

### 2.8 Storage Layer

#### Port / Adapter pattern

```
handlers / middleware
        │
        │ depend on interfaces only
        ▼
  storage/port.go
  ┌─────────────────────────────────────────┐
  │ UserRepository                          │
  │ AppRepository                           │
  │ NotificationRepository                  │
  │ NotificationPublisher                   │
  └─────────────────────────────────────────┘
        │                     │
        │ implemented by       │ implemented by
        ▼                     ▼
  storage/sqlite/         (future)
  user.go                 storage/postgres/
  app.go                  storage/redis/ (cache)
  notification.go         storage/kafka/ (publisher)
```

#### SQLite configuration

- WAL (Write-Ahead Log) mode — allows concurrent readers with one writer
- Foreign keys enforced via `PRAGMA foreign_keys=ON`
- Schema managed by GORM `AutoMigrate` (development); production migrations should use
  `golang-migrate` or Atlas
- Single file: configurable via `DATABASE_PATH` env var

---

## 3. API Reference

### Public Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/auth/login` | None | Authenticate; returns JWT |
| POST | `/auth/register` | None | Register new user (if enabled) |
| GET | `/health` | None | Liveness check; pings database |

### Application Token Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/message` | App token | Send a notification |

### User API (JWT required)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/notification` | List notifications (paginated, filterable) |
| GET | `/api/v1/notification/:id` | Get single notification |
| PUT | `/api/v1/notification/:id/read` | Mark as read |
| DELETE | `/api/v1/notification/:id` | Soft-delete notification |
| DELETE | `/api/v1/notification` | Soft-delete all notifications |
| GET | `/api/v1/application` | List apps |
| POST | `/api/v1/application` | Create app (returns plaintext token once) |
| PUT | `/api/v1/application/:id` | Update app metadata |
| DELETE | `/api/v1/application/:id` | Delete app + its notifications |
| POST | `/api/v1/application/:id/token` | Rotate app token |
| GET | `/api/v1/ws/ticket` | Issue 30-second WebSocket ticket |
| GET | `/ws?ticket=<t>` | Open WebSocket connection |

### Admin API (JWT + is_admin required)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/user` | List all users |
| POST | `/api/v1/user` | Create user |
| DELETE | `/api/v1/user/:id` | Delete user + all their apps and notifications |
| PUT | `/api/v1/user/:id/password` | Reset user password |

### Notification Filter Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `limit` | int (1–100, def 20) | Page size |
| `offset` | int (def 0) | Page offset |
| `app_id` | string | Filter by application |
| `read` | bool string (`true`/`false`) | Filter by read status |
| `priority` | int (0–10) | Filter by exact priority |
| `q` | string | Full-text LIKE search on title and message |

---

## 4. Configuration

All configuration is via environment variables.

| Variable | Default | Description |
|----------|---------|-------------|
| `ENV` | `development` | `development` (colored logs) or `production` (JSON logs) |
| `LISTEN_ADDR` | `:8080` | HTTP listen address |
| `DATABASE_PATH` | `notifications.db` | SQLite file path |
| `JWT_SECRET` | *(must set in prod)* | HMAC secret for JWT signing |
| `JWT_EXPIRY_HOURS` | `24` | JWT lifetime in hours |
| `DEFAULT_ADMIN_USER` | `admin` | Username for auto-created admin |
| `DEFAULT_ADMIN_PASS` | `admin` | Password for auto-created admin |
| `ALLOW_REGISTRATION` | `true` | Allow open user self-registration |
| `RETENTION_DAYS` | `30` | Auto-delete notifications older than N days (0 = off) |
| `ALLOWED_ORIGINS` | `*` | Comma-separated CORS origins (`*` = allow all) |

**Startup validation:** `config.Validate()` is called before the server starts. In `production`
mode, the server refuses to start if `JWT_SECRET` is the default placeholder value.

---

## 5. Dependency Graph

```
main
 ├── config
 ├── logger  (zap)
 ├── db
 │    ├── gorm / sqlite driver
 │    └── models
 ├── storage/sqlite
 │    ├── storage/port.go  (interfaces)
 │    └── models
 ├── hub
 │    ├── gorilla/websocket
 │    └── logger
 ├── router
 │    ├── handlers
 │    │    ├── storage/port.go
 │    │    ├── auth
 │    │    ├── hub
 │    │    └── models
 │    ├── middleware
 │    │    ├── storage/port.go
 │    │    ├── auth
 │    │    └── config
 │    └── gin
 └── http.Server
```

**Key invariant:** `handlers` and `middleware` import `storage/port.go` (interfaces) only.
They never import `storage/sqlite`, `gorm`, or any concrete driver.

---

## 6. Design Decisions

### Why Go?
Strong concurrency primitives (goroutines, channels) map naturally to the fan-out WebSocket
model. A single binary with no runtime dependency simplifies deployment. CGO is required only
for the SQLite driver.

### Why Gin?
Lightweight HTTP framework with good middleware composability, fast routing, and broad ecosystem
familiarity. Swappable to stdlib `net/http` + a router (e.g. chi) if desired — the repository
pattern means no business logic touches gin types.

### Why SQLite (now) / Postgres (future)?
SQLite requires zero infrastructure for development and small deployments. The port/adapter
pattern (storage/port.go) makes swapping to Postgres a matter of adding `storage/postgres/`
without touching any handler or service code.

### Why SHA-256 for app tokens (not bcrypt)?
App tokens are 32 random bytes (64-char hex) — cryptographically high-entropy. SHA-256 is
appropriate because there is no weak human-chosen password to protect against dictionary attacks.
bcrypt's slowness would add latency on every inbound notification with no security benefit.

### Why prefix index on app tokens?
`token_prefix` (first 8 chars) is indexed so the database can narrow from millions of rows to
~1 before comparing the full SHA-256 hash. Without it, every `POST /message` would require a
full table scan.

### Why WebSocket tickets?
Passing a long-lived JWT as a URL query param exposes it in browser history, proxy logs, and
server access logs. A 30-second opaque ticket prevents token leakage at the cost of one extra
round trip before the WebSocket upgrade.

### Why port/adapter over direct GORM in handlers?
- Database can be swapped (SQLite → Postgres → DynamoDB) without touching handlers
- Handlers are unit-testable with in-memory fakes (no database required)
- `NotificationPublisher` interface lets Hub be swapped for Kafka without handler changes
- Compile-time enforcement: missing `context.Context` on a repo call is a build error

---

## 7. Known Limitations

| # | Limitation | Severity | Planned fix |
|---|-----------|----------|-------------|
| L1 | Hub slow-client eviction spawns unbounded goroutines | Medium | Phase 2 (#25) |
| L2 | Webhook delivery is unbounded raw goroutines | Medium | Phase 2 (#26) |
| L3 | No service layer — business logic in handlers | Medium | Phase 1 (#21) |
| L4 | Auth has no interface — no swap point for OIDC | Medium | Phase 3 (#27) |
| L5 | No JWT refresh tokens or blacklist | Medium | Phase 3 |
| L6 | Rate limiter is per-process — wrong across replicas | Medium | Phase 5 (#32) |
| L7 | Logger is global singleton — not injected | Low | Phase 1 (#29) |
| L8 | GORM AutoMigrate not suitable for production | Low | Phase 4 |
| L9 | No pagination cursor — large offsets are slow | Low | Phase 1 |
| L10 | Webhook timeout hard-coded at 10s | Low | Phase 2 |

---

## 8. Roadmap

### Phase 1 — Service Layer & DI (Issue #21)

**Goal:** Extract business logic from handlers into typed service structs. Handlers become pure
transport adapters (parse request → call service → serialise response).

```
service/
  notification.go   ← NotificationService{repo, publisher, apps}
  app.go            ← AppService{repo}
  user.go           ← UserService{repo}
  auth.go           ← AuthService{users, cfg}
```

Benefits:
- Business logic is unit-testable without HTTP or database
- Logger injected as constructor argument (resolves #29)
- Clear boundary for adding features (pagination cursors, notification batching)

---

### Phase 2 — Concurrency & Reliability (Issues #25, #26)

**Goal:** Eliminate unbounded goroutine creation under load.

#### Hub eviction pool (#25)

Replace inline `go func() { h.unregister <- cl }(c)` with a bounded eviction channel:

```go
type Hub struct {
    evictQueue chan *Client  // bounded, e.g. 512
    ...
}
// Single evictWorker goroutine drains the queue
```

#### Webhook worker pool (#26)

```go
type WebhookDispatcher struct {
    queue   chan webhookJob  // configurable depth
    workers int             // configurable count
}
// workers goroutines drain the queue; queue full = logged drop
```

Config additions: `WEBHOOK_WORKERS` (default 4), `WEBHOOK_QUEUE_DEPTH` (default 256),
`WEBHOOK_TIMEOUT` (default 10s).

---

### Phase 3 — Auth Hardening (Issue #27)

**Goal:** Define auth interfaces so any auth backend can be swapped.

```go
// auth/port.go
type TokenIssuer interface {
    Issue(userID string, isAdmin bool) (string, error)
}
type TokenVerifier interface {
    Verify(token string) (*Claims, error)
}
```

**Planned auth improvements:**
- JWT refresh tokens (short-lived access + long-lived refresh pair)
- Token blacklist via Redis (logout / revocation)
- OIDC / OAuth2 verifier adapter (`auth/oidc/`) — drop-in via interface
- API key scopes (read-only, write-only per application)
- Webhook signature (HMAC-SHA256 header on outbound webhook calls)

---

### Phase 4 — Postgres (Issue #30)

**Goal:** Production-grade relational database with connection pooling and schema migrations.

```
storage/
  postgres/
    db.go              ← gorm.Open(postgres.Open(dsn)), pool config
    user.go
    app.go
    notification.go    ← partial indexes: WHERE deleted_at IS NULL
```

New config variables:
```
DATABASE_DRIVER=postgres
DATABASE_DSN=postgres://user:pass@host:5432/notifyq?sslmode=require
DATABASE_MAX_OPEN_CONNS=25
DATABASE_MAX_IDLE_CONNS=5
DATABASE_CONN_MAX_LIFETIME=5m
```

Driver selection in `main.go`:
```go
switch cfg.DatabaseDriver {
case "postgres":
    database, err = pgadapter.Open(cfg.DatabaseDSN, cfg.DBPool)
default:
    database, err = sqlite.Open(cfg.DatabasePath)
}
```

Migration tooling: replace `AutoMigrate` with `golang-migrate` or Atlas for production.

---

### Phase 5 — Redis (Issue #32)

**Goal:** Shared state across multiple server replicas.

**Part A — Distributed rate limiting:**

```go
// middleware/redis_rate_limit.go
func RedisRateLimit(limiter *redis_rate.Limiter, key string, rps int) gin.HandlerFunc
```

Replaces in-process `ipRateLimiter`. Works correctly when N instances run behind a load balancer.

**Part B — Notification cache:**

```
storage/
  redis/
    notification_cache.go  ← read-through / write-through wrapping NotificationRepository
```

Cache key: `notifs:user:<id>:<limit>:<offset>:<filter_hash>`
TTL: configurable via `REDIS_CACHE_TTL` (default 60s)
Invalidation: on `MarkRead`, `DeleteNotification`, `SendNotification`

New config variables:
```
REDIS_ADDR=redis:6379
REDIS_PASSWORD=
REDIS_DB=0
REDIS_CACHE_TTL=60s
```

---

### Phase 6 — Kafka (Issue #31)

**Goal:** Durable, replayable, multi-consumer event stream for notifications.

```
storage/
  kafka/
    publisher.go       ← implements NotificationPublisher via sarama

worker/
  kafka_consumer.go    ← consumer group → Hub fan-out
  webhook_consumer.go  ← consumer group → WebhookDispatcher
```

**Data flow:**

```
POST /message
  → NotificationService.Send()
  → NotificationRepository.Create()     (persist first)
  → KafkaPublisher.Publish()             (async, durable)
        │
        ├─ Consumer group A → Hub.Send() → WebSocket clients
        └─ Consumer group B → WebhookDispatcher.Enqueue()
```

Benefits:
- Notifications survive server restart (Kafka retains messages)
- Multiple consumer types (mobile push, email, Slack) without modifying the send path
- Back-pressure and replay built in

New config variables:
```
KAFKA_BROKERS=broker1:9092,broker2:9092
KAFKA_TOPIC=notifyq.notifications
KAFKA_GROUP_ID_WS=notifyq-ws-consumer
KAFKA_GROUP_ID_WEBHOOK=notifyq-webhook-consumer
```

---

### Phase 7 — Observability

**Goal:** Production-grade metrics, tracing, and structured logging.

| Tool | Purpose | Integration point |
|------|---------|-------------------|
| Prometheus | Request latency, notification throughput, WS connection count | Middleware wrapper + Hub metrics |
| OpenTelemetry | Distributed tracing across send → persist → deliver | Injected via context into service constructors |
| Grafana | Dashboards for latency, error rate, queue depth | Consumes Prometheus metrics |
| Sentry / Rollbar | Error capture and alerting | Replace `logger.L.Error` at service boundary |

**Structured log fields (already in place):**
- `request_id` — injected by `RequestID` middleware, propagated on every log line
- `method`, `path`, `status`, `latency`, `client_ip` — from `ZapLogger` middleware
- `user_id` — added in service layer (Phase 1)

---

### Phase 8 — Multi-tenancy & Enterprise

**Goal:** Support multiple isolated tenants in a single deployment.

- **Tenant isolation:** All models gain a `tenant_id` column; all queries scoped by tenant
- **Tenant-scoped rate limiting:** per-tenant quotas in addition to per-IP limits
- **SSO / SAML:** `auth/saml/` adapter satisfying `TokenVerifier` interface
- **Audit log:** immutable append-only table logging all admin actions
- **Notification categories and channels:** apps declare channel types; users subscribe selectively
- **Mobile push:** `NotificationPublisher` adapter for APNs/FCM
- **Notification templates:** parameterised templates stored per app

---

## 9. Target Architecture (v2)

The diagram below shows the fully realised architecture after all phases complete.

```
                          ┌─────────────────────────────────────────┐
                          │              Clients                    │
                          │   Browser  │  Mobile  │  CLI  │  SDK    │
                          └──────┬─────┴────┬─────┴───┬───┴────┬────┘
                                 │          │         │        │
                          ┌──────▼──────────▼─────────▼────────▼────┐
                          │           Load Balancer / API GW         │
                          └────────────────────┬──────────────────────┘
                                               │
                    ┌──────────────────────────▼──────────────────────────┐
                    │                  NotifyQ Instances (N)               │
                    │                                                      │
                    │  ┌──────────┐  ┌──────────┐  ┌───────────────────┐  │
                    │  │ Gin HTTP │  │   Auth   │  │  WebSocket Hub    │  │
                    │  │ handlers │  │ Service  │  │  (per instance)   │  │
                    │  └────┬─────┘  └────┬─────┘  └────────┬──────────┘  │
                    │       │             │                  │             │
                    │  ┌────▼─────────────▼──────────────────▼──────────┐  │
                    │  │              Service Layer                      │  │
                    │  │  NotificationService  AppService  UserService   │  │
                    │  └────┬────────────────────────────────────────────┘  │
                    │       │                                               │
                    │  ┌────▼──────────────────────────────────────────┐   │
                    │  │           storage/port.go  (interfaces)        │   │
                    │  └────┬───────────┬──────────────┬───────────────┘   │
                    └───────┼───────────┼──────────────┼───────────────────┘
                            │           │              │
                   ┌────────▼──┐  ┌─────▼──────┐  ┌───▼──────────────┐
                   │  Postgres  │  │   Redis    │  │      Kafka        │
                   │ (primary + │  │ (cache +   │  │ (notification     │
                   │  replicas) │  │ rate limit)│  │  event stream)    │
                   └────────────┘  └────────────┘  └──────────┬────────┘
                                                               │
                                              ┌────────────────┴──────────────┐
                                              │        Kafka Consumers         │
                                              ├─ WS fan-out consumer           │
                                              ├─ Webhook dispatcher consumer   │
                                              ├─ Mobile push consumer (APNs/FCM│
                                              └───────────────────────────────┘
```

**Key properties of v2:**
- Horizontally scalable: N stateless HTTP instances behind a load balancer
- Shared rate limiting via Redis (not per-process)
- Durable notification delivery via Kafka (survives instance restart)
- Database read replicas via Postgres connection pool routing
- Redis cache absorbs hot notification list reads

---

## 10. Deployment

### Current (single binary)

```bash
# Development
make run

# Docker
docker build -t notifyq .
docker run -p 8080:8080 -v notifyq-data:/data \
  -e JWT_SECRET=your-secret \
  -e ENV=production \
  notifyq
```

### Docker Compose (Phase 4+)

```yaml
services:
  notifyq:
    image: notifyq
    environment:
      ENV: production
      DATABASE_DRIVER: postgres
      DATABASE_DSN: postgres://notifyq:pass@postgres:5432/notifyq?sslmode=require
      REDIS_ADDR: redis:6379
      JWT_SECRET: ${JWT_SECRET}
    depends_on: [postgres, redis]

  postgres:
    image: postgres:16-alpine
    volumes: [pg-data:/var/lib/postgresql/data]

  redis:
    image: redis:7-alpine
```

### Kubernetes (Phase 6+)

```yaml
# Deployment: 3 replicas, HPA on CPU + custom metric (notification queue depth)
# Services: ClusterIP for internal, Ingress for external
# ConfigMap: non-secret config
# Secret: JWT_SECRET, DATABASE_DSN, REDIS_PASSWORD, KAFKA credentials
# PodDisruptionBudget: min 2 replicas available during rolling update
```

---

*This document reflects the architecture as of the current codebase. Update alongside significant
structural changes.*
