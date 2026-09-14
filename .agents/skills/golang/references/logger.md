# Logger da aplicação

O logger da aplicação está em `internal/infra/logger/logger.go`. Ele encapsula dois `slog.Logger`: JSON para produção/observabilidade e texto colorido para desenvolvimento no terminal.

## Inicialização

Crie o logger no bootstrap, por meio de `logger.NewLogger()`, e injete `*logger.Logger` nas dependências da aplicação. Não crie loggers dentro de commands, repositories ou handlers.

```go
log := logger.NewLogger()
baseDeps := &dependency.BaseDeps{
	Log: log,
}
```

`NewLogger` também configura o logger global do `slog`:

- `ENV=development`: usa o handler `tint` em `stderr`, com saída legível e colorida;
- qualquer outro ambiente: usa `slog.JSONHandler` em `stdout`, adequado para coleta por Loki/observabilidade.

Os dois handlers aceitam nível mínimo `DEBUG`. O timestamp é formatado como `01-02-2006 15:04:05.000` e os níveis JSON são normalizados para `DEBUG`, `INFO`, `WARN`, `ERROR` e `CRITICAL`.

## Escolha do formato

Use os métodos `*JSON` e `*Text` quando o destino precisar ser explícito:

```go
log.InfoJSON("command recebido",
	"command", "CreateAccount",
	"request_id", requestID,
)

log.InfoText("servidor HTTP ouvindo", "port", port)
```

Para fluxo de aplicação e entrada de commands, prefira JSON estruturado. Para mensagens operacionais de desenvolvimento ou bootstrap local, use texto legível. Não misture dados sensíveis ou credenciais nos atributos.

## Níveis e atributos

Use mensagens curtas e estáveis, com atributos nomeados:

- `Debug`: diagnóstico detalhado, sem impacto operacional;
- `Info`: eventos normais de ciclo de vida e negócio;
- `Warn`: rejeições esperadas, degradação ou situações que exigem atenção;
- `Error`: falhas de operação ou de uma requisição;
- `Critical`: falhas que impedem inicialização ou funcionamento do serviço.

```go
log.ErrorJSON("falha ao salvar conta",
	"command", "CreateAccount",
	"account_id", accountID,
	"error", err,
)
```

Não concatene strings para esconder estrutura. Use pares chave-valor ou `slog.Attr` explícito. Mantenha nomes de campos consistentes, como `command`, `request_id`, `trace_id`, `account_id` e `error`.

## Tracing

Use `WithTrace(ctx)` quando um log JSON precisar carregar o contexto do OpenTelemetry:

```go
log.WithTrace(ctx).Error("falha ao executar command",
	"command", "CreateAccount",
	"error", err,
)
```

Quando o contexto não contém um span válido, `WithTrace` retorna o logger JSON sem adicionar IDs. O método adiciona `trace_id` e `span_id` somente quando esses valores existem; não gere IDs manualmente.

Para erros de domínio, combine o logger com `fault.Attrs(err)` para registrar `error_code`, causa, campos e origem de forma estruturada:

```go
log.WithTrace(ctx).Error("falha no command",
	"command", "CreateAccount",
	fault.Attrs(err)...,
)
```

## Dados sensíveis

Nunca registre senhas, tokens, credenciais ou payloads brutos que contenham dados sensíveis. Quando um DTO precisar ser registrado, implemente `slog.LogValuer` para selecionar e mascarar campos antes da saída. O projeto usa esse padrão nos DTOs de entrada de account e transaction.

```go
log.WithTrace(ctx).Info("command recebido", "input", input)
```

Esse exemplo só é seguro quando `input` implementa `LogValue` com mascaramento apropriado. Caso contrário, registre apenas identificadores não sensíveis e campos necessários ao diagnóstico.

## Critical e encerramento

`CriticalJSON` e `CriticalText` usam o nível customizado `slog.LevelError + 1`. Use-os para falhas de startup, como configuração inválida ou indisponibilidade obrigatória de PostgreSQL/NATS:

```go
if cfg == nil {
	log.CriticalText("failed to load config")
	os.Exit(1)
}
```

Não use `Critical` para erros normais de requests ou para substituir o tratamento de erros. O log crítico não encerra o processo por si só; a decisão de abortar o bootstrap deve permanecer no lifecycle da aplicação.

## Acesso direto aos handlers

`SlogJSON()` e `SlogText()` existem para integrações que exigem diretamente `*slog.Logger`, como middleware HTTP/gRPC. Use-os apenas quando a API consumidora exigir esse tipo; para código comum, prefira os métodos do wrapper `Logger` ou `WithTrace`.

```go
loggingMiddleware := middleware.Logging(baseDeps.Log.SlogJSON())
```

## Checklist

- O logger foi criado no bootstrap e injetado, sem estado global adicional?
- O formato escolhido corresponde ao ambiente e ao destino?
- A mensagem é estável e os dados estão em atributos estruturados?
- O contexto de tracing foi propagado com `WithTrace(ctx)` quando aplicável?
- Erros de domínio usam `fault.Attrs`?
- Nenhum segredo ou dado sensível é registrado?
- `Critical` está restrito a falhas de startup ou indisponibilidade fatal?
