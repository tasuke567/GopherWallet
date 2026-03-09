# 🐹 GopherWallet: High-Concurrency Transaction Engine

A production-grade wallet and transaction engine built with **Go**, designed to handle thousands of concurrent transfers with ACID guarantees. Built to demonstrate real-world backend engineering skills for fintech/banking systems.

## Architecture Overview

```
┌────────────────┐     ┌──────────┐     ┌────────────┐
│   Fiber HTTP   │────▶│  Service  │────▶│ PostgreSQL │
│   (API Layer)  │     │  (Logic)  │     │  (ACID TX) │
└────────────────┘     └─────┬─────┘     └────────────┘
        │                    │
        │              ┌─────▼─────┐     ┌────────────┐
        │              │   NATS    │────▶│Notification│
        │              │ (Events)  │     │  (Worker)  │
        │              └───────────┘     └────────────┘
        │
   ┌────▼────┐
   │  Redis  │ (Cache + Idempotency)
   └─────────┘
```

## Key Features

| Feature | Description |
|---|---|
| **Database Transactions** | `BEGIN → SELECT FOR UPDATE → DEBIT → CREDIT → COMMIT` with automatic `ROLLBACK` on failure |
| **Deadlock Prevention** | Accounts locked in consistent order (smaller ID first) |
| **Idempotency** | Dual-layer protection: Redis middleware + DB unique constraint |
| **Event-Driven** | NATS pub/sub for async notifications after successful transfers |
| **Balance Caching** | Redis cache with invalidation-on-write to reduce DB load |
| **Observability** | Prometheus metrics + Grafana dashboards |
| **Graceful Shutdown** | Signal handling with proper cleanup of all connections |

## Tech Stack

| Component | Technology |
|---|---|
| Language | Go 1.22+ |
| HTTP Framework | Fiber v2 |
| Database | PostgreSQL 16 (pgx driver) |
| Cache | Redis 7 |
| Message Broker | NATS 2.10 |
| Monitoring | Prometheus + Grafana |
| Container | Docker + Docker Compose |
| CI/CD | GitHub Actions |

## Project Structure

```
.
├── cmd/
│   └── api/
│       └── main.go              # Entry point & dependency wiring
├── internal/
│   ├── domain/                   # Core models & interfaces (no external deps)
│   │   ├── account.go
│   │   ├── transaction.go
│   │   ├── tx.go                 # Transaction manager interface
│   │   └── errors.go
│   ├── wallet/                   # Business logic
│   │   ├── service.go            # Transfer logic with DB transactions
│   │   ├── repository.go         # PostgreSQL implementation
│   │   ├── handler.go            # HTTP handlers
│   │   ├── cache.go              # Redis caching layer
│   │   └── service_test.go       # Unit tests with mocks
│   ├── event/                    # Event definitions & interfaces
│   │   ├── events.go
│   │   └── broker.go
│   ├── middleware/               # HTTP middleware
│   │   ├── idempotency.go        # Redis-based duplicate prevention
│   │   └── prometheus.go         # Request metrics
│   └── notification/             # Event consumer worker
│       └── worker.go
├── pkg/
│   ├── config/                   # Environment configuration
│   ├── database/                 # PostgreSQL connection pool
│   └── messaging/                # NATS client wrapper
├── migrations/                   # SQL migration files
├── .github/workflows/ci.yml     # CI/CD pipeline
├── docker-compose.yml            # Full stack (Postgres, Redis, NATS, Prometheus, Grafana)
├── Dockerfile                    # Multi-stage build
└── prometheus.yml                # Metrics scrape config
```

## Quick Start

### Run with Docker Compose (recommended)

```bash
docker compose up --build
```

This starts all services:
- **API** → http://localhost:8080
- **Prometheus** → http://localhost:9090
- **Grafana** → http://localhost:3000 (admin/admin)
- **NATS Monitor** → http://localhost:8222

### API Endpoints

```bash
# Health check
curl http://localhost:8080/health

# Create accounts
curl -X POST http://localhost:8080/api/v1/accounts \
  -H "Content-Type: application/json" \
  -d '{"user_id": "user-001", "balance": 1000000, "currency": "THB"}'

curl -X POST http://localhost:8080/api/v1/accounts \
  -H "Content-Type: application/json" \
  -d '{"user_id": "user-002", "balance": 500000, "currency": "THB"}'

# Transfer money (with idempotency key)
curl -X POST http://localhost:8080/api/v1/transfers \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: txn-unique-001" \
  -d '{"from_account_id": 1, "to_account_id": 2, "amount": 50000}'

# Get account balance
curl http://localhost:8080/api/v1/accounts/1

# Prometheus metrics
curl http://localhost:8080/metrics
```

## How the Transfer Works (Interview Talking Points)

### 1. Race Condition Prevention
```
Client A (transfer 100) ──┐
                           ├──▶ SELECT ... FOR UPDATE (locks row)
Client B (transfer 200) ──┘    Client B waits until A commits

Timeline:
  A: BEGIN → LOCK(account) → UPDATE balance → COMMIT
  B: ............WAITING.............. → LOCK → UPDATE → COMMIT
```

### 2. Deadlock Prevention
```go
// Always lock accounts in ascending ID order
firstID, secondID := fromID, toID
if firstID > secondID {
    firstID, secondID = secondID, firstID
}
// Lock firstID, then secondID → consistent order → no deadlocks
```

### 3. Idempotency (Prevent Double Transfers)
```
Request 1 (key: "txn-001") → Redis SET NX → ✅ Proceed → Transfer → 201 Created
Request 2 (key: "txn-001") → Redis SET NX → ❌ Key exists → 409 Conflict
```

### 4. Event-Driven Architecture
```
Transfer Success → Publish to NATS "wallet.transfer.completed"
                        │
                        ├──▶ Notification Worker (send email/SMS)
                        ├──▶ Analytics Worker (track metrics)
                        └──▶ Audit Worker (compliance log)
```

## Running Tests

```bash
# Unit tests
go test ./... -v

# With race detector
go test ./... -race

# With coverage
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## License

MIT
