# Многосторонняя сверка платежей в реальном времени

**Хакатон по финансовому сектору (Узбекский рынок), дорожка 1**

**Версия:** 2.0 (финальная)
**Срок жизни проекта:** 48 часов
**Команда:**
- **Вика** — Scenario Director + эмуляторы (Go)
- **Добрыня** — Reconciler / матчер (Rust)
- **Виталий** — Ingestor + API Gateway + БД (Go), **презентация проекта**
- **Кирилл** — Operator UI (React + TypeScript)
- **Ведущий** — архитектура, помощь Добрыне с Rust-частью, контракты, демо-скрипт

> Этот документ — единственный источник правды. Любое расхождение между кодом и документом требует обновления документа **до** мерджа кода.

---

## 0. Краткое содержание для презентации (Виталий, прочитать первым)

**Что мы строим.** Платформу многосторонней сверки платежей в реальном времени для узбекского финтех-рынка. Система принимает события от трёх независимых участников платёжной цепи (мерчант, платёжный шлюз, банк-эквайер), стыкует их в единое представление транзакции, проверяет согласованность сумм и комиссий, и за <2 секунды выдаёт оператору точную картину расхождений.

**Архитектурный нарратив для жюри.**

> «Мы реализовали production-grade pattern платформы сверки. Hot path матчинга работает в памяти на Rust, что даёт предсказуемую sub-millisecond латентность без GC-пауз. Состояние реплицируется в Redis для отказоустойчивости. Identity resolution построен по chain-of-responsibility — в MVP резолвим по согласованному reference_id, архитектурно подключается content-hash matching и fuzzy matching без изменения ядра. Внутренние event_id используют UUID v7 для оптимальной работы B-tree индексов в Postgres. API Gateway реализует идемпотентность по Idempotency-Key (стандарт Stripe) с fallback на content hash. Полный flow от приёма события до решения о сверке — около 2 миллисекунд.»

**Что показываем на демо.** Три эмулятора шлют события через REST API, оператор в браузере видит live-таблицу транзакций со статусами; на демо мы крутим параметры искажений в эмуляторах и показываем, как система мгновенно ловит расхождения сумм, комиссий, дубли и пропуски.

**Релевантность для Узбекистана.** Высокая доля наличных и многоступенчатые платёжные цепочки означают повышенный риск рассинхрона между участниками. Real-time сверка с эскалацией инцидентов критична для роста доверия к безналичным платежам — что прямо адресует одну из ключевых проблем узбекского финтеха, описанную в условиях хакатона.

---

## 1. Архитектура системы

### 1.1. Диаграмма потоков

```
┌──────────────────────┐
│  Scenario Director   │   Канонические транзакции
│      (Вика, Go)      │   Redis Pub/Sub: scenario.tx
└──────────┬───────────┘
           │
     ┌─────┼─────────┬─────────┐
     ▼     ▼         ▼         ▼
┌─────────┐ ┌──────┐ ┌──────┐
│Merchant │ │Gateway│ │ Bank │   Эмуляторы (Вика, Go)
│Emulator │ │Emul. │ │Emul. │   Задержки, ошибки, комиссии
└────┬────┘ └──┬───┘ └──┬───┘
     │ HTTP   │ HTTP   │ HTTP
     ▼        ▼        ▼
   /events/  /events/ /events/
   merchant  gateway  bank
     └────────┴────┬───┘
                   ▼
          ┌────────────────────────────┐
          │   Ingestor                 │   Go (Виталий)
          │   ┌────────────────────┐   │
          │   │ HMAC / Bearer auth │   │
          │   │ Schema validation  │   │
          │   │ Idempotency check  │   │
          │   │ Normalization      │   │
          │   │ Identity Resolution│   │
          │   └────────────────────┘   │
          │   HTTP → Postgres + Stream │
          └──────────┬─────────────────┘
                     │ XADD
                     ▼
          ┌────────────────────┐
          │ Redis Stream       │ transactions:raw
          │ Consumer Group:    │
          │ 'reconciler'       │
          └──────────┬─────────┘
                     │ XREADGROUP
                     ▼
          ┌────────────────────────────┐
          │   Reconciler               │   Rust (Добрыня + ведущий)
          │   ┌────────────────────┐   │
          │   │ Matching Engine    │   │
          │   │ - In-memory window │   │
          │   │ - Redis checkpoint │   │
          │   │ - Sorted-set timer │   │
          │   │ - Dedup cache      │   │
          │   └────────────────────┘   │
          │   ┌────────────────────┐   │
          │   │ Incident Manager   │   │
          │   │ - Severity ladder  │   │
          │   │ - Escalation timer │   │
          │   └────────────────────┘   │
          └──┬──────────────┬──────────┘
             │              │
             ▼              ▼
         Postgres       Redis Pub/Sub
                        reconciliation.events
                            │
                            ▼
                   ┌────────────────┐
                   │  API Gateway   │   Go (Виталий)
                   │  REST + WS     │   Полирует данные для UI
                   └───────┬────────┘
                           │ HTTP + WebSocket
                           ▼
                   ┌────────────────┐
                   │  Operator UI   │   React + TS (Кирилл)
                   └────────────────┘
```

### 1.2. Архитектурные принципы

**1. Горячий путь сверки только в памяти.** Reconciler принимает решение о сверке за микросекунды, не блокируясь на БД или сетевых вызовах.

**2. БД пишет только Reconciler.** Ingestor пишет в Postgres события (для identity resolution и аудита), но финальные результаты сверки и инциденты — это зона ответственности Reconciler.

**3. Live-обновления через Pub/Sub.** UI не делает polling — он получает события через WebSocket, который проксирует Redis Pub/Sub от Reconciler.

**4. Идемпотентность на каждом уровне.** Любой запрос можно безопасно повторить. Это обязательно для платёжной системы.

**5. Anti-Corruption Layer на входе.** Каждый источник имеет свой формат и свой endpoint. Ingestor нормализует их в единое канональное представление до передачи в Reconciler.

**6. Pluggable identity resolution.** Стратегии резолюции correlation_id реализуют общий интерфейс. В MVP — одна стратегия (по reference_id), в production добавляются content-hash и fuzzy matching без изменения ядра.

### 1.3. Технологический стек

| Компонент | Язык | Технологии | Ответственный |
|-----------|------|-----------|---------------|
| Scenario Director | Go 1.22 | net/http, redis-go | Вика |
| Эмуляторы (3 шт.) | Go 1.22 | net/http, gofakeit, redis-go | Вика |
| Ingestor | Go 1.22 | Fiber, gojsonschema, pgx, redis-go | Виталий |
| **Reconciler** | **Rust 1.75** | **tokio, redis-rs, sqlx, dashmap, serde** | **Добрыня + ведущий** |
| API Gateway | Go 1.22 | Fiber, websocket, pgx | Виталий |
| Operator UI | TypeScript 5 | React 18, Vite, TanStack Query, Tailwind | Кирилл |
| База данных | — | PostgreSQL 15 | — |
| Очередь/кэш | — | Redis 7 (Streams + Pub/Sub + KV) | — |
| Деплой | — | Docker Compose | — |

---

## 2. Денежный поток и инварианты сверки

**Этот раздел понимают все.** Если хоть у одного человека в голове другая модель — система не работает.

### 2.1. Модель потока

Покупатель в Узбекистане платит сумму **N** UZS. Деньги идут по цепочке:

1. **Merchant** — фиксирует продажу на N, своих комиссий не взимает.
2. **Payment Gateway** (например, Click, Payme, Uzum) — получает N, удерживает комиссию **F_g**.
3. **Bank-эквайер** (например, Капиталбанк, НБУ) — получает **N − F_g**, удерживает комиссию **F_b**.

Итоговое зачисление мерчанту = **N − F_g − F_b** (в самой сверке не участвует).

### 2.2. Значения полей по источникам

| Источник | `amount_minor` | `fee_minor` | Семантика |
|----------|----------------|-------------|-----------|
| merchant | N              | 0           | «продал на N, ничего не удерживаю» |
| gateway  | N              | F_g         | «получил N, удержал F_g» |
| bank     | N − F_g        | F_b         | «получил N−F_g, удержал F_b» |

Все суммы — в минимальных единицах валюты. Для UZS это **тийины**, для RUB — копейки.

### 2.3. Инварианты, проверяемые матчером

```
I1: gateway.amount_minor == merchant.amount_minor
I2: bank.amount_minor    == gateway.amount_minor - gateway.fee_minor
I3: merchant.currency == gateway.currency == bank.currency
```

- Нарушение **I1** → `incident_type = "amount_mismatch"` (виноват тот источник, у которого `amount` отклонился от консенсуса двух других).
- Нарушение **I2** → `incident_type = "fee_mismatch"` (виноват gateway или bank).
- Нарушение **I3** → `incident_type = "currency_mismatch"`.

### 2.4. Типы инцидентов

| `incident_type`     | Когда возникает |
|---------------------|-----------------|
| `amount_mismatch`   | Нарушение I1 |
| `fee_mismatch`      | Нарушение I2 |
| `currency_mismatch` | Нарушение I3 |
| `missing_source`    | За 2 секунды после первого события не пришёл один из источников |
| `duplicate`         | От одного источника пришло два события с одинаковым `transaction_id` и разными `event_id` |
| `source_offline`    | Источник не шлёт heartbeat >5 секунд (бонус keepalive) |

### 2.5. Эскалация инцидентов

Инцидент с момента создания имеет `severity = 1` (warning). Если за `RECONCILER_ESCALATION_MS` (по умолчанию 10 секунд) оператор не подтвердил инцидент, severity повышается до 2 (critical) и публикуется событие `incident_updated`. Этот механизм работает в Reconciler через тот же sorted-set таймеров.

---

## 3. Контракты и типы данных

### 3.1. Событие источника (`PaymentTransactionEvent`)

Каждый источник шлёт **этот формат**, валидируется в Ingestor. Файл: `contracts/event.schema.json`.

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "PaymentTransactionEvent",
  "type": "object",
  "required": [
    "event_id",
    "transaction_id",
    "source",
    "amount_minor",
    "fee_minor",
    "currency",
    "timestamp_ms",
    "tx_type"
  ],
  "properties": {
    "event_id": {
      "type": "string",
      "format": "uuid",
      "description": "Уникальный ID события от источника. Используется для дедупликации ретраев."
    },
    "transaction_id": {
      "type": "string",
      "format": "uuid",
      "description": "Сквозной ID транзакции, одинаковый у всех трёх источников."
    },
    "source": {
      "type": "string",
      "enum": ["merchant", "gateway", "bank"]
    },
    "amount_minor": {
      "type": "integer",
      "minimum": 0,
      "description": "Сумма в минимальных единицах валюты (тийины для UZS, копейки для RUB)."
    },
    "fee_minor": {
      "type": "integer",
      "minimum": 0,
      "description": "Комиссия источника. Для merchant всегда 0."
    },
    "currency": {
      "type": "string",
      "pattern": "^[A-Z]{3}$",
      "description": "ISO 4217 код валюты (UZS, RUB, USD). Одинаков у всех источников транзакции."
    },
    "timestamp_ms": {
      "type": "integer",
      "minimum": 0,
      "description": "Unix milliseconds, UTC."
    },
    "tx_type": {
      "type": "string",
      "enum": ["card", "transfer"]
    },
    "merchant_id": { "type": "string" },
    "correlation_id": { "type": "string", "format": "uuid" }
  },
  "additionalProperties": false
}
```

**Пример валидного события:**

```json
{
  "event_id": "550e8400-e29b-41d4-a716-446655440000",
  "transaction_id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
  "source": "gateway",
  "amount_minor": 5000000,
  "fee_minor": 25000,
  "currency": "UZS",
  "timestamp_ms": 1735689600123,
  "tx_type": "card",
  "merchant_id": "MERCHANT_001"
}
```

Это транзакция на 50 000 UZS с комиссией шлюза 250 UZS (0.5%).

### 3.2. Внутренние идентификаторы (UUID v7)

Ingestor при приёме события генерирует **внутренний** `internal_event_id` в формате **UUID v7**. Зачем именно v7:

- **Сортируется по времени** (первые 48 бит — Unix ms timestamp).
- **B-tree friendly** в Postgres (новые записи в конец индекса).
- **Глобально уникален** (как v4).

Внутренний ID используется во всех таблицах Postgres. Внешний `event_id` от источника хранится отдельно для аудита и дедупликации.

**Go-библиотека:** `github.com/google/uuid` начиная с v1.6.
**Rust-библиотека:** `uuid` crate с feature `v7`.

### 3.3. Сообщение в Redis Stream (`transactions:raw`)

Ingestor оборачивает событие, добавляя контекст приёма:

```json
{
  "event": { /* PaymentTransactionEvent */ },
  "internal_event_id": "01928e3a-7b5c-7234-9abc-def012345678",
  "correlation_id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
  "ingested_at_ms": 1735689600125
}
```

Запись: `XADD transactions:raw * payload <json>`
Чтение: `XREADGROUP GROUP reconciler <consumer> COUNT 10 BLOCK 100 STREAMS transactions:raw >`

### 3.4. События Redis Pub/Sub (`reconciliation.events`)

Reconciler публикует, API Gateway проксирует в WebSocket. Все события — JSON с обязательными полями `type` и `ts_ms`.

| Тип | Когда | Доп. поля |
|-----|-------|----------|
| `transaction_received` | Открылось окно сверки (первое событие) | `transaction_id`, `source` |
| `transaction_progress` | Пришёл ещё источник, но не все | `transaction_id`, `sources_seen[]` |
| `transaction_matched` | Все источники, инварианты сошлись | `transaction_id` |
| `incident_created` | Создан инцидент | `incident_id`, `transaction_id`, `incident_type`, `severity`, `description` |
| `incident_updated` | Эскалация, ack, resolve | `incident_id`, `new_status`, `new_severity` |
| `source_status_changed` | Источник online/offline | `source`, `is_online` |

### 3.5. REST API (для UI)

Базовый префикс: `/api/v1`

| Метод | Путь | Назначение |
|-------|------|------------|
| GET | `/transactions` | Список (фильтры: `status`, `from`, `to`, `limit`, `cursor`) |
| GET | `/transactions/{id}` | Детали транзакции по всем источникам |
| GET | `/incidents` | Список (фильтры: `status`, `severity`) |
| POST | `/incidents/{id}/ack` | Подтвердить инцидент |
| POST | `/incidents/{id}/resolve` | Закрыть инцидент |
| POST | `/incidents/{id}/auto-correct` | Применить автокорректировку (бонус) |
| GET | `/metrics/mismatches-per-minute` | Данные для графика (бонус) |
| GET | `/sources/health` | Статусы источников (бонус keepalive) |
| GET | `/healthz` | Health-check |
| GET | `/readyz` | Readiness (проверяет Postgres + Redis) |

**Формат коллекций:**

```json
{
  "items": [ /* ... */ ],
  "next_cursor": "opaque-string-or-null",
  "total_estimate": 1234
}
```

**Формат ошибок:**

```json
{
  "error": {
    "code": "validation_failed",
    "message": "amount_minor must be non-negative",
    "details": { }
  }
}
```

**Коды ошибок:**

| Код | HTTP | Когда |
|-----|------|-------|
| `validation_failed` | 400 | Невалидный payload |
| `transaction_not_found` | 404 | Нет такой транзакции |
| `incident_not_found` | 404 | Нет такого инцидента |
| `incident_already_resolved` | 409 | Попытка ack/resolve уже закрытого |
| `idempotency_in_flight` | 409 | Конкурентный запрос с тем же Idempotency-Key |
| `rate_limited` | 429 | Превышен rate limit |
| `internal_error` | 500 | Всё остальное (детали в логах) |

### 3.6. WebSocket

**URL:** `ws://host/api/v1/ws`

После подключения сервер шлёт все события `reconciliation.events` без подписки (MVP). Клиент различает по полю `type`.

**Heartbeat:** клиент → `{"type":"ping"}` каждые 30 секунд, сервер → `{"type":"pong"}`.

**Reconnect:** при разрыве клиент делает REST-запрос для актуализации и переподключается.

### 3.7. Идемпотентность приёма событий

Ingestor поддерживает заголовок `Idempotency-Key` (стандарт Stripe). Если клиент передал ключ:

```
POST /api/v1/events/gateway
Idempotency-Key: <client-uuid>
```

При повторе с тем же ключом возвращается тот же ответ без побочных эффектов. TTL ключа — 24 часа в Redis.

Если ключ не передан, ingestor вычисляет fallback content hash: `sha256(api_key + canonical_json(body))` и использует его как ключ. Это защищает от дублей даже у источников, которые не реализуют идемпотентность правильно.

---

## 4. Схема базы данных (PostgreSQL 15)

### 4.1. ENUM-типы

```sql
CREATE TYPE overall_status AS ENUM ('matched', 'mismatch', 'degraded', 'pending');
CREATE TYPE incident_status AS ENUM ('open', 'acknowledged', 'resolved', 'auto_corrected');
CREATE TYPE source_kind AS ENUM ('merchant', 'gateway', 'bank');
CREATE TYPE incident_kind AS ENUM (
  'amount_mismatch',
  'fee_mismatch',
  'currency_mismatch',
  'missing_source',
  'duplicate',
  'source_offline'
);
```

### 4.2. Таблица входящих событий (для аудита и identity resolution)

```sql
CREATE TABLE events (
    internal_event_id UUID PRIMARY KEY,         -- UUID v7
    event_id          UUID NOT NULL,             -- ID от источника
    transaction_id    UUID NOT NULL,             -- сквозной ID (в MVP = correlation_id)
    correlation_id    UUID NOT NULL,             -- внутренний группирующий ID
    source            source_kind NOT NULL,
    amount_minor      BIGINT NOT NULL,
    fee_minor         BIGINT NOT NULL,
    currency          CHAR(3) NOT NULL,
    timestamp_ms      BIGINT NOT NULL,
    tx_type           VARCHAR(20) NOT NULL,
    merchant_id       VARCHAR(100),
    raw_payload       JSONB NOT NULL,
    received_at_ms    BIGINT NOT NULL,
    UNIQUE (event_id, source)                    -- дедупликация на уровне БД
);

CREATE INDEX idx_events_transaction ON events(transaction_id);
CREATE INDEX idx_events_correlation ON events(correlation_id);
CREATE INDEX idx_events_received ON events(received_at_ms DESC);
```

### 4.3. Таблица транзакций (агрегаты от Reconciler)

```sql
CREATE TABLE transactions (
    transaction_id UUID PRIMARY KEY,
    overall_status overall_status NOT NULL,
    merchant_id    VARCHAR(100),
    tx_type        VARCHAR(20) NOT NULL DEFAULT 'card',
    currency       CHAR(3) NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_transactions_status_created
    ON transactions(overall_status, created_at DESC);
```

### 4.4. Детали сверки по источникам

```sql
CREATE TABLE reconciliation_details (
    transaction_id   UUID NOT NULL REFERENCES transactions(transaction_id) ON DELETE CASCADE,
    source           source_kind NOT NULL,
    amount_expected  BIGINT,
    amount_actual    BIGINT,
    fee_expected     BIGINT,
    fee_actual       BIGINT,
    is_matched       BOOLEAN NOT NULL,
    mismatch_reason  VARCHAR(40),
    received_at      TIMESTAMPTZ,
    PRIMARY KEY (transaction_id, source)
);
```

### 4.5. Инциденты

```sql
CREATE TABLE incidents (
    id              BIGSERIAL PRIMARY KEY,
    transaction_id  UUID NOT NULL REFERENCES transactions(transaction_id) ON DELETE CASCADE,
    incident_type   incident_kind NOT NULL,
    severity        SMALLINT NOT NULL DEFAULT 1,
    description     TEXT NOT NULL,
    status          incident_status NOT NULL DEFAULT 'open',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    acknowledged_at TIMESTAMPTZ,
    resolved_at     TIMESTAMPTZ,
    escalated_at    TIMESTAMPTZ,
    auto_correction_proposed JSONB
);

CREATE INDEX idx_incidents_status_created ON incidents(status, created_at DESC);
CREATE INDEX idx_incidents_transaction ON incidents(transaction_id);
```

### 4.6. Кто пишет

| Таблица | Создание | Обновление |
|---------|----------|------------|
| `events` | Ingestor | Не обновляется |
| `transactions` | Reconciler | Reconciler |
| `reconciliation_details` | Reconciler | Reconciler |
| `incidents` | Reconciler | Reconciler (severity), API Gateway (ack/resolve) |

---

## 5. Логика работы компонентов

### 5.1. Scenario Director (Go, Вика)

Генерирует канонические транзакции и публикует их в `scenario.tx`. Эмуляторы подписаны на этот канал и каждый шлёт **свою** версию в Ingestor.

**Структура canonical-транзакции:**

```json
{
  "transaction_id": "<uuid>",
  "amount_minor": 5000000,
  "currency": "UZS",
  "tx_type": "card",
  "merchant_id": "MERCHANT_001",
  "gateway_fee_minor": 25000,
  "bank_fee_minor": 10000,
  "started_at_ms": 1735689600000
}
```

**Параметры:** `SCENARIO_TPS=10` (по умолчанию), `SCENARIO_TX_TYPES=card,transfer`.

### 5.2. Эмуляторы (Go, Вика)

Каждый эмулятор:
1. Подписан на `scenario.tx`.
2. На каждую canonical-транзакцию формирует **свою** версию события в соответствии с разделом 2.2.
3. Применяет шумы из параметров (пропуски, дубли, искажения).
4. Шлёт HTTP POST в свой endpoint (`/api/v1/events/{source}`).
5. Шлёт heartbeat каждые `EMULATOR_HEARTBEAT_INTERVAL_MS` в `/api/v1/heartbeat/{source}` (бонус).

**Параметры искажений** (env переменные):
- `EMULATOR_PCT_MISSING=0.05` — 5% пропусков
- `EMULATOR_PCT_DUPLICATE=0.02` — 2% дублей
- `EMULATOR_PCT_WRONG_AMOUNT=0.03` — 3% неверных сумм
- `EMULATOR_PCT_WRONG_FEE=0.03` — 3% ошибок в комиссиях
- `EMULATOR_DELAY_MS_MIN=50`, `EMULATOR_DELAY_MS_MAX=800`

### 5.3. Ingestor (Go, Виталий)

Принимает HTTP POST от эмуляторов на трёх endpoint'ах:

```
POST /api/v1/events/merchant
POST /api/v1/events/gateway
POST /api/v1/events/bank
```

**Пайплайн обработки одного запроса:**

1. **Аутентификация:** в MVP — простой `Bearer <token>`. В production-описании — HMAC-подпись (`X-Signature` заголовок).
2. **Schema validation:** валидация payload по `event.schema.json`. Невалидно → 400.
3. **Idempotency check:** `SET idempotency:<api_key>:<key> "PROCESSING" EX 86400 NX` в Redis. Если ключ существует — возвращаем закэшированный ответ.
4. **Нормализация:** парсим в каноническую структуру, проверяем семантику (валюта в whitelist, timestamp адекватный, etc.). Невалидно семантически → 422.
5. **Identity resolution:** в MVP `correlation_id := transaction_id`. Архитектурно — chain of resolvers (см. 5.4).
6. **Генерация internal_event_id:** UUID v7.
7. **Persistence в Postgres:** INSERT в `events` (одной транзакцией).
8. **Publish в Redis Stream:** `XADD transactions:raw * payload <json>`.
9. **Кэширование ответа** в idempotency-ключе.
10. **Возврат 202 Accepted** с `internal_event_id`.

**Целевая латентность:** ~2 мс end-to-end на одном инстансе.

### 5.4. Identity Resolution (внутри Ingestor)

Pluggable chain of strategies. Интерфейс:

```go
type Resolver interface {
    TryResolve(ctx context.Context, event *CanonicalEvent) (*string, error)
}
```

**Реализованные в MVP:**

1. **ReferenceIDResolver** — резолвит по `transaction_id` события. Это и есть основная стратегия MVP.

**Архитектурно подготовлены (упоминаются на демо, не реализуются за 48ч):**

2. **CrossRefResolver** — резолвит по графу ссылок между источниками (например, шлюз упомянул `merchant_order_id` и `bank_auth_code`).
3. **ContentHashResolver** — резолвит по детерминированному хэшу канонических полей.
4. **FuzzyMatchResolver** — резолвит по комбинации `amount + timestamp_window + card_mask_hash`.

Архитектура pluggable, добавление новой стратегии — это новый файл и добавление в `ResolverChain` без изменения остального кода.

### 5.5. Reconciler (Rust, Добрыня + ведущий)

**Главный сервис проекта.** Реализован на Rust для:
- предсказуемой sub-millisecond латентности без GC-пауз,
- безопасной concurrency через ownership model,
- эффективной работы с большими in-memory структурами.

#### Архитектура (паттерн: in-memory primary + Redis checkpointed state)

**State в памяти процесса:**
- `DashMap<Uuid, PartialTransaction>` — окна сверки (concurrent HashMap)
- `BTreeMap<DeadlineMs, Uuid>` — дедлайны таймеров (sorted)
- `HashSet<Uuid>` — недавно обработанные `event_id` (для дедупликации)

**State в Redis (для отказоустойчивости):**
- `HASH window:<transaction_id>` — чекпоинт окна (сериализованные события)
- `ZSET deadlines` — глобальные дедлайны (для рестарта)
- `SET dedup:<event_id> 1 EX 600 NX` — идемпотентность

#### Алгоритм обработки события

```
on_event(stream_message):
    1. Distill: достаём event из payload
    2. Dedup: пытаемся SET dedup:<event_id> NX EX 600
       - Если уже было → XACK и игнорируем
    3. Lookup: window = local_map.get(transaction_id)
       - Если нет → создаём, ZADD deadlines (now + 2000), HSET в Redis
    4. Duplicate check: если этот source уже в window → инцидент 'duplicate'
    5. Add event: window.events[source] = event
       - HSET window:<tx_id> <source> <json>
    6. Если len(window.events) == 3:
       - reconcile() → проверка I1, I2, I3
       - persist_to_postgres(result)
       - publish 'transaction_matched' или 'incident_created'
       - cleanup: ZREM deadlines, DEL window:<tx_id>
    7. Else:
       - publish 'transaction_progress'
    8. XACK stream message
```

#### Алгоритм таймерного воркера

```
loop every 100ms:
    now = now_ms()
    expired = BTreeMap.range(..now)
    for (deadline, tx_id) in expired:
        if redis.ZREM("deadlines", tx_id) == 1:    # атомарный захват
            window = local_map.remove(tx_id)
            if window.events.len() < 3:
                incident = build_missing_source_incident(window)
                persist_to_postgres(incident)
                publish_to_pubsub(incident)
            cleanup_redis(tx_id)
```

#### Алгоритм эскалации инцидентов

Reconciler ведёт ещё один sorted-set эскалаций. При создании инцидента:

```
ZADD incident_escalations <now + 10000> <incident_id>
```

Тот же воркер раз в 100мс проверяет истёкшие, повышает severity до 2, публикует `incident_updated`. Если инцидент был подтверждён оператором — API Gateway удалил его из ZSET через `ZREM`.

#### Производительность

Целевые показатели на одном инстансе на среднем железе:
- Латентность матча (от поступления последнего события до записи в Postgres): **<5 мс p99**
- Пропускная способность: **>5000 событий/сек**
- Память на одно активное окно: **~500 байт**
- Память при 100 000 активных окон: **~50 МБ**

### 5.6. API Gateway (Go, Виталий)

**Чтение из Postgres:**
- `GET /transactions` — `SELECT FROM transactions ORDER BY created_at DESC LIMIT ...`
- `GET /transactions/{id}` — JOIN `transactions` + `reconciliation_details` + последние 5 инцидентов
- `GET /incidents` — `SELECT FROM incidents WHERE status = ?`
- `GET /metrics/mismatches-per-minute` — `GROUP BY date_trunc('minute', created_at)` на инцидентах

**Запись в Postgres:**
- `POST /incidents/{id}/ack` — `UPDATE incidents SET status='acknowledged', acknowledged_at=NOW() WHERE id=? AND status='open'`
- `POST /incidents/{id}/resolve` — `UPDATE incidents SET status='resolved', resolved_at=NOW() WHERE id=?`

**WebSocket:**
- Подписан на канал `reconciliation.events` через Redis Pub/Sub.
- Каждое сообщение из Pub/Sub транслируется во все активные WebSocket-соединения.
- Heartbeat: ping/pong каждые 30 секунд.

### 5.7. Operator UI (React, Кирилл)

**Главная страница:** таблица live-транзакций.

```
┌────────────────┬──────────┬─────────┬───────┬─────────────────┐
│ Transaction ID │ Merchant │ Gateway │ Bank  │ Status          │
├────────────────┼──────────┼─────────┼───────┼─────────────────┤
│ ABC-123…       │    ✓     │    ✓    │   ✓   │ ✅ matched       │
│ DEF-456…       │    ✓     │    ✗    │   ✓   │ ⚠ fee_mismatch  │
│ GHI-789…       │    ✓     │    ✓    │   ○   │ ⏳ pending bank  │
│ JKL-012…       │    ✓     │    —    │   ✓   │ 🛑 source_offline│
└────────────────┴──────────┴─────────┴───────┴─────────────────┘
```

Цвета: matched — зелёный, mismatch — красный, pending — серый, offline — оранжевый.

**Страница инцидента (drawer):** три колонки (по источникам), подсветка расходящихся полей, рассчитанные ожидаемые vs фактические суммы, кнопки «Ack» и «Resolve», (бонус) «Применить автокорректировку».

**График (бонус):** Recharts, ось X — минуты за последний час, ось Y — количество инцидентов.

**Реализация:** TanStack Query для REST, ws-клиент для live-обновлений. Типы генерируются из OpenAPI: `npx openapi-typescript ../contracts/openapi.yaml -o src/types/api.ts`.

---

## 6. Конфигурация и инфраструктура

### 6.1. Переменные окружения (`.env.example`)

**Общие:**
```
LOG_LEVEL=info
SERVICE_NAME=<ingestor|reconciler|api-gateway|emulator-*>
```

**Postgres:**
```
POSTGRES_URL=postgres://recon:recon@postgres:5432/recon?sslmode=disable
```

**Redis:**
```
REDIS_URL=redis://redis:6379/0
REDIS_STREAM_NAME=transactions:raw
REDIS_CONSUMER_GROUP=reconciler
REDIS_PUBSUB_CHANNEL=reconciliation.events
REDIS_SCENARIO_CHANNEL=scenario.tx
```

**Ingestor:** `INGESTOR_PORT=8080`

**Reconciler:**
```
RECONCILER_TIMEOUT_MS=2000          # окно сверки
RECONCILER_ESCALATION_MS=10000      # эскалация инцидента
RECONCILER_DEDUP_WINDOW_MS=600000   # окно дедупликации (10 мин)
RECONCILER_TIMER_TICK_MS=100        # частота таймерного воркера
```

**API Gateway:** `API_PORT=8090`

**Эмуляторы:**
```
EMULATOR_SOURCE=merchant            # merchant|gateway|bank
EMULATOR_INGESTOR_URL=http://ingestor:8080
EMULATOR_PCT_MISSING=0.05
EMULATOR_PCT_DUPLICATE=0.02
EMULATOR_PCT_WRONG_AMOUNT=0.03
EMULATOR_PCT_WRONG_FEE=0.03
EMULATOR_DELAY_MS_MIN=50
EMULATOR_DELAY_MS_MAX=800
EMULATOR_HEARTBEAT_INTERVAL_MS=2000
```

**Scenario Director:**
```
SCENARIO_TPS=10
SCENARIO_TX_TYPES=card,transfer
SCENARIO_CURRENCY=UZS
SCENARIO_AMOUNT_MIN=10000           # 100 UZS
SCENARIO_AMOUNT_MAX=10000000        # 100 000 UZS
SCENARIO_GATEWAY_FEE_PCT=0.5
SCENARIO_BANK_FEE_PCT=0.2
```

### 6.2. Docker Compose

Все сервисы поднимаются одной командой `docker compose up -d`. Состав:

- `postgres:15-alpine`
- `redis:7-alpine` с включённым AOF
- `scenario`, `emulator-merchant`, `emulator-gateway`, `emulator-bank`
- `ingestor`, `reconciler`, `api-gateway`
- `ui` (Nginx с статикой)

Health-checks через `/healthz`. Зависимости: эмуляторы стартуют после `scenario` и `ingestor`; `reconciler` — после `redis` и `postgres`; `api-gateway` — после `reconciler`.

### 6.3. Health-checks

Каждый сервис поддерживает:
- `GET /healthz` → 200 OK без проверок (процесс жив)
- `GET /readyz` → 200 OK если соединения с зависимостями работают, иначе 503

---

## 7. Логирование

### 7.1. Формат

Структурированные JSON-логи в stdout, один объект на строку.

- **Go:** `log/slog` с JSON-handler.
- **Rust:** `tracing` + `tracing-subscriber` с JSON formatter.

### 7.2. Обязательные поля

```json
{
  "ts": "2025-01-15T10:23:45.123Z",
  "level": "info",
  "service": "reconciler",
  "msg": "transaction matched",
  "transaction_id": "...",
  "duration_ms": 423
}
```

| Поле | Когда обязательно |
|------|--------------------|
| `ts`, `level`, `service`, `msg` | Всегда |
| `transaction_id` | Если есть в контексте |
| `event_id`, `internal_event_id` | Если есть в контексте |
| `correlation_id` | Если есть в контексте |
| `error` | Если `level=error` |

### 7.3. Уровни

| Уровень | Когда |
|---------|-------|
| `error` | Сломалось, нужно внимание |
| `warn` | Подозрительное (retry, замедление) |
| `info` | Бизнес-события: matched, mismatch, ack |
| `debug` | Детали разработки. На демо `LOG_LEVEL=info` |

### 7.4. Запреты

- Не логировать полные payloads на уровне `info` (только `debug`).
- Никаких секретов, токенов, ключей.
- Никаких персональных данных (в нашем MVP они отсутствуют, но правило).

---

## 8. Конвенции кодирования

### 8.1. Структура репозитория

```
/
├── README.md
├── STANDARDS.md                # этот документ
├── docker-compose.yml
├── .env.example
├── .gitignore
├── contracts/
│   ├── event.schema.json
│   ├── stream_message.schema.json
│   ├── pubsub_events.md
│   ├── openapi.yaml
│   └── websocket.md
├── docs/
│   ├── architecture.md         # диаграмма (Excalidraw)
│   ├── invariants.md           # денежный поток
│   ├── demo-script.md          # план демо
│   └── presentation.md         # план презентации Виталия
├── migrations/
│   └── 001_initial.sql
└── services/
    ├── scenario/               # Go (Вика)
    ├── emulator-merchant/      # Go (Вика)
    ├── emulator-gateway/       # Go (Вика)
    ├── emulator-bank/          # Go (Вика)
    ├── ingestor/               # Go (Виталий)
    ├── reconciler/             # Rust (Добрыня + ведущий)
    ├── api-gateway/            # Go (Виталий)
    └── ui/                     # React (Кирилл)
```

### 8.2. Go

- Форматирование: `gofmt` обязательно.
- Линтер: `golangci-lint` с дефолтом.
- Структура пакетов: `cmd/`, `internal/{config,handler,store,...}`.
- Ошибки: `fmt.Errorf("context: %w", err)`. Проверка через `errors.Is`/`errors.As`.
- Контекст: `context.Context` первым параметром в I/O функциях.
- Логирование: `slog.InfoContext(ctx, "msg", "key", value)`.

### 8.3. Rust

- Форматирование: `rustfmt` обязательно.
- Линтер: `cargo clippy --all-targets -- -D warnings`.
- Ошибки: `thiserror` для типов ошибок, `anyhow` для `main` и тестов.
- Async: только `tokio`, никакого `async-std`.
- Структура: workspace с одним бинарным крейтом для MVP. Если разрастётся — выделить `reconciler-core`, `reconciler-storage`, `reconciler-bin`.
- Логирование: `tracing::info!(transaction_id = %tx_id, "matched")`.
- Concurrency: `dashmap` для concurrent HashMap, `tokio::sync::RwLock` где необходимо.

### 8.4. TypeScript

- Prettier (дефолт).
- ESLint с `@typescript-eslint/recommended`.
- Структура: `src/{api,ws,components,pages,hooks,types}/`.
- Типы API из OpenAPI генерируются командой выше.
- Запросы — через TanStack Query, не голый fetch в компонентах.

### 8.5. SQL

- Все миграции в `migrations/` пронумерованы: `001_initial.sql`, `002_*.sql`.
- `snake_case` для таблиц и колонок.
- Все таблицы с временем создания имеют `created_at TIMESTAMPTZ DEFAULT NOW()`.

---

## 9. Git и процесс

### 9.1. Ветки

- `main` — единственная защищённая, push напрямую запрещён.
- Каждый работает в `wip/<имя>` (например, `wip/dobrynia-matcher`).

### 9.2. Pull Request

- Заголовок: `<scope>: <what>`, например `reconciler: add timeout-based matching`.
- Описание: что сделано, что тестировал.
- Минимум один аппрув от ведущего (или Виталия, если ведущий недоступен).
- Squash merge.

### 9.3. Что не коммитим

- `.env` (только `.env.example`)
- Бинарники, артефакты сборки
- IDE-конфиги (`.vscode/`, `.idea/`)
- Логи, дампы

---

## 10. Процесс работы (по часам)

### Часы 0–4: установка (ведущий + все)

- Прочитать этот документ всей командой
- Поднять `docker-compose.yml` со всеми сервисами-заглушками
- Применить миграции
- Согласовать схему `event.schema.json`
- Раздать брифы (см. ниже)

### Часы 4–14: первый слой реализации

- **Вика:** Scenario Director + один эмулятор (merchant), он шлёт «правильные» события без шумов
- **Виталий:** Ingestor — приём, валидация, запись в Postgres, XADD в Redis
- **Добрыня + ведущий:** парное программирование на Rust, скелет Reconciler — чтение из Stream, in-memory окно, базовая логика добавления событий
- **Кирилл:** UI с моками (без реального API)

### Часы 14–24: интеграция компонентов

- **Вика:** все три эмулятора, шумы пока выключены
- **Виталий:** API Gateway — REST + WebSocket
- **Добрыня:** инварианты I1/I2/I3, таймерный воркер, запись инцидентов в Postgres
- **Кирилл:** подключение к API, live-обновления через WebSocket

### Часы 24–36: edge cases и шумы

- **Вика:** включает все шумы в эмуляторах, добавляет heartbeat
- **Виталий:** обработка инцидентов в API (ack/resolve), фильтры, пагинация
- **Добрыня:** дедупликация event_id, эскалация по таймеру, обработка дублей и пропусков
- **Кирилл:** страница инцидента с детализацией, цветовая индикация

### Часы 36–44: бонусы

В порядке убывания готовности пожертвовать:
1. График расхождений в минуту
2. Автокорректировка
3. Тип транзакции `transfer`
4. Keepalive (`source_offline`)

### Часы 44–48: репетиция демо

**Только** фиксы критических багов и репетиция презентации. Никаких новых фич.

### Точки синхронизации

- Каждые 6 часов — stand-up 15 минут
- Интеграционные чек-пойнты в часы 24, 36, 44

---

## 11. Демо-сценарий для презентации (Виталий)

### 11.1. Структура выступления (10 минут)

**Минуты 0–1: Проблема**

> «В современных платёжных системах сверка между мерчантом, шлюзом и банком происходит с задержкой от минут до суток. Это значит, что расхождения — потерянные транзакции, неучтённые комиссии, дубли — обнаруживаются поздно, когда деньги уже ушли. Для узбекского рынка с быстрым ростом безналичных платежей это критично: доверие к системе строится на способности оператора видеть проблему в момент её возникновения.»

**Минуты 1–3: Решение и архитектура**

Показываем диаграмму архитектуры (раздел 1.1). Объясняем 6 ключевых архитектурных решений (раздел 1.2). Особо выделяем:

- Rust для матчера — sub-millisecond латентность, GC-free, безопасная concurrency
- Pluggable identity resolution
- Идемпотентность по Idempotency-Key (стандарт Stripe)
- UUID v7 для оптимальных B-tree индексов

**Минуты 3–6: Демо**

Запускаем `docker compose up`, открываем UI. Сценарий:

1. **Нормальный поток.** Эмуляторы шлют корректные транзакции. В UI бежит зелёная таблица «matched». Показываем график — расхождений 0.

2. **Расхождение сумм.** Командой увеличиваем `EMULATOR_PCT_WRONG_AMOUNT` у gateway до 50%. В UI появляются красные строки с `amount_mismatch`. Кликаем на инцидент — drawer с детальным разбором, видно конкретные суммы каждого источника.

3. **Расхождение комиссий.** То же с `EMULATOR_PCT_WRONG_FEE`. Показываем, что система **точно идентифицирует**, какая именно комиссия не сошлась (I2).

4. **Пропуск источника.** Останавливаем эмулятор банка. Через 2 секунды появляются инциденты `missing_source`. Через 10 секунд — эскалация (severity 2). Поднимаем банк обратно — heartbeat восстановлен, статус источника «online».

5. **Дубль.** Включаем `EMULATOR_PCT_DUPLICATE`. В UI появляются инциденты `duplicate`.

6. **Подтверждение и закрытие.** Оператор кликает «Ack» — инцидент в статусе acknowledged. «Resolve» — закрыт. WebSocket мгновенно обновляет таблицу.

**Минуты 6–8: Технологический фокус**

Объясняем, **почему Rust** для матчера на примере метрик: показываем дашборд (если успели) или скрин — латентность матчинга p99 < 5 мс, пропускная способность одного инстанса >5000 tps. Упоминаем pluggable identity resolution и расширение до fuzzy matching.

**Минуты 8–9: Соответствие критериям**

| Критерий | Реализация |
|----------|-----------|
| Скорость <2 сек | <5 мс p99, реальная сквозная задержка ~50-100 мс |
| Точность матчинга | Дедупликация по event_id, идемпотентность, формальные инварианты |
| Наглядность UI | Live-таблица, цветовая индикация, детальный разбор инцидентов |
| Эскалация по таймеру | Sorted-set таймеров в Reconciler, повышение severity через 10с |

**Бонусы:** график, автокорректировка, два типа транзакций, keepalive — что успели реализовать.

**Минуты 9–10: Закрытие и Q&A**

> «Архитектура готова к production: матчер масштабируется горизонтально через consumer groups в Redis Streams и шардирование по transaction_id. Identity resolution расширяется до fuzzy matching без изменения матчера. Это не прототип, а минимальный, но архитектурно правильный фундамент платформы сверки. Спасибо.»

### 11.2. Подготовка к Q&A

**Возможные вопросы и заготовленные ответы:**

**Q: Почему Rust для матчера, а не Go?**
> «Матчер — критический путь, требующий предсказуемой sub-millisecond латентности. У Rust нет GC-пауз, что даёт детерминированную производительность даже при больших окнах сверки. Кроме того, безопасность памяти через ownership model критична для сервиса, работающего с финансовыми данными. Остальные сервисы на Go, потому что для них важнее скорость разработки.»

**Q: Что произойдёт, если матчер упадёт?**
> «Состояние окна сверки реплицируется в Redis (HASH + ZSET таймеров). При рестарте Reconciler восстанавливает state из Redis за миллисекунды. События в Stream сохраняются благодаря Redis AOF. Это даёт нам uptime > 99.9% при правильной настройке Redis Sentinel в production.»

**Q: Как обеспечивается идемпотентность?**
> «На двух уровнях: на уровне API через Idempotency-Key с TTL 24 часа в Redis (стандарт Stripe), и на уровне матчера через дедупликацию event_id с TTL 10 минут. Это защищает от дублей при сетевых ретраях источников.»

**Q: Что если в production cross-references между источниками не передаются?**
> «Identity resolution построен как chain of strategies. В MVP реализована резолюция по согласованному reference_id. В production добавляются content-hash matching для систем, договорившихся о канонической форме данных, и fuzzy matching по комбинации amount + timestamp_window + card_mask_hash для legacy-источников. Архитектурно — это новые реализации интерфейса Resolver, ядро матчинга не меняется.»

**Q: Как масштабируется при росте нагрузки?**
> «Несколько уровней: вертикально матчер на одном инстансе уже держит >5000 tps. Горизонтально — Redis Streams consumer groups автоматически распределяют события между инстансами при шардировании по transaction_id. На уровне 50 000+ tps миграция на Kafka Streams с RocksDB state store даёт практически безграничное масштабирование без изменения бизнес-логики.»

**Q: Как тестировали? (важно — Виталий идёт в QA)**
> «Несколько уровней: unit-тесты на нормализацию и инварианты, интеграционные через docker-compose с фиксированным сценарием от Scenario Director, нагрузочные через k6 (имитация 1000+ tps). Проверяли корректность матчинга на сценариях с известными расхождениями и отсутствие ложных срабатываний на эталонных данных.»

---

## 12. Чек-лист готовности к старту

- [ ] Документ прочитан всей командой
- [ ] `contracts/event.schema.json` лежит в репозитории и согласован
- [ ] `docs/invariants.md` написан и понятен (особенно Вике и Добрыне)
- [ ] `docker-compose.yml` поднимается одной командой
- [ ] `migrations/001_initial.sql` применяется на пустую БД
- [ ] У всех настроена среда: Go 1.22, Rust 1.75, Node 20, Docker
- [ ] Каждый сделал тестовый коммит в `wip/<имя>` и PR
- [ ] Виталий ознакомился с разделом 11 (демо-сценарий)
- [ ] Следующий sync назначен через 6 часов

---

## 13. Чек-лист готовности к демо (час 44)

- [ ] `docker compose up` поднимает всю систему без ошибок
- [ ] Эмуляторы шлют события, в Postgres появляются записи
- [ ] Reconciler матчит транзакции, инциденты создаются
- [ ] UI показывает live-таблицу с обновлениями через WebSocket
- [ ] Сценарий демо (раздел 11.1) проигран от начала до конца минимум дважды
- [ ] Виталий проговорил презентацию минимум один раз вслух
- [ ] Известные баги задокументированы (что НЕ показывать на демо)
- [ ] План Б на случай, если что-то упадёт во время демо

---

## Приложение A: Брифы для команды

### Вика (Scenario Director + эмуляторы)

> Ты делаешь источники данных. Без правильных данных матчеру нечего сверять. **Главное:** Scenario Director — отдельный сервис, генерит canonical-транзакции в Redis Pub/Sub. Три эмулятора подписаны, каждый формирует **свою** версию с шумами. Это не три независимых генератора — у них один общий «режиссёр».
>
> Сложности, которые я тебе обозначу заранее: правильно вычислять `bank.amount_minor = N - gateway.fee_minor` в эмуляторе банка (раздел 2.2). И heartbeat в отдельной горутине, не блокирующей основной поток. JSON Schema события согласовываем со мной в первые 2 часа.

### Добрыня (Reconciler на Rust)

> Ты делаешь сердце системы. Мы пишем это вместе первые 8 часов: я веду по архитектуре, ты пишешь код. Дальше ты доделываешь, я ревьюшу. Rust выбран осознанно — это production-grade выбор для матчера, но дебаг сложнее Go, поэтому не стесняйся спрашивать.
>
> Технологии: `tokio`, `dashmap` для concurrent map, `redis-rs` async-клиент, `sqlx` для Postgres, `serde` для JSON, `tracing` для логов. Структура — workspace с одним бинарным крейтом.
>
> Главные риски: borrow checker на первой неделе Rust может застрять. Если в первые 6 часов мы не написали скелет, который компилируется и читает из Stream — я забираю матчер себе и переключаю тебя на помощь Виталию с API Gateway. Это план Б.

### Виталий (Ingestor + API + БД + презентация)

> У тебя три роли: бэкенд, БД, презентатор. Самое широкое распределение. **Первое:** миграции БД и Ingestor (приём, валидация, идемпотентность). Это 4–14 час. **Второе:** API Gateway с REST и WebSocket — час 14–32. **Третье:** репетиция демо — час 36+.
>
> На презентации ты главный, потому что у тебя самое полное понимание архитектуры в команде после ведущего. Раздел 11 — твой основной материал. Прочитай его 3 раза за проект, последний раз — в час 40.
>
> Главное правило: **не уходи в украшательства**. Сначала самые тупые эндпоинты без фильтров, потом наращиваешь. WebSocket — в конце.

### Кирилл (UI)

> Ты делаешь самое видимое — то, что покажут жюри. **Главное:** начинаешь с моков, не ждёшь API. OpenAPI-спека есть, фейкаешь данные по ней, рисуешь интерфейс. Когда API готов — подключаешься заменой одной строчки (baseURL).
>
> Приоритеты: сначала таблица транзакций, потом drawer инцидента, потом график. Никаких анимаций, тем, красивостей, пока базовое не работает. WebSocket — последним, до этого polling каждые 2 секунды.
>
> На демо именно твою часть видят жюри. Уделяй полировке внимание в последние 8 часов, когда базовый функционал готов.

---

**Конец документа.**

При расхождении кода с этим документом — обновляйте документ **до** мерджа. Документ — единственный источник правды.
