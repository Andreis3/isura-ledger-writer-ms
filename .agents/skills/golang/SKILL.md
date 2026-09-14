---
name: golang
description: Aplicar as convenções de desenvolvimento Go deste projeto ao criar, alterar, revisar ou testar código Go, incluindo arquitetura, concorrência, erros, graceful shutdown e observabilidade.
metadata:
  short-description: Convenções Go do isura-ledger-writer-ms
---

# Regras de desenvolvimento Go

Estas regras se aplicam a todo código Go deste repositório.

Os exemplos estão separados por contexto. Consulte a referência correspondente quando precisar implementar ou revisar o padrão:

- [Factories e composição](references/factories.md)
- [Composição de repositories](references/composer.md)
- [Contratos e payloads](references/contracts.md)
- [Estado e concorrência](references/concurrency.md)
- [Erros](references/errors.md)
- [Workers e graceful shutdown](references/workers.md)
- [Logger da aplicação](references/logger.md)
- [Métricas Prometheus](references/prometheus.md)
- [Tracing OpenTelemetry](references/tracer.md)
- [Persistência PostgreSQL](references/postgres.md)

As dez boas práticas adicionais estão em [references/additional-practices.md](references/additional-practices.md). Aplique-as quando forem pertinentes ao código alterado.

## Organização

- Escreva código idiomático, execute `gofmt` nos arquivos alterados e mantenha funções pequenas e coesas.
- Evite importações circulares. Use interfaces para respeitar o fluxo `transport → application → domain ← infrastructure`.
- Evite `any` e `interface{}`. Prefira tipos concretos, interfaces pequenas e genéricos com restrições quando o comportamento esperado for conhecido.
- Use `any` somente quando a natureza do valor for realmente dinâmica ou quando uma API externa exigir isso. Nesse caso, limite seu uso à borda da aplicação, valide o valor imediatamente e documente a razão.
- Não use `any` para esconder contratos entre camadas, substituir DTOs, ignorar tipos de retorno ou facilitar um type assertion.
- Toda nova dependência de infraestrutura deve ser instanciada e mantida em `internal/infra/dependency/base_deps.go`. Por exemplo, ao adicionar Redis, crie o cliente no `BaseDeps` e injete-o nas factories.
- Todo novo command deve ter uma factory em `internal/infra/factory`, como `create_account_factory.go`, reunindo repositórios, publishers, logger, tracer e métricas.

## Estado e concorrência

- Não declare na struct um campo que mudará de valor durante a vida do objeto sem definir sua sincronização. Prefira estado imutável após a construção e variáveis locais.
- Ao compartilhar estado, proteja todas as leituras e escritas com `sync.Mutex`, `sync.RWMutex`, `atomic` ou canais.
- Não compartilhe mapas, slices ou ponteiros mutáveis sem copiar ou sincronizar o acesso.
- Execute `go test -race ./...` quando alterar código concorrente.

## Erros

- Sempre trate erros. Não ignore retornos com `_` nem use blocos `if err != nil` vazios.
- Adicione contexto ao propagar erros: `fmt.Errorf("salvar lançamento: %w", err)`.
- Em `Close`, `Shutdown` e `Flush`, trate também os erros de limpeza.

## Goroutines e graceful shutdown

- Servidores, consumidores e workers devem implementar graceful shutdown com `context.Context`, `signal.NotifyContext`, `sync.WaitGroup` ou `errgroup`.
- Nunca crie uma goroutine por item sem limite. Use worker pool, semaphore ou fila com capacidade limitada e aplique backpressure ou rejeição controlada.

No encerramento, pare de aceitar trabalho novo, cancele consumidores e feche conexões, exporters e filas dentro de um timeout.

## Logs e observabilidade

- Logs de fluxo da aplicação e entrada de commands devem ser JSON estruturado, com campos como `command`, `request_id`, `trace_id`, `account_id` e `error`.
- Logs informativos operacionais, como a porta do servidor, devem ser TEXT legível.
- Nunca registre senhas, tokens, credenciais ou dados sensíveis.
- Use spans do OpenTelemetry em operações relevantes, propague o contexto, registre falhas com `span.RecordError(err)` e marque o status.
- Exponha métricas Prometheus para requests, erros, duração e jobs. Use labels de baixa cardinalidade; nunca use IDs de usuário, conta ou request como labels.

## Verificação

Quando aplicável, execute:

```bash
gofmt -w .
go vet ./...
go test ./...
go test -race ./...
```
