# Guia de concorrência em Go

Este guia transforma em regras operacionais os princípios de concorrência presentes em **Concurrency in Go**, **100 Go Mistakes and How to Avoid Them** e **Learn Go with Pocket-Sized Projects**. Ele complementa as convenções específicas do repositório; não as substitui.

## Antes de adicionar concorrência

Concorrência trata da estrutura e coordenação de tarefas; paralelismo trata da execução simultânea. Não presuma que uma solução concorrente será mais rápida. Para trabalho pequeno, o custo de sincronização e agendamento pode superar o benefício. Quando desempenho for a justificativa, compare a solução sequencial e a concorrente com benchmark.

Determine antes de criar uma goroutine:

- quem é responsável pelo ciclo de vida dela;
- como ela termina;
- como recebe cancelamento;
- como erros são propagados;
- quais dados são compartilhados;
- qual é o limite de concorrência;
- como ocorre backpressure.

A rotina que cria uma goroutine deve garantir que ela possa terminar. Não introduza goroutines que dependam indefinidamente de um send/receive sem caminho de cancelamento.

## Context

Use `context.Context` para deadline, timeout e cancelamento que atravessam a operação. Quando um send/receive em channel pode bloquear e existe contexto cancelável, use `select` para que `ctx.Done()` possa interromper a espera.

Não armazene contextos em entidades ou structs de domínio. Propague-os pela chamada.

## Channels

Deixe explícita a direção quando possível:

```go
func consume(ch <-chan Event)
func produce(ch chan<- Event)
```

Se uma função precisa ler e escrever no mesmo channel, reavalie se ela acumulou responsabilidades.

Defina ownership: quem cria, escreve, lê e fecha. Em geral, o produtor responsável por encerrar a produção é quem fecha o channel.

Um channel não é automaticamente um mecanismo de broadcast: uma mensagem é consumida por um receiver. Não use múltiplos receivers esperando que todos observem cada mensagem.

## Mutex ou channel

Use sincronização de memória quando o problema é proteger estado compartilhado em um escopo pequeno e bem definido. Mantenha o mutex próximo do estado protegido e não o exponha como parte do contrato.

Use channels quando o problema principal é coordenação, fluxo e ownership de trabalho entre goroutines. Não force channels quando um mutex simples expressa melhor a exclusão necessária.

## WaitGroup e errgroup

Faça o registro no `WaitGroup` antes de iniciar as goroutines que serão aguardadas. Cada goroutine deve sinalizar término de forma segura, normalmente com `defer wg.Done()`.

Quando for necessário propagar erros entre tarefas concorrentes, prefira uma abstração que modele essa propagação explicitamente, como `errgroup`, em vez de combinar WaitGroup com canais de erro ad hoc.

## Limites e backpressure

Não crie uma goroutine por item sem limite. Para trabalho CPU-bound, o paralelismo útil é limitado pelos recursos de CPU. Para I/O-bound, o limite depende também da capacidade dos serviços externos, pools, quotas e latência.

Workers, semáforos e filas devem possuir limites explícitos. Uma fila sem limite apenas desloca o problema de sobrecarga para memória e latência.

## Race conditions

Data race e race condition não são sinônimos. Um programa sem data race ainda pode produzir comportamento incorreto dependente de ordem/timing.

Sempre que alterar código concorrente, execute:

```bash
go test -race ./...
```

A ausência de alerta do race detector não prova que não exista race; os caminhos concorrentes relevantes precisam ser exercitados pelos testes.

## Checklist de revisão

Antes de concluir código concorrente, verifique:

- toda goroutine possui caminho de término;
- cancelamento interrompe operações bloqueantes relevantes;
- não há map/slice/ponteiro mutável compartilhado sem sincronização;
- ownership dos channels está claro;
- não existe send para channel sem receiver possível após shutdown;
- limites de workers e filas são explícitos;
- erros não desaparecem dentro de goroutines;
- shutdown espera ou cancela trabalho de forma controlada;
- o race detector foi executado quando aplicável.

## Fontes-base

- Katherine Cox-Buday — *Concurrency in Go: Tools and Techniques for Developers*.
- Teiva Harsanyi — *100 Go Mistakes and How to Avoid Them*.
- Aliénor Latour, Donia Chaiehloudj et al. — *Learn Go with Pocket-Sized Projects*.
