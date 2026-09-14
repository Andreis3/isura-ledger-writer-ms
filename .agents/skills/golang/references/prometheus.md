# Métricas Prometheus

As métricas da aplicação são implementadas em `internal/infra/observability/prometheus.go` com OpenTelemetry Metrics e exportadas pelo exporter Prometheus. A implementação é encapsulada em `observability.Prometheus` e injetada pelas dependências-base.

## Inicialização e lifecycle

Crie uma única instância no bootstrap, por meio de `observability.NewPrometheus()`, e injete-a em `BaseDeps`:

```go
prom, err := observability.NewPrometheus()
if err != nil {
	log.CriticalText("failed to initialize Prometheus", slog.String("error", err.Error()))
	os.Exit(1)
}

baseDeps := &dependency.BaseDeps{Prom: prom}
```

O meter usa:

- nome: `isura-ledger-ms`;
- versão: `1.0.0`;
- exporter Prometheus;
- recurso OpenTelemetry com nome e versão do serviço.

Não crie instrumentos ou exporters dentro de handlers, commands ou repositories. O `MeterProvider` pertence ao lifecycle da aplicação e deve ser encerrado no shutdown com `Prometheus.Close()`.

## Instrumentos existentes

| Instrumento | Tipo | Unidade | Labels |
|---|---|---|---|
| `ledger_requests_total` | Counter | contagem | `router`, `status_code`, `protocol` |
| `ledger_db_query_duration_milliseconds` | Histogram | milissegundos | `database`, `table`, `method` |
| `ledger_grpc_request_duration_milliseconds` | Histogram | milissegundos | `router`, `status_code`, `protocol` |
| `ledger_transactions_total` | Counter | contagem | `status` |
| `ledger_command_duration_milliseconds` | Histogram | milissegundos | `command` |
| `ledger_command_total` | Counter | contagem | `command`, `state` |

As durações usam buckets explícitos de 5 ms até 100.000 ms. Preserve milissegundos ao registrar valores e use o mesmo padrão de unidade do instrumento.

## Métodos de registro

Use os métodos da abstração `Prometheus` ou da interface `application.Metrics`; não acesse os instrumentos privados diretamente.

```go
metrics.RecordRequestTotal(routeIdentifier, "http", statusCode)
metrics.RecordRequestDuration(
	routeIdentifier,
	"http",
	statusCode,
	float64(time.Since(start).Milliseconds()),
)
```

Para queries, registre valores estáveis de banco, tabela e operação:

```go
start := time.Now()
defer func() {
	metrics.RecordDBQueryDuration(
		"postgres",
		"accounts",
		"find_by_id",
		float64(time.Since(start).Milliseconds()),
	)
}()
```

Para commands, use nomes estáveis e estados previamente definidos, como `CreateAccount` com `failure` ou `exist`. Não use mensagens de erro, IDs, chaves de idempotência ou dados de usuário como labels.

```go
start := time.Now()
defer func() {
	metrics.RecordCommandDuration(
		"CreateAccount",
		float64(time.Since(start).Milliseconds()),
	)
}()

metrics.RecordCommandTotal("CreateAccount", "failure")
```

## Cardinalidade

Labels devem ter cardinalidade baixa e conjunto previsível. São apropriados:

- protocolo (`http`, `grpc`);
- status numérico ou categorias limitadas;
- nomes fixos de rotas, commands, tabelas e métodos;
- estados finitos de transação/command.

São proibidos como labels:

- `account_id`, `transaction_id`, `request_id` ou `trace_id`;
- CPF/CNPJ, idempotency key ou qualquer identificador de usuário;
- mensagem de erro ou stack trace;
- URL completa com parâmetros dinâmicos.

Esses dados pertencem a logs e traces, não às séries Prometheus.

## Decorators de repository

Os decorators em `internal/infra/postgres/repository/observability` devem registrar duração e tracing ao redor da chamada real, preservando o contrato do repository:

```go
start := time.Now()
defer func() {
	r.metric.RecordDBQueryDuration(
		"postgres",
		"accounts",
		"save",
		float64(time.Since(start).Milliseconds()),
	)
}()

err := r.repo.Save(ctx, account)
if err != nil {
	span.RecordError(err)
	return err
}
return nil
```

O decorator não deve alterar erros, aplicar regras de domínio ou criar novas conexões. A composição dele ocorre em `internal/infra/dependency/composer_repository.go`.

## Contexto e custo

A implementação atual registra os instrumentos usando `context.Background()`. Não use métricas para transportar contexto, logs ou informações de request. Para correlação, use OpenTelemetry tracing e logs estruturados.

Evite criar atributos dinamicamente dentro de loops de alta frequência quando os valores puderem ser pré-calculados. Prefira métodos pequenos e nomes de label constantes.

## Checklist

- A métrica foi adicionada ao `Prometheus` e inicializada em `NewPrometheus`?
- O nome, tipo, unidade e descrição do instrumento estão definidos?
- Os valores registrados usam milissegundos quando aplicável?
- Todos os labels têm cardinalidade baixa e previsível?
- A métrica está exposta pela interface `application.Metrics` quando é usada por uma camada interna?
- O registro acontece no middleware/decorator correto, sem duplicação?
- O encerramento do `MeterProvider` permanece no lifecycle da aplicação?
- Foram evitados IDs, segredos e mensagens dinâmicas como labels?

## Verificação

Depois de alterar métricas, execute:

```bash
gofmt -w internal/infra/observability/prometheus.go
go test ./...
go vet ./...
```

Valide também as séries exportadas no endpoint de métricas e, quando a alteração envolver decorators ou concorrência:

```bash
go test -race ./...
```
