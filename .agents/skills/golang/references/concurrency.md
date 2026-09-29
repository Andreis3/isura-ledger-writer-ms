# Estado e concorrência no projeto

Este arquivo registra convenções **específicas do repositório**. Para fundamentos, lifecycle de goroutines, channels, cancellation, WaitGroup, backpressure e race conditions, consulte [go-concurrency-guide.md](go-concurrency-guide.md). Para concorrência transacional do ledger, consulte [ledger-engineering-guide.md](ledger-engineering-guide.md).

## Estado compartilhado em memória

Prefira estado imutável depois da construção e variáveis locais. Quando múltiplas goroutines realmente precisarem compartilhar estado mutável, escolha explicitamente o mecanismo de sincronização e mantenha-o próximo do estado protegido.

```go
type Counter struct {
	mu    sync.Mutex
	value int
}

func (c *Counter) Increment() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.value++
	return c.value
}
```

Todas as leituras e escritas do mesmo estado devem obedecer à mesma disciplina de sincronização. Não proteja apenas writes deixando reads concorrentes fora do lock.

Não compartilhe map, slice, buffer ou ponteiro mutável entre workers sem ownership claro, cópia ou sincronização.

## Concorrência em memória não substitui concorrência no banco

Não use `sync.Mutex`, map global de locks ou semaphore dentro da aplicação para proteger invariantes que precisam valer entre múltiplas instâncias do serviço.

No ledger, a concorrência entre writers é coordenada pelo PostgreSQL com `SERIALIZABLE`, constraints e retry otimista. Um mutex de processo não protege contra outra réplica.

Da mesma forma, não introduza `SELECT FOR UPDATE`, advisory lock ou outra estratégia de banco como "equivalente" a um mutex sem seguir a revisão arquitetural definida em `ledger-engineering-guide.md`.

## Workers

Workers devem possuir:

- limite explícito de concorrência;
- caminho de encerramento;
- propagação de cancelamento;
- comportamento conhecido para fila cheia/backpressure;
- tratamento explícito de erro.

Use o contexto recebido durante o processamento. Não substitua por `context.Background()` no meio de um job apenas para impedir cancelamento.

## Verificação

Ao alterar goroutines, workers, sincronização ou shutdown, execute:

```bash
go test -race ./...
```

O race detector complementa os testes; ele não substitui testes determinísticos de invariantes e concorrência.
