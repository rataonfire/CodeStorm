# Chinor — Real-Time Payment Reconciliation Platform

Chinor is a high-performance fintech platform designed for the Uzbekistan market. it provides instant reconciliation of digital payments between Merchants, Payment Gateways (Click, Payme), and Banks (NBU, Kapitalbank).

## Key Features

- **Real-Time Matching:** Rust-based in-memory matching engine with < 2ms processing latency.
- **Automated Incident Detection:** Instantly identifies amount mismatches, fee discrepancies, missing sources, and duplicate transactions.
- **Visual Monitoring:** Live dashboard showing transaction flow, incident spikes, and matcher performance.
- **Scalable Architecture:** Built with Go, Rust, Redis, and PostgreSQL for high throughput (1000+ events/sec).
- **Security:** HMAC-signed events and idempotent processing.

## Tech Stack

- **Core Engine:** Rust (High-performance matching logic)
- **API & Ingestor:** Go (Fiber framework)
- **Frontend:** React + Vite (Vanilla CSS)
- **Storage:** PostgreSQL (Permanent records), Redis (Real-time streams & state)
- **Deployment:** Docker & Docker Compose

## Getting Started

### Prerequisites

- Docker and Docker Compose installed.

### Installation & Run

1. Clone the repository:
   ```bash
   git clone <repository-url>
   cd CodeStorm
   ```

2. Start the entire system:
   ```bash
   docker-compose up -d --build
   ```

3. Access the components:
   - **Dashboard (UI):** [http://localhost:3000](http://localhost:3000)
   - **API Gateway:** [http://localhost:8090](http://localhost:8090)
   - **Ingestor API:** [http://localhost:8080](http://localhost:8080)

## System Architecture

The platform follows a distributed event-driven architecture:

1. **Scenario Generator:** Simulates thousands of payment events.
2. **Emulators:** Mock Merchants, Gateways, and Banks sending JSON payloads to the Ingestor.
3. **Ingestor (Go):** Receives raw events, persists them to Postgres, and pushes them to Redis Streams.
4. **Reconciler (Rust):** Consumes the Redis Stream, performs in-memory matching, and detects discrepancies.
5. **API Gateway (Go):** Provides REST and WebSocket endpoints for the frontend.
6. **Frontend (React):** Real-time monitoring and incident management interface.

## Business Impact

- **3–4%** of digital payments fail or result in discrepancies that are often never resolved.
- Companies typically spend **5–20 hours per week** on manual reconciliation.
- Chinor reduces detection time from **24 hours to < 10 seconds**.

## License

© 2026 Chinor Uzbekistan. All rights reserved.
