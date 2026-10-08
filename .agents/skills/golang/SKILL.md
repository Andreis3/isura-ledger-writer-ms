---
name: golang
description: Aplicar as convenções de desenvolvimento Go deste projeto ao criar, alterar, revisar ou testar código Go, incluindo arquitetura, concorrência, erros, graceful shutdown e observabilidade.
metadata:
  short-description: Convenções Go do isura-ledger-writer-ms
---

# Regras de desenvolvimento Go

Estas regras se aplicam a todo código Go deste repositório.

Os exemplos estão separados por contexto. Consulte somente as referências pertinentes à tarefa.

## Hierarquia de autoridade

Quando duas orientações entrarem em tensão, use esta precedência:

1. decisões e invariantes explícitas do Isura Ledger;
2. regras específicas deste repositório e do `AGENTS.md`;
3. guias de engenharia derivados dos livros;
4. práticas Go genéricas.

Uma recomendação genérica não deve sobrescrever silenciosamente uma decisão arquitetural consciente do ledger. Mudanças nessas decisões exigem análise de trade-offs e, quando duradouras, ADR.

## Referências específicas do repositório

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

Para decisões envolvendo SOLID, coesão e abstrações, consulte também [SOLID aplicado a Go](../solid-go/SKILL.md).

## Guias de engenharia

- [Concorrência em Go — guia baseado nos livros](references/go-concurrency-guide.md)
- [Performance em Go — guia baseado nos livros](references/go-performance-guide.md)
- [Tipos, interfaces e generics — guia baseado nos livros](references/go-types-generics-guide.md)
- [DDD e arquitetura — guia baseado nos livros](references/ddd-architecture-guide.md)
- [Serviços, resiliência e operação — guia baseado nos livros](references/service-resilience-guide.md)
- [Engenharia do Isura Ledger — invariantes e decisões do ledger](references/ledger-engineering-guide.md)

O `ledger-engineering-guide.md` é a referência principal quando a alteração tocar saldo, entries, sequenciamento, idempotência, concorrência transacional, UoW, outbox ou reconciliação.

As dez boas práticas adicionais estão em [references/additional-practices.md](references/additional-practices.md). Aplique-as quando forem pertinentes ao código alterado.

## Base de conhecimento derivada dos livros

As referências com sufixo `-guide.md` consolidam princípios dos livros usados como base de engenharia do projeto em regras operacionais para o agente. Consulte somente as referências pertinentes à tarefa para evitar carregar contexto desnecessário.

Esses guias complementam as regras específicas do repositório. Em caso de conflito, siga a ordem de autoridade definida no `AGENTS.md` e preserve as decisões arquiteturais já estabelecidas no projeto. Não aplique uma recomendação genérica de livro de forma automática quando ela violar uma invariância ou decisão explícita do ledger.

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
- Em alterações de concorrência, execute `make unit` e `make unit-verbose` (race detector nos testes); considere `make run-race` para validar o processo em execução.

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
- Use spans do OpenTelemetry em operações relevantes, propague o contexto, registre falhas com `span.RecordError(err)`. Não acesse o SDK diretamente para contornar a abstração de tracing.
- Exponha métricas Prometheus para requests, erros, duração e jobs. Use labels de baixa cardinalidade; nunca use IDs de usuário, conta ou request como labels.

## Verificação

Quando aplicável, execute:

```bash
gofmt -w <arquivos-go-alterados>
make vet
make unit
make unit-verbose      # concorrência, quando aplicável
make unit-cover        # cobertura, quando aplicável
make integration-tests # integração, quando aplicável
```
