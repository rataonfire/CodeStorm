---
name: project-codestorm-overview
description: CodeStorm project — real-time multi-party payment reconciliation system for Uzbek fintech hackathon
metadata:
  type: project
---

Real-time multi-party payment reconciliation platform. 48-hour hackathon project.

**Stack:**
- Scenario Director + 3 Emulators (Go) → Redis Pub/Sub `scenario.tx`
- Ingestor (Go/Fiber, port 8080) → Postgres `events` + Redis Stream `transactions:raw`
- Reconciler/Matcher (Rust/tokio, port 8081) → reads stream, writes Postgres `transactions`/`incidents`/`reconciliation_details`, publishes `reconciliation.events`
- API Gateway (Go/Fiber, port 8090) → reads Postgres, proxies Redis Pub/Sub to WebSocket
- Single Postgres DB: `recon` user/password/db

**Key invariants (§2.3 of Project_Bible.md):**
- I1: `gateway.amount == merchant.amount`
- I2: `bank.amount == gateway.amount - gateway.fee`
- I3: all currencies equal

**Project structure:**
- `/services/` — Go services (scenario, emulators, ingestor)
- `/matcher/` — Rust reconciler
- `/api-gateway/` — Go API gateway
- `/tests/integration/` — Go integration tests
- `docker-compose.yml` — unified compose at project root

**Why:** Production-grade pattern for reconciliation; Rust matcher for sub-ms latency; Redis Streams consumer groups; pluggable identity resolution.
