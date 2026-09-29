# Guia de performance em Go

Este guia deriva principalmente de **Efficient Go** e é complementado pelos alertas de otimização de **100 Go Mistakes and How to Avoid Them**.

## Regra principal

Não otimize por intuição. Primeiro estabeleça uma linha de base, meça o comportamento, identifique o gargalo e só então altere o código.

Performance deve ser tratada como um processo orientado por dados:

```text
objetivo -> medição -> perfil -> hipótese -> mudança -> nova medição
```

Uma alteração que torna o código mais complexo sem ganho medido deve ser questionada.

## Benchmarks

Use benchmarks para responder perguntas específicas. Compare alternativas sob condições equivalentes e evite concluir a partir de uma única execução ruidosa.

Ao otimizar um caminho crítico:

- defina a métrica que importa;
- construa benchmark representativo;
- observe tempo e alocações quando pertinentes;
- altere uma variável relevante por vez;
- repita a medição após a mudança.

Não use microbenchmark para provar comportamento de produção que depende de banco, rede, contenção ou scheduler sem reconhecer essa limitação.

## Profiling

Antes de alterar estruturas ou algoritmos por performance, use profiling para localizar o custo real. Dependendo do problema, investigue CPU, heap/alocações, goroutines, bloqueios e mutexes.

Não conclua que uma função é gargalo apenas porque aparece no código crítico; confirme sua contribuição no perfil.

## Alocações e memória

Reduzir alocações pode ajudar caminhos realmente quentes, mas não sacrifique clareza por pequenas economias não medidas.

Ao investigar memória:

- diferencie memória viva de volume de alocações;
- considere pressão sobre o garbage collector;
- investigue retenção inesperada por slices e referências;
- evite manter buffers ou objetos grandes vivos sem necessidade;
- reutilização/pooling só deve ser adotada quando o perfil justificar.

## Concorrência e performance

Concorrência não implica aceleração. Paralelizar trabalho pequeno pode piorar o resultado por sincronização, agendamento e contenção.

Ao aumentar workers, considere também:

- número de CPUs para trabalho CPU-bound;
- capacidade do pool PostgreSQL;
- limites de conexões;
- capacidade do serviço remoto;
- backpressure;
- contenção;
- p95/p99, e não apenas throughput médio.

## Ledger

No ledger, não troque correção por throughput. Alterações no caminho de transação, sequenciamento, isolamento ou idempotência precisam preservar invariantes antes de qualquer ganho de performance.

Para hot accounts, meça taxa de conflito, retries, retry exhaustion e latências de cauda antes de propor locking diferente, single-writer ou particionamento adicional.

## Checklist

Antes de aceitar uma otimização:

- existe baseline anterior;
- o gargalo foi observado;
- há benchmark/profile reproduzível;
- a mudança preserva semântica e testes;
- o ganho é relevante para a métrica real;
- p95/p99 não pioraram de forma indesejada;
- complexidade adicional é proporcional ao ganho;
- uma nova medição confirma o resultado.

## Fontes-base

- Bartlomiej Plotka — *Efficient Go: Data-Driven Performance Optimization*.
- Teiva Harsanyi — *100 Go Mistakes and How to Avoid Them*.
