# Guia de engenharia do Isura Ledger

Este documento traduz as decisões atuais do **Isura Ledger Writer** em regras operacionais para implementação e revisão. Ele deve ser lido junto com os demais guias da skill Go, especialmente `ddd-architecture-guide.md`, `go-concurrency-guide.md`, `go-performance-guide.md` e `postgres.md`.

O objetivo não é impor um rótulo arquitetural. O objetivo é preservar as invariantes financeiras e de concorrência que o código atual implementa.

## Modelo mental do ledger

Considere quatro responsabilidades distintas:

```text
accounts
  -> identidade da conta e configuração contábil

transactions
  -> envelope da operação, status, valor, operação e idempotência

entries
  -> lançamentos contábeis append-only por conta

outbox
  -> eventos de integração persistidos atomicamente com a operação
```

O saldo de uma conta não é mantido por `UPDATE` em uma coluna mutável de saldo. Cada lançamento registra o `running_balance` resultante daquele ponto do stream da conta.

Para obter o estado confirmado mais recente:

```sql
SELECT sequence_number, running_balance
FROM entries
WHERE account_id = $1
ORDER BY sequence_number DESC
LIMIT 1
```

Portanto, `running_balance` é um snapshot imutável derivado do lançamento, não uma segunda fonte mutável de saldo.

## Invariantes fundamentais

Mudanças no ledger devem preservar simultaneamente:

1. lançamentos persistidos são append-only;
2. `amount` é positivo;
3. `direction` é `DEBIT` ou `CREDIT`;
4. moeda do lançamento é válida e compatível com a conta;
5. cada conta possui uma sequência linear de lançamentos;
6. `sequence_number` cresce monotonamente dentro da conta;
7. não podem existir dois lançamentos com o mesmo `(account_id, sequence_number)`;
8. `running_balance` deve corresponder à aplicação determinística do lançamento sobre o estado anterior;
9. contas com `BALANCE_NON_NEGATIVE` não podem confirmar saldo negativo;
10. overflow de `int64` em saldo ou sequência deve falhar;
11. uma requisição idempotente não pode produzir duas transações financeiras distintas;
12. falha parcial não pode confirmar apenas parte da operação contábil.

Não enfraqueça essas invariantes para simplificar repository, SQL, testes ou aumentar throughput.

## Semântica contábil

A regra de impacto no saldo pertence ao domínio.

No modelo atual:

| Tipo de conta | Débito | Crédito |
| --- | --- | --- |
| `ASSET` | aumenta | diminui |
| `EXPENSE` | aumenta | diminui |
| `LIABILITY` | diminui | aumenta |
| `REVENUE` | diminui | aumenta |
| `EQUITY` | diminui | aumenta |

A implementação de referência está em `account.Account.ApplyEntry`.

Não duplique essa tabela semântica em handler, command, repository ou SQL. Persistência pode proteger constraints estruturais, mas a decisão contábil continua pertencendo ao domínio.

## LedgerState

`account.LedgerState` representa o último estado confirmado conhecido de uma conta:

```go
type LedgerState struct {
    AccountID      string
    SequenceNumber int64
    RunningBalance int64
}
```

`ApplyEntry` recebe um estado e retorna um novo estado. Preserve esse comportamento sem mutação do estado recebido.

A sequência e o saldo de uma nova entry só devem ser atribuídos depois que todas as validações necessárias daquela aplicação forem concluídas.

`ApplyHistoricalEntry` possui propósito diferente: reconstrução/reconciliação histórica. Ele pode recalcular um estado persistido sem aplicar a política financeira atual de saldo não negativo, mas ainda deve validar configuração contábil, direção, valor e moeda. Não use esse método no fluxo normal para contornar rejeições de saldo.

## Append-only

Não implemente `UPDATE` ou `DELETE` de entries como mecanismo normal de correção financeira.

Uma correção contábil deve ser representada por novos lançamentos compatíveis com a regra de negócio correspondente.

Mudanças administrativas de dados históricos, migrations ou ferramentas de reparo exigem fluxo explícito, auditável e separado do caminho normal de escrita.

## Sequenciamento por conta

A ordem relevante é por conta, não uma sequência global do sistema.

O banco protege essa propriedade com:

```text
UNIQUE(account_id, sequence_number)
```

A constraint `unique_entry_sequence_number` é parte do mecanismo de concorrência e não apenas uma validação de dados.

Não remova, relaxe ou substitua essa constraint sem redesenhar e testar a estratégia de concorrência do ledger.

## Concorrência otimista

O fluxo atual é intencionalmente otimista:

```text
ler último estado
      |
      v
aplicar lançamento no domínio
      |
      v
calcular sequence_number + running_balance
      |
      v
INSERT
      |
      +--> sucesso -> commit
      |
      +--> conflito -> rollback -> retry
```

Não é necessário adquirir `FOR UPDATE` no fluxo atual apenas para impedir que duas requisições leiam o mesmo último estado. O sistema permite essa leitura concorrente e resolve a disputa no momento da confirmação.

A transação PostgreSQL usa isolamento `SERIALIZABLE`.

São considerados conflitos retryable no desenho atual:

- `40001` — serialization failure;
- `40P01` — deadlock detected;
- `23505` especificamente na constraint `unique_entry_sequence_number`.

Não transforme todo `23505` em retry. Uma violação de unicidade pode representar erro definitivo, inclusive conflito de idempotência.

## Exemplo: dois débitos concorrentes

Considere saldo inicial 100 e política `BALANCE_NON_NEGATIVE`. Duas requisições tentam debitar 100 simultaneamente.

Ambas podem inicialmente observar:

```text
sequence_number = N
running_balance = 100
```

Ambas podem calcular temporariamente:

```text
sequence_number = N + 1
running_balance = 0
```

Somente uma pode confirmar `(account_id, N+1)`. A outra operação deve sofrer conflito/abort e executar novamente a transação.

No retry, ela observa o estado confirmado:

```text
sequence_number = N + 1
running_balance = 0
```

Ao reaplicar o débito de 100, `ApplyEntry` calcula saldo negativo e `BALANCE_NON_NEGATIVE` rejeita a operação.

Esse comportamento é uma invariância central. Não mova a validação de saldo para fora da transação retryable.

## Retry

O retry deve repetir a **unidade transacional inteira**, e não somente o INSERT que falhou.

Isso é necessário porque um novo attempt precisa reler o estado confirmado e recalcular:

- `sequence_number`;
- `running_balance`;
- políticas dependentes do estado;
- qualquer decisão derivada da leitura anterior.

O aggregate/estado transitório do attempt anterior não deve ser reutilizado como se ainda fosse válido.

O retry atual é limitado, usa backoff exponencial com jitter e respeita cancelamento do contexto. Preserve essas propriedades.

Retry exhaustion deve ser observável. Não crie loop infinito para tentar esconder contenção.

## Ordem determinística

Quando uma operação toca mais de uma conta, processe as contas em ordem determinística.

O repository atual ordena entries por `AccountID` antes de calcular os estados. O command também evita ordem arbitrária ao carregar contas.

Essa disciplina reduz diferenças de ordem entre transações concorrentes e deve ser preservada, principalmente se no futuro forem introduzidos locks adicionais.

A ordem técnica não altera a semântica de débito/crédito da operação.

## Idempotência

`idempotency_key` identifica a requisição lógica. `request_fingerprint` identifica o conteúdo canônico associado à chave.

O comportamento esperado é:

```text
chave inexistente
    -> executar e persistir

mesma chave + mesmo fingerprint
    -> replay do resultado existente

mesma chave + fingerprint diferente
    -> conflito de idempotência
```

Nunca trate "mesma chave" isoladamente como autorização para retornar qualquer transação existente.

A unicidade de `idempotency_key` no PostgreSQL é a proteção final contra a corrida em que duas requisições passam simultaneamente pela leitura inicial.

Conflito nessa constraint não pertence ao mesmo retry de `sequence_number`. O fluxo deve recuperar a transação vencedora e comparar o fingerprint.

O fingerprint deve ser determinístico para a mesma requisição lógica e não depender de UUID, timestamp ou outro valor gerado durante um attempt.

## Atomicidade e Unit of Work

Uma transação financeira é confirmada como uma unidade.

Dentro do mesmo Unit of Work devem permanecer atomicamente consistentes os dados que fazem parte da confirmação da operação, incluindo transaction, entries e evento de outbox correspondente quando aplicável.

Repositories envolvidos no command devem resolver a transação PostgreSQL propagada pelo contexto. Não abra uma nova transação independente dentro de um repository participante.

Não publique evento de integração de forma não transacional antes do commit financeiro.

## Outbox

O outbox existe para acoplar atomicamente a decisão financeira à intenção de publicação.

A propriedade desejada é:

```text
COMMIT:
transaction + entries + outbox

ou

ROLLBACK:
nenhum deles
```

A entrega externa do evento pode ocorrer posteriormente e deve possuir sua própria estratégia de idempotência/retry.

Não confunda atomicidade de persistência do outbox com exactly-once de transporte.

## Transaction e entries

`transactions` representa o envelope da operação. `entries` representa os fatos contábeis por conta.

No modelo atual, `TRANSFER` exige exatamente dois lançamentos:

- uma entry debit;
- uma entry credit;
- contas distintas;
- mesmo amount.

O aggregate atualmente limita transações a no máximo duas entries. Trate isso como uma regra atual do modelo, não como uma propriedade universal de double-entry accounting.

Antes de implementar fee, tax, split, settlement ou outra operação multi-leg, revise explicitamente essa limitação em vez de contorná-la no repository.

## Dinheiro

Valores monetários persistidos usam representação inteira. Não introduza `float32` ou `float64` para amount ou balance.

A moeda faz parte do valor monetário e deve ser validada. Não realize operação entre moeda da entry e moeda da conta quando elas não forem compatíveis.

Mudanças de escala monetária, casas decimais ou suporte a novas moedas são mudanças de domínio e persistência, não simples formatação.

## Fronteiras arquiteturais

Preserve:

```text
transport -> application -> domain <- infrastructure
```

O domínio não deve depender de:

- pgx/PostgreSQL;
- nome de constraint;
- SQL;
- HTTP/gRPC;
- DTO de transporte;
- detalhes de outbox/broker;
- criteria definido dentro de infraestrutura.

A infraestrutura pode conhecer o domínio para implementar ports, mas o domínio não deve importar infraestrutura.

## Repository

O repository persiste e reconstrói estado; ele não define a semântica contábil.

É aceitável que o adapter PostgreSQL coordene leituras necessárias para persistência, como carregar o último `LedgerState`. O cálculo financeiro deve continuar delegado ao domínio.

Se uma consulta existe somente para idempotência, projeção ou leitura operacional e não exige reconstruir o aggregate inteiro, considere uma porta/query específica. Não force o aggregate a carregar dados desnecessários apenas para satisfazer uma consulta técnica.

## Constraints no PostgreSQL

Constraints são uma segunda linha de defesa das invariantes que o banco consegue expressar localmente.

O schema atual protege, entre outras propriedades:

```text
entries.amount > 0
entries.sequence_number > 0
entries.direction IN ('DEBIT', 'CREDIT')
char_length(entries.currency) = 3
UNIQUE(entries.account_id, entries.sequence_number)
UNIQUE(transactions.idempotency_key)
transactions.amount > 0
```

Não dependa exclusivamente das constraints para regra de negócio. O domínio deve rejeitar estados inválidos antes da persistência sempre que a regra pertence ao modelo.

Ao adicionar uma invariância que também pode ser protegida de forma simples pelo banco, avalie defesa em profundidade.

## Reconciliação

Reconciliação deve conseguir reconstruir o estado de uma conta aplicando entries na ordem de `sequence_number`.

Uma divergência entre o `running_balance` persistido e o valor recalculado deve ser tratada como sinal de inconsistência e observada/investigada.

Ferramentas de reconciliação não devem alterar silenciosamente o histórico para fazer os números "baterem".

## Observabilidade específica do ledger

Além das métricas HTTP/genéricas, acompanhe sinais que ajudam a provar saúde das invariantes:

- quantidade de retries por concorrência;
- retry exhaustion;
- serialization failures;
- deadlocks;
- conflitos de `unique_entry_sequence_number`;
- idempotency replay;
- idempotency conflict;
- duração do command;
- duração da transação;
- falhas de outbox;
- divergências detectadas por reconciliação.

Nunca use `account_id`, `transaction_id`, `idempotency_key` ou outros identificadores de alta cardinalidade como label Prometheus.

IDs podem aparecer em logs/traces quando permitido pelas regras de segurança e privacidade do projeto.

## Performance

O caminho crítico de escrita do ledger prioriza correção.

Antes de alterar isolamento, remover validações, introduzir cache de saldo ou adicionar lock pessimista para ganhar performance:

1. meça o gargalo;
2. identifique se é CPU, banco, contenção ou I/O;
3. observe p95/p99;
4. observe retries/conflitos;
5. crie benchmark/load test reproduzível;
6. prove que a nova estratégia preserva as invariantes.

Cache não deve se tornar fonte autoritativa de saldo.

## Testes obrigatórios para mudanças no núcleo

Ao alterar sequenciamento, saldo, idempotência, UoW ou persistência de entries, cubra pelo menos os cenários pertinentes:

- primeira entry de uma conta recebe sequência 1;
- entries subsequentes mantêm sequência contínua;
- duplicate `(account_id, sequence_number)` é rejeitado;
- débito/crédito respeita o tipo contábil;
- política non-negative rejeita saldo negativo;
- unrestricted permite saldo negativo quando a semântica exigir;
- overflow é rejeitado;
- moedas incompatíveis são rejeitadas;
- duas operações concorrentes sobre a mesma conta não confirmam a mesma sequência;
- retry relê estado e recalcula saldo;
- retry exhaustion retorna erro controlado;
- mesma idempotency key + mesmo fingerprint faz replay;
- mesma key + fingerprint diferente gera conflito;
- corrida de idempotência produz uma única transação;
- rollback não deixa entries parciais;
- outbox e ledger possuem atomicidade quando participam da mesma operação;
- reconciliação reproduz o `running_balance` esperado.

Quando houver concorrência em memória/goroutines, execute também:

```bash
go test -race ./...
```

## Alterações que exigem revisão arquitetural explícita

Não faça como refactor local uma mudança que introduza qualquer um destes comportamentos:

- saldo mutável por `UPDATE`;
- remoção do append-only;
- sequência global substituindo sequência por conta;
- remoção de `SERIALIZABLE`;
- remoção da unique de sequência;
- `FOR UPDATE` ou advisory locks como nova estratégia central;
- Redis/cache como fonte de saldo;
- retry ilimitado;
- multi-leg transaction além do modelo atual;
- moedas com escalas diferentes;
- sharding/partitioning de entries;
- publicação sem outbox;
- mudança da fronteira de consistência;
- event sourcing completo como substituição do modelo atual.

Essas opções não são proibidas. Elas alteram premissas centrais e precisam de trade-off analysis, testes de concorrência e, preferencialmente, ADR.

## Checklist de review

Antes de aprovar uma alteração no ledger, responda:

- a mudança preserva append-only?
- a fonte do saldo continua derivável das entries?
- `sequence_number` continua linear por conta?
- a concorrência possui um vencedor determinístico no banco?
- o perdedor relê o estado antes de tentar novamente?
- saldo non-negative é validado dentro da unidade retryable?
- idempotência distingue replay de payload conflitante?
- não existe commit parcial entre transaction/entries/outbox?
- regra financeira permaneceu no domínio?
- infraestrutura não vazou para o domínio?
- erro transitório está separado de erro definitivo?
- métricas permitem observar contenção e falhas?
- testes exercitam a corrida relevante?
- qualquer ganho de performance foi medido?

Se alguma resposta for "não", trate a mudança como alteração de arquitetura/invariante, e não como refactor comum.

## Relação com os demais guias

Use este arquivo como referência principal quando a tarefa tocar o núcleo financeiro do Isura Ledger.

Consulte também:

- `ddd-architecture-guide.md` para boundaries, repositories e trade-offs;
- `go-concurrency-guide.md` para goroutines, channels e sincronização em memória;
- `go-performance-guide.md` para benchmark, profiling e otimização orientada por dados;
- `go-types-generics-guide.md` para abstrações de tipos;
- `service-resilience-guide.md` para chamadas externas, retry e operação;
- `postgres.md` para convenções específicas de persistência deste repositório.

Este guia descreve o desenho vigente. Se uma decisão arquitetural mudar por ADR, atualize este documento junto com a implementação para evitar que o agente preserve uma premissa que deixou de ser válida.
