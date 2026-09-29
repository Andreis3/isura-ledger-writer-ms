# IDEA — Ajustes arquiteturais e evolução futura do Isura Ledger Writer

## Contexto

O `isura-ledger-writer-ms` implementa o núcleo de escrita do ledger do Isura Bank.

O fluxo atual de criação de uma transação é, em linhas gerais:

```text
CreateTransaction
    -> UnitOfWork / PostgreSQL SERIALIZABLE
    -> validação de idempotência
    -> carregamento das contas
    -> construção da Transaction e Entries
    -> Transaction.Complete()
    -> TransactionRepository.Save()
        -> leitura do último LedgerState de cada conta
        -> Account.ApplyEntry()
        -> atribuição de sequence_number
        -> atribuição de running_balance
        -> INSERT transaction
        -> INSERT entries
    -> INSERT outbox
    -> COMMIT
```

O mecanismo atual de concorrência deve ser preservado:

```text
PostgreSQL SERIALIZABLE
+ UNIQUE(account_id, sequence_number)
+ retry otimista com backoff/jitter
+ reconstrução da transação a cada tentativa
```

O objetivo desta ideia **não é redesenhar o ledger**, mas corrigir problemas de fronteira arquitetural e pequenas inconsistências, além de registrar decisões que podem ser avaliadas futuramente sem misturá-las com o trabalho necessário agora.

---

# Parte 1 — Ajustes necessários

## 1. Remover a dependência do domínio em infraestrutura PostgreSQL

### Problema

Atualmente a interface de repositório do domínio de transação depende de:

```go
internal/infra/postgres/repository/criteria
```

Isso cria uma dependência na direção:

```text
domain
   |
   v
infra/postgres
```

A infraestrutura deve implementar portas definidas em camadas internas, e não fornecer tipos necessários para que o domínio compile.

### Objetivo

Eliminar qualquer dependência de `internal/domain` em `internal/infra`.

### Implementação esperada

Criar um tipo de consulta independente de PostgreSQL próximo à abstração que o utiliza, por exemplo:

```go
package transaction

type FindCriteria struct {
    ID             *string
    IdempotencyKey *string
    WithEntries    bool
}
```

A interface passaria a usar o tipo interno:

```go
type Repository interface {
    Save(ctx context.Context, transaction *Transaction) error
    Find(ctx context.Context, criteria FindCriteria) (*Transaction, error)
    ExistsByIdempotencyKey(ctx context.Context, idempotencyKey string) (bool, error)
}
```

O adapter PostgreSQL deve ser responsável por traduzir esse critério para SQL.

Se `internal/infra/postgres/repository/criteria` tiver funções úteis para montar SQL, elas podem continuar existindo, mas devem receber o critério da camada interna ou um tipo convertido pelo adapter.

### Restrições

- Não mover código SQL para o domínio.
- Não introduzir `pgx`, `pgconn`, tipos PostgreSQL ou tipos de infraestrutura no domínio.
- Não alterar o comportamento das consultas.

### Critérios de aceite

- `internal/domain/...` não importa pacotes de `internal/infra/...`.
- A interface `transaction.Repository` continua independente da tecnologia de persistência.
- O adapter PostgreSQL continua suportando as consultas existentes.
- Testes existentes continuam passando.
- Nenhum comportamento da API é alterado.

---

## 2. Remover o método legado `AddTransnactionID`

### Problema

`Entry` possui o método correto:

```go
func (e *Entry) AddTransactionID(transactionID string)
```

e também um alias com erro de digitação:

```go
func (e *Entry) AddTransnactionID(transactionID string) {
    e.AddTransactionID(transactionID)
}
```

### Implementação esperada

Substituir todas as chamadas de `AddTransnactionID` por `AddTransactionID` e remover o método incorreto.

### Critérios de aceite

- Não existe mais `AddTransnactionID` no projeto.
- Todo código utiliza `AddTransactionID`.
- Testes continuam passando.
- Nenhum comportamento é alterado.

---

## 3. Tornar explícito o motivo da reconstrução do aggregate durante retry

### Contexto

`CreateTransaction.Execute()` inicialmente cria/valida a transação para obter o fingerprint e, dentro de `WithRetryableTransaction`, cria novamente a entidade.

Isso é intencional. Durante uma tentativa:

```text
Transaction.Complete()
TransactionRepository.Save()
    -> Entry.SequenceNumber é atribuído
    -> Entry.RunningBalance é atribuído
```

Reutilizar o mesmo aggregate em um retry poderia carregar estado mutado da tentativa anterior.

### Implementação esperada

Adicionar comentário curto próximo da reconstrução, por exemplo:

```go
// Rebuild the aggregate on every retry because a previous attempt may have
// mutated transaction status and assigned entry sequence/balance values.
entityTransaction, err := input.CreateTransactionFacade()
```

### Importante

**Não mover a construção de `entityTransaction` para fora de `WithRetryableTransaction`.**

Cada tentativa deve trabalhar com um estado novo e reler o estado atual do ledger dentro da nova transação PostgreSQL.

### Critérios de aceite

- O aggregate continua sendo reconstruído a cada retry.
- Existe documentação no código explicando a decisão.
- Nenhuma alteração funcional é introduzida.

---

## 4. Documentar formalmente a responsabilidade das tabelas do ledger

### Modelo atual

```text
accounts
    configuração/identidade da conta

transactions
    envelope da operação e idempotência

entries
    ledger contábil append-only e source of truth do estado financeiro

entries.running_balance
    saldo derivado após a aplicação daquela entry

outbox_events
    publicação transacional de eventos de integração
```

O saldo atual não deve ser tratado como estado mutável de `accounts`.

```text
current balance(account) =
    running_balance da entry com maior sequence_number da conta
```

### Objetivo

Evitar que futuras implementações introduzam `UPDATE accounts SET balance = ...` ou outra tabela mutável como fonte primária do saldo sem decisão arquitetural explícita.

### Restrições

- Não alterar schema apenas para realizar esta tarefa.
- Não criar tabela `balances`.
- Não adicionar coluna `balance` em `accounts`.
- Não criar UPDATE de `entries`.

### Critérios de aceite

- A documentação deixa explícita a responsabilidade de cada estrutura.
- `entries` permanece append-only.
- O saldo continua derivado do ledger.
- Nenhuma alteração funcional é necessária.

---

## 5. Preservar explicitamente o modelo atual de concorrência

O modelo atual utiliza concorrência otimista. Duas transações concorrentes podem inicialmente observar:

```text
sequence_number = N
running_balance = X
```

e calcular `sequence_number = N + 1`.

A constraint:

```sql
UNIQUE(account_id, sequence_number)
```

impede que dois fatos ocupem a mesma posição no stream da conta.

O UoW utiliza `SERIALIZABLE` e trata como retryable:

```text
40001  serialization_failure
40P01  deadlock_detected
23505  unique_entry_sequence_number
```

Depois do rollback, a operação é reconstruída e executada novamente contra o novo estado.

### Exemplo crítico

Conta com saldo 100 e dois withdrawals simultâneos de 100. Ambas as operações podem inicialmente observar saldo 100, mas apenas uma deve confirmar o próximo estado. A outra deve sofrer conflito, realizar retry, reler saldo 0 e ser rejeitada por `BalanceNonNegative`.

### Não fazer

- Não substituir o modelo atual por `SELECT ... FOR UPDATE` apenas para simplificar.
- Não remover `UNIQUE(account_id, sequence_number)`.
- Não calcular `sequence_number` fora da transação.
- Não calcular saldo a partir de estado carregado fora do retry.
- Não fazer retry reutilizando `Transaction`/`Entry` mutadas.
- Não remover `SERIALIZABLE` sem benchmark, testes de concorrência e decisão arquitetural específica.

### Critérios de aceite

Os testes devem continuar cobrindo:

- `sequence_number` monotônico por conta;
- unicidade de `(account_id, sequence_number)`;
- concorrência sobre a mesma conta;
- retry após serialization failure;
- retry após conflito de `sequence_number`;
- insufficient balance após retry.

---

## 6. Avaliar nomenclatura dos métodos que completam referências da Entry

### Contexto

A `Entry` nasce com informações de negócio e posteriormente recebe identificadores internos:

```go
entry.AddAccountID(...)
entry.AddTransactionID(...)
```

`AddAccountID` não representa uma coleção; ele atribui um único identificador.

### Ajuste sugerido

Avaliar renomear `AddAccountID` para `AssignAccountID` ou outro nome que expresse atribuição única. Para consistência, avaliar também se `AddTransactionID` deveria seguir a mesma convenção.

### Restrição

- Esta alteração é apenas de nomenclatura.
- Não redesenhar `EntryBuilder` nesta tarefa.
- Não transformar a entidade inteira em immutable value object.
- Não introduzir mudança funcional.

Se a mudança gerar churn sem ganho suficiente, manter os nomes atuais e registrar a decisão.

---

# Parte 2 — Decisões futuras

Os itens desta seção **não devem ser implementados como parte dos ajustes necessários**. Devem ser registrados como decisões arquiteturais futuras, backlog técnico ou ADRs a avaliar.

## 1. Suporte a transações com mais de duas entries

O aggregate atualmente limita a transação a no máximo duas entries e `TRANSFER` exige exatamente um debit e um credit de mesmo valor em contas distintas.

Um ledger mais genérico pode futuramente precisar de N postings, por exemplo principal + tarifa + imposto + contrapartidas.

Antes de implementar, definir invariantes contábeis, balanceamento, moedas, impacto em idempotência, eventos, APIs e reconciliation. Não remover o limite atual apenas por generalização antecipada.

## 2. Constraint adicional por transaction/account/direction

Avaliar futuramente:

```sql
UNIQUE(transaction_id, account_id, direction)
```

Ela pode fortalecer o modelo atual de duas entries, mas pode ser restritiva se futuramente uma transação aceitar múltiplas entries da mesma direção para a mesma conta. Não adicionar automaticamente agora.

## 3. Semântica do erro após esgotar retries

Avaliar se o contrato deve distinguir:

```text
TIMEOUT
```

de:

```text
TRANSACTION_CONFLICT / CONCURRENCY_EXHAUSTED
```

Antes de alterar, avaliar contrato HTTP/gRPC, clientes, observabilidade, retry do caller, métricas e backward compatibility.

## 4. Definir formalmente o nível de Event Sourcing utilizado

O ledger possui características fortes de Event Sourcing: entries append-only, `sequence_number`, reconstrução do estado, `running_balance` derivado e ausência de UPDATE/DELETE para fatos confirmados.

Ao mesmo tempo, `transactions` funciona como envelope persistido da operação.

Definir futuramente se o projeto pretende ser:

```text
A) Event Sourcing puro para todo o domínio
```

ou:

```text
B) immutable accounting ledger + transaction envelope + CQRS/outbox
```

Não alterar código apenas para satisfazer o rótulo "Event Sourcing puro".

## 5. Imutabilidade mais forte de Entry e Transaction

Avaliar futuramente separar conceitos como:

```text
UncommittedEntry / CommittedEntry
```

ou:

```text
EntryDraft / LedgerEntry
```

onde somente a entry confirmada possui `sequence_number` e `running_balance`.

Isso pode representar estados inválidos pelo sistema de tipos, mas aumenta a quantidade de tipos e conversões. Não implementar sem benefício concreto.

## 6. Estratégia de concorrência em cenários de hot accounts

Se métricas reais demonstrarem contenção relevante, avaliar alternativas como:

```text
partitionamento lógico
single-writer por account_id
fila/stream particionado por account_id
sequenciamento externo
modelo híbrido de locking
```

Não otimizar antecipadamente. Basear qualquer mudança em métricas de retry, exhaustion, p95/p99, conflitos por conta e throughput por hot account.

---

# Fora de escopo desta implementação

O agente **não deve**:

```text
- redesenhar o ledger;
- trocar concorrência otimista por pessimistic locking;
- adicionar SELECT FOR UPDATE;
- remover SERIALIZABLE;
- remover UNIQUE(account_id, sequence_number);
- criar tabela mutável de saldo;
- adicionar balance em accounts;
- fazer UPDATE/DELETE de entries;
- implementar N postings;
- alterar contratos públicos sem necessidade;
- alterar a semântica contábil de Account.ApplyEntry;
- alterar regras de BalancePolicy;
- substituir o Outbox Pattern;
- fazer refatorações amplas não relacionadas.
```

---

# Ordem sugerida de implementação

```text
1. Corrigir a direção da dependência transaction.Repository / criteria.
2. Remover AddTransnactionID.
3. Documentar a reconstrução do aggregate durante retry.
4. Documentar formalmente transactions / entries / running_balance / outbox.
5. Revisar e preservar testes de concorrência e invariantes.
6. Avaliar a pequena melhoria de nomenclatura AddAccountID/AddTransactionID.
```

Após cada alteração:

```bash
go test ./...
```

Executar também os testes de integração PostgreSQL conforme a configuração existente no projeto e, quando aplicável:

```bash
go test -race ./...
```

Não considerar a tarefa concluída apenas porque o código compila.

---

# Definition of Done

A implementação estará concluída quando:

- o domínio não depender de `internal/infra`;
- o typo `AddTransnactionID` tiver sido removido;
- a reconstrução do aggregate por tentativa estiver documentada e preservada;
- a responsabilidade de `accounts`, `transactions`, `entries`, `running_balance` e `outbox_events` estiver documentada;
- o mecanismo de concorrência atual permanecer intacto;
- os invariantes contábeis existentes permanecerem intactos;
- os testes unitários passarem;
- os testes de integração passarem;
- os testes de concorrência continuarem demonstrando sequenciamento monotônico e ausência de double spending;
- não houver mudança desnecessária de API ou schema;
- as decisões futuras permanecerem apenas documentadas, sem implementação oportunista.

---

# Resultado esperado

```text
Transport
    |
    v
Application
    |
    v
Domain / Ports
    ^
    |
Infrastructure Adapters
```

O fluxo crítico deve continuar:

```text
request
   |
   v
CreateTransaction
   |
   v
SERIALIZABLE transaction
   |
   +--> idempotency check
   +--> load account configuration
   +--> rebuild aggregate
   +--> read latest LedgerState
   +--> Account.ApplyEntry()
   +--> assign sequence_number
   +--> assign running_balance
   +--> INSERT transaction
   +--> INSERT immutable entries
   +--> INSERT outbox event
   |
   v
COMMIT
   |
   +-- conflict --> rollback --> jitter/backoff --> rebuild + retry
   +-- success --> return
```

O princípio central deve continuar sendo:

> O saldo não é um campo mutável. O saldo é consequência ordenada dos fatos contábeis persistidos no ledger.
