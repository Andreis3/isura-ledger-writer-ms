# isura-ledger-ms

The double-entry accounting engine of Isura Bank. **Every financial movement goes through here.**

This service is the financial source of truth — it records all debit and credit entries, maintains real-time account balances, guarantees atomic double-entry bookkeeping, and publishes domain events reliably via the Transactional Outbox pattern.

---

## Responsibilities

- Double-entry bookkeeping — debit + credit = 0, always atomic via a single PostgreSQL transaction
- Real-time balance per account
- Transaction idempotency via `idempotency_key` with `UNIQUE CONSTRAINT`
- Reliable event publishing via Transactional Outbox → NATS JetStream
- Accounting representation of accounts (asset, liability, revenue, expense, equity)

---

## Architecture

This service follows **Hexagonal Architecture** with **tactical Domain-Driven Design**, organized around three bounded contexts internally: `transaction`, `account`, and `outbox`.

### Dependency rule

```
transport → application → domain ← infrastructure
```

The `domain` layer imports nothing external. The `infrastructure` layer implements the interfaces defined in `domain`. The `transport` layer knows only about `application` use cases.

### Project structure

```
## 📂 Estrutura do Projeto

isura-ledger-ms/
├── .github/
│   └── workflows/
│       └── golang-build-test.yaml    # CI/CD pipeline de testes e build
├── .air.toml                         # Configuração do hot reload (Air) para desenvolvimento local
├── .gitignore                        # Arquivos e pastas ignorados pelo Git
├── Dockerfile                        # Build multi-stage otimizado para produção
├── Dockerfile.local                  # Build e container para desenvolvimento local com Air
├── Makefile                          # Comandos automatizados para build, migrações e testes
├── README.md                         # Documentação principal do projeto
├── buf.gen.yaml                      # Configuração de geração de código Go do Buf v2
├── buf.yaml                          # Configuração do módulo Protobuf do Buf v2
├── cmd/
│   └── server/
│       └── main.go                  # Ponto de entrada (bootstrap da aplicação)
├── config.example.json               # Exemplo de configuração de variáveis em JSON
├── config.json                       # Configurações locais da aplicação
├── db/
│   ├── accounts.pg.hcl              # Definição Atlas HCL da tabela de contas
│   ├── entries.pg.hcl               # Definição Atlas HCL da tabela de lançamentos
│   ├── outbox.pg.hcl                # Definição Atlas HCL da tabela outbox
│   ├── schema.pg.hcl                # Esquema público base do banco de dados
│   └── transactions.pg.hcl          # Definição Atlas HCL da tabela de transações
├── docker-compose.yml                # Orquestração de serviços (Postgres, Prometheus, Tempo, Grafana)
├── docker/
│   └── tempo/
│       └── tempo.yaml               # Configuração do Grafana Tempo para rastreamento distribuído
├── go.mod                            # Definição do módulo Go e dependências
├── go.sum                            # Checksums criptográficos das dependências
├── internal/
│   ├── application/
│   │   ├── command/
│   │   │   ├── create_account.go    # Use Case: Criação e validação de conta contábil
│   │   │   ├── create_transaction.go# Use Case: Criação de transações double-entry
│   │   │   └── mask.go              # Helpers de mascaramento para logs seguros
│   │   ├── dto/
│   │   │   └── create_account.go    # Objetos de transferência de dados (Input/Output)
│   │   ├── event/
│   │   │   └── transaction_create.go# Contrato/Payload do evento gerado na Outbox
│   │   ├── logger.go                # Interface/Contrato de logs da aplicação
│   │   ├── metrics.go               # Interface/Contrato de métricas da aplicação
│   │   ├── tracer.go                # Interface/Contrato de traces (OpenTelemetry)
│   │   └── uow.go                   # Interface do padrão Unit of Work
│   ├── domain/
│   │   ├── account/
│   │   │   ├── account.go           # Entidade Account, Builder e regras de saldo
│   │   │   └── repository.go        # Interface do repositório de contas
│   │   ├── fault/
│   │   │   ├── errors.go            # Fábricas e mapeamentos de erros de infra/banco
│   │   │   ├── fault.go             # Engine customizável de erros estruturados
│   │   │   └── sentinel.go          # Erros sentinelas globais do negócio
│   │   ├── money/
│   │   │   └── money.go             # Value Object Money (armazenado em centavos)
│   │   ├── outbox/
│   │   │   ├── outbox.go            # Agregado Outbox e máquina de estados
│   │   │   └── repository.go        # Interface do repositório outbox
│   │   ├── tax/
│   │   │   └── cnpj.go              # Value Object CNPJ com validação de dígitos
│   │   ├── transaction/
│   │   │   ├── entry.go             # Entidade Entry (Débito/Crédito)
│   │   │   ├── repository.go        # Interface do repositório de transações
│   │   │   ├── transaction.go       # Agregado Root Transaction + Máquina de Estados
│   │   │   └── types.go             # IDs fortemente tipados (TransactionID, etc.)
│   │   └── validator/
│   │       └── validator.go         # Utilitários de validação e verificação de campos
│   ├── infra/
│   │   ├── configs/
│   │   │   └── configs.go           # Carregamento de configurações via Viper
│   │   ├── dependency/
│   │   │   ├── base_deps.go         # Inicialização de dependências base de infra
│   │   │   └── composer.go          # Wires e composição dos repositórios
│   │   ├── factory/
│   │   │   └── create_account_factory.go # Fábrica de instanciação do Use Case de contas
│   │   ├── logger/
│   │   │   └── logger.go            # Implementação do slog (JSON/Tint dual handler)
│   │   ├── observability/
│   │   │   ├── otel_tracer.go       # Implementação do Tracer OpenTelemetry
│   │   │   └── prometheus.go        # Registro do subsistema de métricas Prometheus
│   │   ├── postgres/
│   │   │   ├── database/
│   │   │   │   ├── helper.go        # Funções auxiliares de conversão de tipos pgx
│   │   │   │   └── querier.go       # Interface unificada para DB Pool e Transações (Tx)
│   │   │   ├── model/
│   │   │   │   ├── account.go       # Model Postgres para Account
│   │   │   │   ├── entry.go         # Model Postgres para Entry
│   │   │   │   ├── outbox.go        # Model Postgres para Outbox
│   │   │   │   └── transaction.go   # Model Postgres para Transaction
│   │   │   ├── repository/
│   │   │   │   ├── criteria/
│   │   │   │   │   └── account.go   # Construtor dinâmico de critérios (Query & Args) para contas
│   │   │   │   ├── observability/
│   │   │   │   │   ├── account_observability.go     # Decorator para métricas/traces de contas
│   │   │   │   │   ├── outbox_observability.go      # Decorator para métricas/traces de outbox
│   │   │   │   │   └── transaction_observability.go # Decorator para métricas/traces de transações
│   │   │   │   ├── account.go       # Repositório Postgres concreto para contas
│   │   │   │   ├── outbox.go        # Repositório Postgres concreto para outbox
│   │   │   │   ├── resolve_db.go    # Helper para extrair Transação ativa do Contexto
│   │   │   │   └── transaction.go   # Repositório Postgres concreto para transações
│   │   │   ├── uow/
│   │   │   │   └── uow.go           # Implementação concreta do Unit of Work com pgx.Tx
│   │   │   └── postgres.go          # Inicialização, pool e ping da conexão PostgreSQL
│   │   └── server/
│   │       ├── graceful_shutdown.go # Controle de encerramento limpo dos servidores
│   │       ├── grpc_server.go       # Ciclo de vida, interceptors e registro do gRPC
│   │       └── http_server.go       # Ciclo de vida e configuração do servidor HTTP (Chi)
│   └── transport/
│       ├── grpc/
│       │   ├── handler/
│       │   │   ├── create_account_handler.go     # Traduz requisições Protobuf ↔ Command de Conta
│       │   │   └── create_transaction_handler.go # Traduz requisições Protobuf ↔ Command de Transação
│       │   ├── interceptor/
│       │   │   ├── logging.go       # Interceptor gRPC de logs estruturados
│       │   │   ├── metrics.go       # Interceptor gRPC de coleta de métricas
│       │   │   └── tracing.go       # Interceptor gRPC de rastreamento distribuído
│       │   ├── pb/ledger/v1/        # Stubs `.go` gerados automaticamente pelo Protobuf/Buf
│       │   │   ├── account.pb.go
│       │   │   ├── ledger.pb.go
│       │   │   ├── ledger_grpc.pb.go
│       │   │   └── transaction.pb.go
│       │   ├── translator/
│       │   │   └── fault_translator.go # Traduz erros de domínio para gRPC Status Codes
│       │   ├── ledger_module.go     # Módulo gRPC específico do Ledger
│       │   ├── module.go            # Interface base para módulos gRPC
│       │   ├── server.go            # Definição estrutural do LedgerServer gRPC
│       │   └── server_registry.go   # Registrador dinâmico de módulos gRPC
│       └── rest/
│           ├── decoder/
│           │   ├── request.go       # Decodificador de corpo HTTP com validação de erros JSON
│           │   ├── response_error.go   # Serializador padronizado de erros REST
│           │   └── response_sucess.go  # Serializador padronizado de sucesso REST
│           ├── handler/
│           │   ├── create_account_handler.go # Handler HTTP para criação de contas
│           │   └── healthcheck_handler.go    # Handler de Health Check com métricas do runtime Go
│           ├── middleware/
│           │   ├── logging.go       # Middleware HTTP de logs de requisição
│           │   ├── metric.go        # Middleware HTTP de métricas Prometheus
│           │   └── tracing.go       # Middleware HTTP para OpenTelemetry Spans
│           ├── module/
│           │   ├── account_module.go     # Agrupador de rotas REST de contas
│           │   ├── healthcheck_module.go # Agrupador de rotas REST de healthcheck
│           │   └── metrics_module.go     # Exposição do endpoint `/metrics` do Prometheus
│           ├── register.go          # Registrador central de rotas no roteador Chi
│           ├── setup.go             # Inicialização e injeção do roteador Chi REST
│           ├── translator/
│           │   └── fault_translator.go # Mapeia códigos de domínio para HTTP Status Codes
│           └── types/
│               └── route.go         # Tipagens e estruturas auxiliares para roteamento HTTP
├── payload.json                      # Payload de exemplo para testes de requisição
├── prometheus.yml                    # Configuração de scraping do Prometheus
├── proto/
│   └── ledger/
│       └── v1/
│           ├── account.proto        # Contrato Protobuf de contas e enums de tipo/moeda
│           ├── ledger.proto         # Contrato Protobuf dos serviços globais do Ledger
│           └── transaction.proto    # Contrato Protobuf de transações e lançamentos
├── targets.txt                       # Arquivo de alvos para testes de carga com Vegeta
└── tests/
    └── unit/
        └── domain/
            ├── account/
            │   ├── account_test.go   # Testes unitários da entidade Account
            │   └── suite_test.go     # Suíte Ginkgo para Account
            ├── money/
            │   ├── money_test.go     # Testes unitários do Value Object Money
            │   └── suite_test.go     # Suíte Ginkgo para Money
            ├── outbox/
            │   ├── outbox_test.go    # Testes unitários do agregado Outbox
            │   └── suite_test.go     # Suíte Ginkgo para Outbox
            └── transaction/
                ├── suite_test.go     # Suíte Ginkgo para Transaction
                ├── transaction_test.go # Testes unitários do agregado Transaction
                └── types_test.go     # Testes da máquina de estados de transações
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

A `Transaction` always contains exactly two `Entry` records — one `DEBIT` and one `CREDIT` with equal amounts. These invariants are enforced by `AddEntry()` before any persistence occurs.

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

The writer persists each transaction, its entries, and its outbox event atomically. The outbox relay then publishes committed events to NATS JetStream:

```text
CreateTransaction
       │
       └── PostgreSQL transaction
             ├── transactions
             ├── entries (financial postings sequenced per account)
             └── outbox_events
                     │
                     └── OutboxRelay → NATS JetStream
```

All writes happen in one PostgreSQL transaction:

```
BEGIN
  INSERT INTO transactions ...
  INSERT INTO entries ...      (debit)
  INSERT INTO entries ...      (credit)
  INSERT INTO outbox_events ... (event payload)
COMMIT
```

`entries` records financial postings in per-account `sequence_number` order. Each entry stores a derived `running_balance`; the latest entry provides the current balance projection. The `accounts` table stores account identity and policy, not a mutable balance. If the commit fails, neither entries nor the outbox event are persisted. After commit, the relay publishes the event to JetStream.

### Idempotency

Every transaction carries an `idempotency_key`. A `UNIQUE CONSTRAINT` on the `transactions` table ensures that concurrent retries with the same key result in exactly one committed transaction — even if two requests race past the application-level `ExistsByIdempotencyKey` check.

### Unit of Work

The `UnitOfWork` interface wraps the PostgreSQL transaction lifecycle. All repository calls within a use case's `Execute` method receive a `context.Context` carrying the active `pgx.Tx`. Each repository's `resolveDB` method picks the transaction over the connection pool when present.

```go
return c.uow.WithRetryableTransaction(ctx, func(ctxTx context.Context) error {
    // transaction, entries, and outbox use one pgx.Tx
    if err := c.transactionRepository.Save(ctxTx, tx); err != nil {
        return err
    }
    return c.outboxRepository.Save(ctxTx, event)
})
```

### CQRS

Write operations live in `application/command/`, read operations in `application/query/`. Commands return only `error`. Queries return `(Result, error)`.

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
git clone https://github.com/andreis3/isura-ledger-ms
cd isura-ledger-ms

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
| `entries` | Double-entry records — always two per transaction |
| `accounts` | Accounting representation of accounts |
| `outbox_events` | Transactional outbox — pending NATS JetStream events |

### Key constraints

```sql
-- idempotency guarantee
UNIQUE (idempotency_key) ON transactions

-- double-entry integrity
FOREIGN KEY (transaction_id) REFERENCES transactions(id) ON entries

-- outbox ordering
INDEX (status, created_at) ON outbox_events
```

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
go test ./internal/infrastructure/... -tags=integration

# all tests with race detector
go test -race ./...
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
