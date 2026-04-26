# NotifyQ

> Self-hosted push notification server — send from any service via HTTP, receive in real time over WebSockets or the built-in web dashboard.

![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-green)
![Docker](https://img.shields.io/badge/Docker-ready-2496ED?logo=docker&logoColor=white)
![SQLite](https://img.shields.io/badge/Database-SQLite-003B57?logo=sqlite&logoColor=white)

![NotifyQ dashboard showing the Applications panel](assets/image.png)

---

## Table of Contents

- [Features](#features)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [API Reference](#api-reference)
  - [Authentication](#authentication)
  - [Sending Notifications](#sending-notifications)
  - [Notifications](#notifications)
  - [Applications](#applications)
  - [Users (Admin)](#users-admin-only)
  - [WebSocket Stream](#websocket-stream)
- [Project Structure](#project-structure)
- [Documentation](#documentation)
- [Development](#development)
- [Contributing](#contributing)
- [Security](#security)
- [License](#license)

---

## Features

| Capability | Details |
| ---------- | ------- |
| **REST API** | Send, list, filter, delete, and mark notifications as read |
| **Real-time delivery** | WebSocket stream with per-user broadcast and ping/pong keep-alive |
| **App tokens** | Each application gets an independent 64-char hex token; stored as SHA-256 |
| **JWT authentication** | HS256 Bearer tokens with configurable expiry |
| **WebSocket tickets** | Short-lived 30 s opaque tickets prevent JWTs from appearing in logs |
| **Multi-user** | Admin panel for full user management |
| **Webhooks** | Per-application outbound webhook on every notification |
| **Notification retention** | Configurable auto-purge of old notifications (background worker) |
| **Rate limiting** | Per-IP token-bucket rate limiting on auth and message endpoints |
| **Structured logging** | Zap — colored console in development, JSON in production |
| **Web dashboard** | Embedded single-page UI (no separate server required) |
| **SQLite backend** | Zero external dependencies; single binary deployment |
| **Docker ready** | Multi-stage production Dockerfile included |

---

## Quick Start

### Prerequisites

- **Go 1.25+** with CGO enabled (`gcc` or equivalent C toolchain required for SQLite)
- **Or Docker** — no local toolchain needed

### Run locally

```bash
git clone https://github.com/deannos/notification-queue.git
cd notification-queue

# Copy and configure environment
cp .env.example .env

make run
```

Open [http://localhost:8080](http://localhost:8080) and sign in with the default credentials:

| Username | Password |
| -------- | -------- |
| `admin`  | `admin`  |

> **Before exposing to the internet:** change the default admin password and set a strong `JWT_SECRET` in your `.env` file.

### Run with Docker

```bash
docker build -t notifyq .

docker run -d \
  -p 8080:8080 \
  -v notifyq-data:/data \
  -e JWT_SECRET=your-secret-here \
  -e DEFAULT_ADMIN_PASS=strongpassword \
  -e ENV=production \
  --name notifyq \
  notifyq
```

---

## Configuration

All settings are provided via environment variables. Copy `.env.example` to `.env` for local development.

| Variable | Default | Description |
| -------- | ------- | ----------- |
| `ENV` | `development` | Runtime mode: `development` (colored logs) or `production` (JSON logs) |
| `LISTEN_ADDR` | `:8080` | Address and port to listen on |
| `DATABASE_PATH` | `notifications.db` | Path to the SQLite database file |
| `JWT_SECRET` | *(required in production)* | HMAC-SHA256 secret used to sign JWT tokens |
| `JWT_EXPIRY_HOURS` | `24` | Token validity in hours |
| `DEFAULT_ADMIN_USER` | `admin` | Admin username created on first run |
| `DEFAULT_ADMIN_PASS` | `admin` | Admin password created on first run |
| `ALLOW_REGISTRATION` | `true` | Allow public self-registration |
| `RETENTION_DAYS` | `30` | Auto-delete notifications older than N days (`0` to disable) |
| `ALLOWED_ORIGINS` | `*` | Comma-separated CORS allowed origins |

**Production note:** the server will refuse to start in `production` mode if `JWT_SECRET` is the default placeholder value.

---

## API Reference

### Authentication

All protected endpoints require a JWT Bearer token obtained from `/auth/login`. Admin endpoints additionally require the `is_admin` claim.

**Login**

```http
POST /auth/login
Content-Type: application/json

{"username": "admin", "password": "admin"}
```

Returns:

```json
{"token": "<jwt>", "expires_in": 86400}
```

Include the token in subsequent requests:

```
Authorization: Bearer <jwt>
```

**Register** *(if `ALLOW_REGISTRATION=true`)*

```http
POST /auth/register
Content-Type: application/json

{"username": "alice", "password": "s3cur3pass"}
```

---

### Sending Notifications

Create an application in the dashboard to obtain an app token, then:

```http
POST /message
X-App-Token: <your-app-token>
Content-Type: application/json

{
  "title": "Deploy successful",
  "message": "v1.2.3 is live on production",
  "priority": 7
}
```

**Priority scale:** `1` (lowest) → `10` (critical) · Default: `5`

**curl example:**

```bash
curl -s -X POST http://localhost:8080/message \
  -H "X-App-Token: your-app-token" \
  -H "Content-Type: application/json" \
  -d '{"title":"Hello","message":"World","priority":5}'
```

---

### Notifications

All endpoints require `Authorization: Bearer <jwt>`.

| Method | Endpoint | Description |
| ------ | -------- | ----------- |
| `GET` | `/api/v1/notification` | List notifications (paginated, filterable) |
| `GET` | `/api/v1/notification/:id` | Get a single notification |
| `PUT` | `/api/v1/notification/:id/read` | Mark as read |
| `DELETE` | `/api/v1/notification/:id` | Delete a notification |
| `DELETE` | `/api/v1/notification` | Delete all notifications |

**Filter parameters for `GET /api/v1/notification`:**

| Parameter | Type | Default | Description |
| --------- | ---- | ------- | ----------- |
| `limit` | integer (1–100) | `20` | Page size |
| `offset` | integer | `0` | Page offset |
| `app_id` | string | — | Filter by application ID |
| `read` | `true` / `false` | — | Filter by read status |
| `priority` | integer (0–10) | — | Filter by exact priority |
| `q` | string | — | Full-text search on title and message |

---

### Applications

All endpoints require `Authorization: Bearer <jwt>`.

| Method | Endpoint | Description |
| ------ | -------- | ----------- |
| `GET` | `/api/v1/application` | List your applications |
| `POST` | `/api/v1/application` | Create an application — returns the plaintext token once |
| `PUT` | `/api/v1/application/:id` | Update name or description |
| `DELETE` | `/api/v1/application/:id` | Delete application and all its notifications |
| `POST` | `/api/v1/application/:id/token` | Rotate the app token |

---

### Users (Admin only)

All endpoints require `Authorization: Bearer <jwt>` with admin privileges.

| Method | Endpoint | Description |
| ------ | -------- | ----------- |
| `GET` | `/api/v1/user` | List all users |
| `POST` | `/api/v1/user` | Create a user |
| `DELETE` | `/api/v1/user/:id` | Delete a user and all their data |
| `PUT` | `/api/v1/user/:id/password` | Reset a user's password |

---

### WebSocket Stream

**Recommended flow — ticket auth:**

```bash
# 1. Obtain a short-lived ticket (requires valid JWT)
GET /api/v1/ws/ticket
Authorization: Bearer <jwt>

# Returns: {"ticket": "<32-byte hex>"}

# 2. Open WebSocket (ticket valid for 30 s)
GET /ws?ticket=<ticket>
```

**Alternative — direct JWT** *(exposes token in URL)*:

```
GET /ws?token=<jwt>
```

Each message is a JSON object:

```json
{
  "event": "notification",
  "notification": {
    "id": "...",
    "title": "Deploy successful",
    "message": "v1.2.3 is live",
    "priority": 7,
    "read": false,
    "created_at": "2026-03-31T17:00:00Z",
    "app": { "id": "...", "name": "CI Pipeline" }
  }
}
```

**JavaScript example:**

```js
// Obtain a ticket first, then connect
const { ticket } = await fetch('/api/v1/ws/ticket', {
  headers: { Authorization: `Bearer ${jwtToken}` },
}).then(r => r.json());

const ws = new WebSocket(`ws://localhost:8080/ws?ticket=${ticket}`);
ws.onmessage = (e) => {
  const { event, notification } = JSON.parse(e.data);
  if (event === 'notification') console.log(notification.title);
};
```

---

## Project Structure

```
.
├── main.go             # Composition root — wire deps, start server
├── config/             # Environment-based configuration and startup validation
├── db/                 # GORM + SQLite init, WAL mode, retention worker
├── models/             # User, App, Notification GORM structs
├── storage/            # Repository interfaces (port.go) and SQLite adapters
├── auth/               # JWT generation/parsing, app token generation and hashing
├── hub/                # Per-user WebSocket broadcast hub and WS ticket store
├── middleware/         # JWT auth, app token auth, admin guard, rate limiter, logger
├── handlers/           # HTTP request handlers
├── router/             # Gin route registration and middleware chains
├── logger/             # Zap logger initialization (dev: console, prod: JSON)
├── ui/                 # React + TypeScript frontend source (Vite)
├── web/                # Embedded static assets (built from ui/)
├── docs/               # Architecture and improvement documentation
├── Dockerfile
└── Makefile
```

---

## Documentation

| Document | Description |
| -------- | ----------- |
| [docs/architecture.md](docs/architecture.md) | Full architecture reference: package responsibilities, data models, request lifecycle, authentication model, WebSocket design, concurrency model, storage layer (port/adapter pattern), design decisions, known limitations, and the phased roadmap to v2 (service layer → Postgres → Redis → Kafka). |
| [docs/enhancementv1.md](docs/enhancementv1.md) | Issue-level improvement catalogue: ten concrete problems in the current codebase (missing repository pattern, no DI, goroutine leaks, missing context propagation, non-atomic deletes, etc.) with before/after code examples and a prioritized implementation plan. |

---

## Development

```bash
# Build the UI and start the development server
make run

# Build the UI and compile a production binary (outputs ./notifyq)
make build

# Start the Vite dev server with HMR (requires a running Go server)
make ui-dev

# Remove build artifacts and database
make clean
```

**CGO is required.** The SQLite driver links against `libsqlite3`. Ensure `gcc` (or `clang` on macOS) is available before building.

### Running tests

No automated tests exist yet. When adding them:

- Use `testing` + `httptest` for handler tests
- Use in-memory SQLite (`:memory:`) for repository tests
- Run with `go test ./...`

---

## Contributing

1. Fork the repository and create a feature branch (`git checkout -b feature/your-feature`).
2. Make your changes, ensuring the project builds with `make build`.
3. Open a pull request against `main` with a clear description of the change and its motivation.

Please keep pull requests focused — one feature or fix per PR. For significant changes, open an issue first to discuss the approach.

---

## Security

**Hardening checklist before production deployment:**

- Set `JWT_SECRET` to a cryptographically random value (at minimum 32 bytes).
- Change `DEFAULT_ADMIN_PASS` from the default.
- Set `ENV=production` to enable JSON logging and startup validation.
- Set `ALLOW_REGISTRATION=false` if open self-registration is not required.
- Restrict `ALLOWED_ORIGINS` to your actual frontend origin instead of `*`.
- Run behind a reverse proxy (Nginx, Caddy, Traefik) that handles TLS termination.

To report a security vulnerability, please open a private security advisory on GitHub rather than a public issue.

---

## License

[MIT](LICENSE)
