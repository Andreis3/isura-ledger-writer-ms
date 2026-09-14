# Tracing OpenTelemetry

O tracing da aplicação é implementado em `internal/infra/observability/otel_tracer.go`. A infraestrutura usa OTLP HTTP para exportar spans ao collector e expõe à aplicação os contratos pequenos de `application.Tracer`, `application.Span` e `application.SpanContext`.

## Inicialização e lifecycle

Inicialize o tracer uma única vez no bootstrap, usando a configuração da aplicação:

```go
tracer, tracerShutdown, err := observability.InitOtelTracer(
	context.Background(),
	cfg,
)
if err != nil {
	log.CriticalText("failed to initialize tracer", slog.String("error", err.Error()))
	os.Exit(1)
}

baseDeps := &dependency.BaseDeps{
	Tracer:         tracer,
	TracerShutdown: tracerShutdown,
}
```

`InitOtelTracer` configura:

- exporter OTLP HTTP com `cfg.OpenTelemetry.Host`;
- conexão sem TLS, apropriada à configuração interna atual;
- resource com nome e versão da aplicação, além de processo, sistema operacional e host;
- `TracerProvider` global do OpenTelemetry;
- propagação W3C `TraceContext` e `Baggage`;
- processamento em lote com fila máxima de 4096 spans e lote máximo de 512 spans.

No shutdown, pare novos trabalhos e execute a função retornada por `InitOtelTracer` dentro do contexto de encerramento. Trate o erro de `Shutdown`, pois ele faz flush dos spans pendentes:

```go
if err := deps.TracerShutdown(closeCtx); err != nil {
	deps.Log.ErrorContext(closeCtx, "failed to shutdown tracer", "error", err)
}
```

Não crie providers, exporters ou tracers dentro de handlers, commands ou repositories.

## Amostragem

O sampler atual usa `TraceIDRatioBased`:

- ambiente padrão: `100%` (`1.0`);
- `dev`, `staging` e `load-test`: `5%` (`0.05`).

Preserve a amostragem baixa em cenários de carga para evitar pressão desnecessária no collector. Qualquer mudança deve considerar volume, custo de exportação e necessidade de diagnóstico.

## Criando spans

Sempre passe o contexto recebido para `Start`, use o contexto retornado nas chamadas seguintes e finalize o span com `defer span.End()`:

```go
func (c *CreateAccount) Execute(ctx context.Context, input Input) error {
	ctx, span := c.tracer.Start(ctx, "CreateAccount.Execute")
	defer span.End()

	if err := c.repo.Save(ctx, account); err != nil {
		span.RecordError(err)
		return err
	}
	return nil
}
```

Os nomes de span devem ser estáveis e descrever a operação, seguindo os padrões existentes:

- `CreateAccountHandler.Handle`;
- `CreateAccount.Execute`;
- `AccountRepository.Save`;
- `Publisher.Publish`;
- `NatsConsumerServer.ProcessWorker`.

Não inclua IDs, tokens, chaves de idempotência ou valores dinâmicos no nome do span. Registre esses dados em logs, quando seguros, ou como atributos de baixa sensibilidade se houver necessidade real de análise.

## Propagação de contexto

O contexto retornado por `Start` deve ser propagado para repositories, publishers, workers e demais operações descendentes:

```go
ctx, span := tracer.Start(ctx, "Publisher.Publish")
defer span.End()

if err := publisher.Publish(ctx, event); err != nil {
	span.RecordError(err)
	return err
}
```

Não substitua o contexto por `context.Background()` durante uma requisição ou job. Isso quebra a relação entre spans e impede cancelamento e correlação. Use `context.Background()` apenas na inicialização do provider, conforme o bootstrap atual, ou em um shutdown controlado.

## Registro de erros

Registre no span os erros que fazem a operação falhar antes de retorná-los:

```go
result, err := repository.Find(ctx, criteria)
if err != nil {
	span.RecordError(err)
	return nil, err
}
return result, nil
```

O contrato atual de `application.Span` expõe `End`, `RecordError` e `SpanContext`, mas não expõe `SetStatus`. Não acesse o SDK diretamente para contornar essa abstração; se a aplicação precisar de status explícito, altere o contrato e sua implementação de forma coordenada.

## Integração com logs

Use `span.SpanContext().TraceID()` ou `logger.WithTrace(ctx)` para correlacionar traces e logs. Prefira `WithTrace(ctx)` quando estiver usando o logger da aplicação:

```go
ctx, span := tracer.Start(ctx, "CreateTransaction.Execute")
defer span.End()

log.WithTrace(ctx).Info("executando command", "command", "CreateTransaction")
```

Não use `trace_id` ou `span_id` como labels Prometheus. Eles têm alta cardinalidade e devem permanecer em traces e logs.

## Instrumentação por camada

- Transporte: crie spans para requests HTTP, métodos gRPC e handlers de fila.
- Application: crie spans para execução de commands e operações relevantes.
- Infrastructure: crie spans para repositories, publishers e workers.
- Domain: mantenha as regras puras; não faça o domínio depender do SDK ou de `application.Tracer` sem necessidade arquitetural.

Evite instrumentar a mesma operação várias vezes na mesma camada sem uma distinção útil. Spans devem ajudar a localizar latência e falhas, não apenas aumentar o volume de telemetria.

## Checklist

- O tracer é criado no bootstrap e injetado via `BaseDeps`?
- O contexto retornado por `Start` é propagado?
- Todo span criado tem `defer span.End()`?
- Erros retornados são registrados com `span.RecordError(err)`?
- Os nomes de spans são estáveis e sem dados dinâmicos?
- A amostragem é adequada ao ambiente e ao volume esperado?
- O shutdown executa e verifica `TracerShutdown`?
- Logs usam `trace_id`/`span_id` para correlação sem transformar esses valores em labels métricos?

## Verificação

Depois de alterar o tracer ou sua composição, execute:

```bash
gofmt -w internal/infra/observability/otel_tracer.go
go test ./...
go vet ./...
```

Se a alteração envolver workers, propagação concorrente ou shutdown:

```bash
go test -race ./...
```
