# Composição de repositories e publishers

O arquivo `internal/infra/dependency/composer_repository.go` centraliza a composição dos adapters de persistência e publicação usados pela aplicação. Ele transforma clientes de infraestrutura presentes em `BaseDeps` em implementações dos contratos definidos nos pacotes de domínio.

## Responsabilidades

- Manter `Composer` com uma referência às dependências-base (`*BaseDeps`), sem criar clientes de infraestrutura por conta própria.
- Expor métodos `Build...` que retornam interfaces do domínio, como `account.Repository`, `balance.Repository`, `transaction.Repository` e `outbox.Repository`.
- Construir o repository PostgreSQL concreto.
- Envolvê-lo com o decorator de observabilidade correspondente, injetando métricas e tracer.
- Compor publishers de infraestrutura como `event.Publisher` quando o caso de uso precisar deles.

As conexões e clientes compartilhados são criados em `internal/infra/dependency/base_deps.go`. O composer apenas monta adapters sobre esses clientes. As factories em `internal/infra/factory` usam o composer para conectar esses adapters aos commands.

## Fluxo de composição

```text
BaseDeps
  ├── Pg, Prom, Tracer ──> repository PostgreSQL ──> decorator de observabilidade
  └── Nats, Tracer ──────> publisher JetStream
                                  ↓
                          contrato do domínio
```

## Repository com observabilidade

O padrão obrigatório é construir primeiro o repository concreto e depois aplicar o decorator:

```go
func (c *Composer) BuildAccountRepo() account.Repository {
	return observability.NewObservabilityAccountRepo(
		repository.NewAccountRepository(c.deps.Pg),
		c.deps.Prom,
		c.deps.Tracer,
	)
}
```

O método retorna `account.Repository`, e não o tipo concreto. Isso mantém a aplicação dependente do contrato de domínio e permite trocar a implementação sem alterar commands.

Os repositories atualmente compostos são:

| Método | Contrato | Implementação | Decorator |
|---|---|---|---|
| `BuildAccountRepo` | `account.Repository` | `repository.NewAccountRepository` | `NewObservabilityAccountRepo` |
| `BuildBalance` | `balance.Repository` | `repository.NewBalanceRepository` | `NewObservabilityBalanceRepo` |
| `BuildTransactionRepo` | `transaction.Repository` | `repository.NewTransactionRepository` | `NewObservabilityTransactionRepo` |
| `BuildOutboxRepo` | `outbox.Repository` | `repository.NewOutBoxRepository` | `NewObservabilityOutboxRepo` |

Não retorne diretamente `repository.New...Repository` se o repository tiver um decorator de observabilidade correspondente. O decorator é responsável por spans, duração das queries e registro de falhas.

## Publisher JetStream

Publishers também podem ser compostos como contratos de domínio/evento:

```go
func (c *Composer) BuildNatsPublisher() event.Publisher {
	return nats.NewJetStreamPublisher(c.deps.Nats.JS, c.deps.Tracer)
}
```

Use o `Nats.JS` já inicializado em `BaseDeps`; não abra uma nova conexão no composer ou no command.

## Ao adicionar um repository

1. Defina ou confirme o contrato no pacote de domínio consumidor, com uma interface pequena.
2. Implemente o adapter concreto em `internal/infra/postgres/repository`.
3. Crie o decorator de observabilidade em `internal/infra/postgres/repository/observability` quando o repository precisar de métricas e tracing.
4. Adicione um método `Build...` em `composer_repository.go` que retorne a interface do domínio.
5. Injete o método nas factories que compõem o command.
6. Preserve o fluxo de dependências `transport → application → domain ← infrastructure`.

Não coloque regras de negócio, validação de entidade, tratamento de protocolo ou decisão de transação no composer. Ele é um assembler de dependências.

## Checklist de revisão

- O método retorna uma interface de domínio, não uma implementação concreta?
- O cliente usado vem de `BaseDeps`?
- O decorator de observabilidade foi aplicado?
- Métricas e tracer foram encaminhados ao decorator?
- O novo método está sendo consumido pela factory apropriada?
- O composer permanece livre de estado global, regras de negócio e lógica de transporte?

## Verificação

Depois de alterar o composer, execute:

```bash
gofmt -w internal/infra/dependency/composer_repository.go
go test ./...
go vet ./...
```

Se a alteração afetar tracing, métricas ou concorrência, execute também:

```bash
go test -race ./...
```
