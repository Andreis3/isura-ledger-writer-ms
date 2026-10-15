# isura-ledger-ms

The double-entry accounting engine of Isura Bank. **Every financial movement goes through here.**

This service is the financial source of truth: it stores immutable debit/credit entries and their per-account running balances, groups postings into balanced business movements, enforces atomic bookkeeping, and publishes committed events reliably through the Transactional Outbox pattern.

---

## Responsibilities

- Double-entry bookkeeping — debit + credit = 0, always atomic via a single PostgreSQL transaction
- Real-time balance per account
- Transaction idempotency via `idempotency_key` with `UNIQUE CONSTRAINT`
- Reliable event publishing via Transactional Outbox → NATS JetStream
- Accounting representation of accounts (asset, liability, revenue, expense, equity)
- Required balanced `movements` grouping transaction entries with an immutable type and statement description

---

## Architecture

This service follows **Hexagonal Architecture** with **tactical Domain-Driven Design**, organized into `transaction`, `account`, and `outbox` modules inside the **Ledger bounded context**.

### Dependency rule

```
transport → application → domain ← infrastructure
```

The `domain` layer imports nothing external. The `infrastructure` layer implements the interfaces defined in `domain`. The `transport` layer knows only about `application` use cases.

### Project structure

The service is organized by domain, application use cases, adapters and infrastructure. Key paths:

```text
cmd/server/                         # Service bootstrap
internal/domain/transaction/        # Transaction, Entry, Movement, domain events
internal/domain/account/            # Accounting accounts and balance policy
internal/domain/outbox/             # Outbox domain model
internal/application/command/       # CreateTransaction and other commands
internal/application/dto/           # REST/application payload validation
internal/infra/postgres/repository/ # PostgreSQL persistence, including movements
internal/infra/postgres/uow/        # pgx Unit of Work
internal/infra/nats/                # NATS JetStream outbox relay
internal/transport/                 # HTTP and gRPC adapters
db/                                 # Atlas HCL schemas, including movements.pg.hcl
proto/                              # Protobuf contracts
tests/                              # Unit, integration and chaos tests
```

---

## Domain model

### Money — Value Object

Monetary values are stored as `int64` cents — never `float64`. This eliminates floating-point precision errors in financial calculations.

```
Money { amount int64, currency Currency }

BRL → "BRL" | USD → "USD" | EUR → "EUR"
```

Operations: `Add`, `Subtract`, `Equal`, `IsSufficientBalance`, `IsZero`, `IsNegative`, `IsPositive`, `String` (e.g. `"100.50 BRL"`).

### Transaction — Aggregate Root

The central aggregate. Enforces double-entry invariants and owns its state machine.

```
PENDING → COMPLETED  ✓
PENDING → FAILED     ✓
COMPLETED → any      ✗
FAILED → any         ✗
```

A `Transaction` contains **at least two** `Entry` records. It may contain multiple balanced movements (for example TED + fee). Debit and credit totals must balance, the transaction currency is consistent, and each entry belongs to exactly one individually balanced `Movement`. The aggregate validates these invariants before persistence.

### Entry — Entity

Represents one side of a double-entry ledger. Belongs to a `Transaction` aggregate — never created independently.

```
Entry {
  ID             EntryID
  TransactionID  TransactionID
  AccountID      AccountID
  SequenceNumber int64
  RunningBalance int64
  Direction      DEBIT | CREDIT
  Amount         Money
  CreatedAt      time.Time
}
```

### Movement — Entity inside Transaction

A movement represents one business purpose within the accounting transaction, such as a TED transfer or its fee. Unlike the earlier JSONB proposal, **the implementation on `main` uses a relational `movements` table with a movement ID**.

```text
Transaction
  Movement (TED_TRANSFER, "TED enviada")
    Entry: DEBIT  100000 cents
    Entry: CREDIT 100000 cents
  Movement (TED_FEE, "Tarifa TED")
    Entry: DEBIT    1000 cents
    Entry: CREDIT   1000 cents
```

A movement has `id`, `transaction_id`, `type`, and required nonblank `description`. Each `Entry` has a `movement_id`; the composite foreign key `(movement_id, transaction_id)` enforces that referenced movements belong to the same transaction. The domain also checks that movement entries balance independently, no entry is assigned twice, and every entry belongs to one movement. Amounts remain authoritative in `entries`, not in a duplicate JSON document.

The API accepts movements with zero-based `entry_positions`, which the Writer resolves to generated entry IDs. The grouping and descriptions participate in the idempotency fingerprint.

### Account — Entity

Accounting representation of a bank account within the ledger. Not the same as the customer-facing account in `isura-account-ms` — the ledger holds only what it needs for bookkeeping.

```
Account {
  ID             AccountID
  AccountExternalID     string      // ID from isura-account-ms (correlation key)
  AccountingType ASSET | LIABILITY | REVENUE | EXPENSE | EQUITY
  BalancePolicy  BALANCE_NON_NEGATIVE | BALANCE_UNRESTRICTED
  Currency       Currency
  CreatedAt      time.Time
  UpdatedAt      time.Time
}
```

### OutboxEvent — Aggregate

Ensures reliable event delivery to NATS JetStream without dual writes. Persisted in the same PostgreSQL transaction as the `Transaction`. The relay claims pending events in PostgreSQL and publishes them to JetStream using the outbox ID as `Nats-Msg-Id` for deduplication.

```
PENDING → FAILED    ✓  (publish attempt failed)
PENDING → SUCCESS   ✓  (published successfully)
FAILED  → PENDING   ✓  (retry — if Attempts < MaxAttempts)
SUCCESS → any       ✗
```

---

## Key design decisions

### Transactional Outbox

The writer persists each transaction, its movements, entries, and its outbox event atomically. The outbox relay then publishes committed events to NATS JetStream:

```text
CreateTransaction
       │
       └── PostgreSQL transaction
             ├── transactions
             ├── movements (purpose + description)
             ├── entries (financial postings, each linked to a movement)
             └── outbox_events
                     │
                     └── OutboxRelay → NATS JetStream
```

All writes happen in one PostgreSQL transaction:

```
BEGIN
  INSERT INTO transactions ...
  INSERT INTO movements ...
  INSERT INTO entries ...      (all debit/credit postings)
  INSERT INTO outbox_events ... (event payload)
COMMIT
```

`accounts` stores account identity, currency, status, and balance policy. For ledger transactions, `entries` is the append-only record of financial postings, ordered per account by `sequence_number`; each entry stores the resulting `running_balance`, and the latest entry provides the balance used by the transaction flow. The normal transaction flow inserts entries and does not update or delete previously committed entries. The existing `balances` table and its account-creation event handler are a separate legacy/bootstrap record and are not the source used to calculate a transaction's running balance. This initiative does not change or remove that table. If a transaction commit fails, its transaction, movements, entries, and outbox event are all rolled back. After commit, the relay publishes the event to JetStream.

`transactions` stores the operation envelope, status, and idempotency key/fingerprint. `outbox_events` stores integration events in the same PostgreSQL transaction as their ledger operation; the relay publishes committed events to JetStream and can retry failed deliveries. Historical balance backfill is a separate operational maintenance flow that can recalculate `entries.running_balance`; it is outside the normal append-only transaction flow and is not changed here.

### Idempotency

Every transaction carries an `idempotency_key`. A `UNIQUE CONSTRAINT` on the `transactions` table ensures that concurrent retries with the same key result in exactly one committed transaction — even if two requests race past the application-level `ExistsByIdempotencyKey` check.

### Unit of Work

The `UnitOfWork` interface wraps the PostgreSQL transaction lifecycle. All repository calls within a use case's `Execute` method receive a `context.Context` carrying the active `pgx.Tx`. Each repository's `resolveDB` method picks the transaction over the connection pool when present.

```go
return c.uow.WithRetryableTransaction(ctx, func(ctxTx context.Context) error {
    // transaction, movements, entries, and outbox use one pgx.Tx
    if err := c.transactionRepository.Save(ctxTx, tx); err != nil {
        return err
    }
    return c.outboxRepository.Save(ctxTx, event)
})
```

### CQRS and downstream Reader

The Go Writer owns the authoritative accounting data in PostgreSQL. A separate Java Ledger Reader is planned with its own PostgreSQL read model, built from events; it does **not** query the Writer database directly. Within the Ledger bounded context, the existing event transport is NATS JetStream. Kafka is considered for cross-bounded-context integrations, not required for the internal Writer → Reader flow. The Reader and its projection/rebuild workflow are not implemented in this repository.

---

## Tech stack

| Layer | Technology                          |
|---|-------------------------------------|
| Transport | gRPC + Protobuf                     |
| Language | Go 1.26.x                           |
| Persistence | PostgreSQL 18 + pgx/v5              |
| Migrations | Atlas                               |
| Events | NATS JetStream (Transactional Outbox) |
| Observability | OpenTelemetry + Prometheus          |
| Container | Docker + Docker Compose             |

### Key dependencies

```
github.com/jackc/pgx/v5          # PostgreSQL driver
github.com/google/uuid           # UUID generation
github.com/nats-io/nats.go       # NATS JetStream client
google.golang.org/grpc           # gRPC server
google.golang.org/protobuf       # Protobuf serialization
go.opentelemetry.io/otel         # Tracing and metrics
github.com/prometheus/client_golang # Prometheus metrics
```

---

## Running locally

### Prerequisites

- Go 1.26.4+
- Docker and Docker Compose
- `atlas` CLI for schema changes

### Setup

```bash
# clone
git clone https://github.com/Andreis3/isura-ledger-writer-ms.git
cd isura-ledger-writer-ms

# create local config and start dependencies
cp config.example.json config.json
make up

# apply schema
make migrate

# start the service
make run-app
```

### Makefile commands

```bash
make migrate         # apply the Atlas schema
make unit            # run unit tests
make integration-tests # run integration tests
make chaos-tests      # run PostgreSQL/NATS failure recovery scenarios
make unit-cover-business # measure application/domain test coverage
make proto-lint      # lint protobuf contracts
make proto-gen       # regenerate protobuf files
make build           # build the application image
```

---

## Environment variables

```env
# Server ports
GRPC_PORT=50051
HTTP_PORT=8080

# PostgreSQL
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_USER=admin
POSTGRES_PASSWORD=admin
POSTGRES_DB=isura_ledger_main

# NATS and tracing
NATS_URL=nats://localhost:4222
OTEL_HOST=localhost

# Outbox relay
NATS_RELAY_BATCH_SIZE=100
NATS_RELAY_MAX_WORKERS=4
NATS_RELAY_MAX_ATTEMPTS=3
NATS_RELAY_POLL_INTERVAL=1s
NATS_RELAY_RETRY_AFTER=5s
```

---

## Database schema

### tables

| Table | Description |
|---|---|
| `transactions` | Aggregate root — one record per transaction |
| `entries` | Immutable debit/credit postings, with account sequencing, running balance, and `movement_id` |
| `movements` | Business movement groups with ID, type, required description, and transaction FK |
| `accounts` | Accounting representation of accounts |
| `balances` | Existing legacy/bootstrap balance record created by the account balance event flow; not used to calculate transaction running balances |
| `outbox_events` | Transactional outbox — pending NATS JetStream events |

### Key constraints

```sql
-- idempotency guarantee
UNIQUE (idempotency_key) ON transactions

-- double-entry integrity
FOREIGN KEY (transaction_id) REFERENCES transactions(id) ON entries

-- movement belongs to the same transaction as each entry
FOREIGN KEY (movement_id, transaction_id) REFERENCES movements(id, transaction_id) ON entries

-- outbox ordering
INDEX (status, created_at) ON outbox_events
```

---

## Movements API example (TED + fee)

For multi-entry transactions, send the total debit amount in cents, the ordered `entries` array, and one or more `movements` with **zero-based** `entry_positions`. Movement `type` and `description` are mandatory. The account IDs below are illustrative; real requests require valid account UUIDs.

```json
{
  "idempotency_key": "ted-0001",
  "operation": "TED_OUT",
  "amount": 101000,
  "entries": [
    {"account_id": "customer-uuid", "direction": "DEBIT", "amount": 100000, "currency": "BRL"},
    {"account_id": "settlement-uuid", "direction": "CREDIT", "amount": 100000, "currency": "BRL"},
    {"account_id": "customer-uuid", "direction": "DEBIT", "amount": 1000, "currency": "BRL"},
    {"account_id": "fee-revenue-uuid", "direction": "CREDIT", "amount": 1000, "currency": "BRL"}
  ],
  "movements": [
    {"type": "TED_TRANSFER", "description": "TED enviada", "entry_positions": [0, 1]},
    {"type": "TED_FEE", "description": "Tarifa TED", "entry_positions": [2, 3]}
  ]
}
```

A simple transfer using `debit_account_id` and `credit_account_id` also requires movements; its normalized entries are ordered **CREDIT (position 0), DEBIT (position 1)**.

### TransactionCreated event

After commit, the Outbox relay publishes the `TransactionCreated` JSON payload to NATS JetStream. The current producer sets `event_version: 1`. Its `entries` include `entry_id`, `position`, `account_id`, `sequence_number`, `direction`, `amount`, `currency`, `running_balance`, and `movement_id`. Each `movements` item contains `movement_id`, `type`, `description`, and `entry_ids`.

The event provides the association needed for the future Reader to produce statements. Downstream consumers must tolerate duplicate delivery and handle account ordering/gaps; event persistence in the Outbox does not by itself guarantee indefinite replay retention.

---

## gRPC API

```protobuf
service LedgerService {
  rpc CreateTransaction (CreateTransactionRequest) returns (CreateTransactionResponse);
  rpc GetBalance        (GetBalanceRequest)         returns (GetBalanceResponse);
  rpc GetTransaction    (GetTransactionRequest)      returns (GetTransactionResponse);
  rpc CreateAccount     (CreateAccountRequest)       returns (CreateAccountResponse);
}
```

---

## Testing

```bash
# unit tests (domain logic — no external dependencies)
go test ./internal/domain/...

# integration tests (requires Docker)
make integration-tests

# all tests with race detector
make chaos-tests
```

---

## References

- *Implementing Domain-Driven Design* — Vaughn Vernon
- *100 Go Mistakes and How to Avoid Them* — Teiva Harsanyi
- *Concurrency in Go* — Katherine Cox-Buday
- *The Go Programming Language* — Donovan & Kernighan
- [pgx/v5 documentation](https://github.com/jackc/pgx)
- [gRPC Go documentation](https://grpc.io/docs/languages/go/)

---

## License

MIT — see [LICENSE](./LICENSE) for details.
