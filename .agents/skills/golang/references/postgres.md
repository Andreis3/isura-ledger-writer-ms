# Persistência PostgreSQL

O adapter PostgreSQL está organizado em `internal/infra/postgres` e separa conexão, acesso genérico, modelos, repositories, critérios, observabilidade e transações:

```text
postgres.go                 pool pgx e configuração
database/                   Querier, conversão de datas e contexto transacional
model/                      mapeamento domínio ↔ pgtype
repository/                 SQL e implementação dos contratos de domínio
repository/criteria/        filtros parametrizados e locks
repository/observability/   decorators de métricas e tracing
uow/                        Unit of Work, commit, rollback e retry
```

A infraestrutura deve respeitar `transport → application → domain ← infrastructure`: o domínio define os contratos; PostgreSQL implementa esses contratos sem levar tipos de transporte para dentro dos repositories.

## Pool e `postgres.go`

`internal/infra/postgres/postgres.go` é responsável por criar e encapsular o pool `pgxpool.Pool`:

- monta a connection string a partir de `configs.Configs`;
- aplica `MaxConns`, `MinConns`, `MaxConnLifetime`, `MaxConnIdleTime` e health check de 15 segundos;
- define `application_name` no PostgreSQL;
- cria o pool com `pgxpool.NewWithConfig`;
- valida conectividade com `pool.Ping` antes de retornar;
- expõe `Pool`, `Exec`, `Query`, `QueryRow` e `SendBatch`;
- fecha o pool com `Close` durante o shutdown.

Crie o PostgreSQL uma vez em `BuildBaseDeps`; não crie pools dentro de repositories ou factories:

```go
pg, err := postgres.NewPostgres(cfg)
if err != nil {
	log.CriticalText("failed to connect to database", slog.String("error", err.Error()))
	os.Exit(1)
}
```

O pool é compartilhado e seguro para uso concorrente. Operações devem receber o `context.Context` da chamada para respeitar cancelamento e deadlines.

## `database.Querier` e transações

Repositories dependem de `database.Querier`, não diretamente do pool:

```go
type Querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}
```

Essa interface permite que o mesmo repository execute fora ou dentro de uma transação. `database.WithTx` coloca o `pgx.Tx` no contexto; `repository.resolveDB` recupera a transação com `database.ExtractTx` e a prioriza sobre o pool:

```go
db := resolveDB(ctx, r.db)
_, err := db.Exec(ctx, query, args...)
```

Não esconda transações em variáveis globais nem faça um repository abrir uma transação isolada quando a operação pertence a um fluxo coordenado pelo UoW.

## Unit of Work

`internal/infra/postgres/uow/uow.go` fornece:

- `WithTransaction`: begin, execução da função, rollback em erro e commit em sucesso;
- `WithRetryableTransaction`: retry limitado para conflito de concorrência;
- erros classificados por `fault.BeginTransactionError`, `fault.RollbackTransactionError`, `fault.CommitTransactionError` e `fault.ConflictError`.

Uso esperado:

```go
err := uow.WithRetryableTransaction(ctx, func(txCtx context.Context) error {
	if err := accountRepo.Save(txCtx, account); err != nil {
		return err
	}
	return transactionRepo.Save(txCtx, transaction)
})
```

Regras do UoW:

- todos os repositories participantes devem receber o `txCtx` fornecido pela função;
- não use o contexto original dentro da transação, pois isso ignora o `pgx.Tx` armazenado;
- rollback usa timeout de 5 segundos para não ficar bloqueado indefinidamente;
- retries são limitados a 5 tentativas;
- somente a violação PostgreSQL `23505` da constraint `unique_account_sequence` é considerada conflito retryable;
- o backoff usa jitter e limite máximo de 200 ms;
- cancelamento do contexto interrompe a espera entre tentativas;
- não faça retry de erros de validação, indisponibilidade genérica ou falhas não classificadas como conflito.

O UoW não deve conter regra de negócio; ele controla atomicidade e retry de uma condição técnica conhecida.

## Modelos PostgreSQL

Os tipos em `internal/infra/postgres/model` representam linhas e tipos PostgreSQL usando `pgtype`:

- `model.Account` ↔ `account.Account`;
- `model.Balance` ↔ `balance.Balance`;
- `model.Transaction` ↔ `transaction.Transaction`;
- `model.Entry` ↔ `transaction.Entry`;
- `model.Outbox` ↔ `outbox.Outbox`.

Cada modelo deve ter conversores explícitos `To...Model` e `To...Domain`. Campos monetários permanecem em centavos (`pgtype.Int8`/`int64`), e valores opcionais de data usam `pgtype.Timestamptz.Valid`:

```go
func ToTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}
```

Não passe modelos `model.*` para a application. Se a conversão de banco para domínio falhar, retorne o erro; não ignore falhas de ID, moeda ou invariantes do builder.

Ao adicionar uma coluna:

1. atualize o schema/migration;
2. atualize o model PostgreSQL;
3. atualize `To...Model` e `To...Domain`;
4. atualize `INSERT`, `SELECT`, `UPDATE` e `Scan` relacionados;
5. atualize o agregado/construtor de domínio quando necessário;
6. adicione testes para valores válidos, nulos e conversão inválida.

## Repositories

Repositories implementam interfaces do domínio e recebem `database.Querier` no construtor. O fluxo normal é:

```text
contexto → resolveDB → criteria/query parametrizada → pgx Scan/Exec
         → model PostgreSQL → entidade de domínio
```

Regras:

- use placeholders `$1`, `$2`, ...; nunca concatene valores de entrada no SQL;
- use `QueryRow` para uma linha, `Query` para múltiplas linhas e `SendBatch` quando as operações forem agrupadas;
- sempre feche `Rows` e `BatchResults` com `defer` após obtê-los;
- verifique `rows.Err()` depois de iterar;
- converta `pgx.ErrNoRows` para o comportamento definido pelo contrato do caso de uso;
- preserve e classifique erros com `fault` quando a falha atravessar a fronteira de infraestrutura;
- não coloque regra de negócio ou decisões de protocolo em SQL/repository.

O repository de transaction usa batch para inserir a transação e suas entries. O repository de outbox usa `FOR UPDATE SKIP LOCKED` para buscar eventos pendentes sem bloquear workers concorrentes.

## Critérios e locks

Os builders em `repository/criteria` montam filtros opcionais e retornam query mais argumentos separados. Critérios devem continuar parametrizados e devem adicionar somente cláusulas conhecidas pelo código:

- `HasForUpdate`: espera a liberação do registro;
- `HasForUpdateSkipLock`: ignora registros bloqueados e é apropriado para filas/consumidores;
- `WithEntries`: controla a carga relacionada quando suportado pelo repository.

Use `FOR UPDATE` apenas dentro de uma transação ativa quando o lock precisar ser mantido até o commit. Use `FOR UPDATE SKIP LOCKED` somente em fluxos que toleram pular registros já processados por outro worker.

## Erros PostgreSQL

Use `errors.Is` e `errors.AsType[*pgconn.PgError]`, nunca comparação de strings. A função `isUniqueViolation` reconhece o código SQLSTATE `23505`; o repository deve converter essa condição para o erro de domínio adequado, como `fault.SaveAccountAlreadyExistsError`.

```go
if isUniqueViolation(err) {
	return fault.SaveAccountAlreadyExistsError(err)
}
return fault.SaveAccountError(err)
```

Mantenha a causa original encadeada para logs, tracing e diagnóstico. Não exponha SQL, credenciais ou detalhes internos diretamente ao cliente.

## Observabilidade

Os decorators em `repository/observability` devem envolver os repositories no composer, criando spans por operação e registrando duração das queries. Eles devem:

- propagar o contexto recebido;
- chamar `span.RecordError(err)` antes de retornar uma falha;
- registrar métricas com nomes fixos de banco, tabela e método;
- retornar o mesmo resultado/erro do repository concreto;
- não criar conexões, alterar regras de domínio ou esconder falhas.

## Checklist

- O pool é criado somente no bootstrap e fechado no shutdown?
- O repository depende de `database.Querier`?
- O contexto transacional é propagado até todas as operações?
- Models e entidades são convertidos explicitamente?
- SQL usa placeholders e critérios parametrizados?
- Rows e batch results são fechados e seus erros verificados?
- `pgx.ErrNoRows` e `pgconn.PgError` são tratados sem comparar texto?
- A operação usa UoW quando precisa de atomicidade entre repositories?
- Locks e retries estão limitados ao caso de uso correto?
- Decorators de tracing e métricas foram aplicados via composer?

## Verificação

Depois de alterar a infraestrutura PostgreSQL, execute:

```bash
gofmt -w internal/infra/postgres
go vet ./...
go test ./...
```

Para alterações em transações, workers, locks ou concorrência, execute também:

```bash
go test -race ./...
```
