# IDEAS — próximos ajustes do Isura Ledger Writer

Este arquivo reúne ideias, correções arquiteturais e próximos passos identificados a partir do estado atual do `isura-ledger-writer-ms`.

Ele não substitui o `TODO.md` nem as tasks formais. A ideia é registrar decisões e pontos de evolução antes de transformá-los em PRD, TechSpec e tarefas executáveis.

---

## 1. Regra de saldo insuficiente por natureza contábil

A validação de saldo insuficiente não deve ser uma regra genérica do tipo `running_balance - amount >= 0`.

| Tipo da conta | Débito | Crédito | Saldo normal |
|---|---:|---:|---|
| ASSET | aumenta | diminui | devedor |
| EXPENSE | aumenta | diminui | devedor |
| LIABILITY | diminui | aumenta | credor |
| REVENUE | diminui | aumenta | credor |
| EQUITY | diminui | aumenta | credor |

O cálculo do saldo deve considerar o `AccountingType` da conta. A regra atual não deve assumir universalmente `DEBIT = -` e `CREDIT = +`.

### Separar natureza contábil de política financeira

O tipo contábil define como o lançamento afeta o saldo. A política financeira define se aquele saldo pode ou não ultrapassar determinado limite.

Possível modelo:

- `BalanceNonNegative`: saldo normal não pode ser ultrapassado.
- `BalanceUnrestricted`: conta sem restrição de saldo.

Não usar `AccountingType` sozinho para decidir se saldo negativo é permitido. Contas internas podem ter políticas diferentes de contas de cliente.

---

## 2. Mover cálculo de saldo para o domínio

Hoje parte importante da regra financeira está no repository PostgreSQL. O repository deveria cuidar principalmente de carregar estado e persistir dados.

Fluxo desejado:

`Repository -> carrega último LedgerState -> Domain.ApplyEntry -> Repository.Save`

Possível abstração:

```go
type LedgerState struct {
    AccountID      string
    SequenceNumber int64
    RunningBalance int64
}

func (a Account) ApplyEntry(state LedgerState, direction transaction.Direction, amount money.Money) (LedgerState, error)
```

Responsabilidades da operação:

- aplicar a natureza contábil da conta;
- validar `BalancePolicy`;
- retornar `ErrInsufficientBalance` quando aplicável;
- incrementar `sequence_number`;
- produzir o próximo `running_balance`.

---

## 3. Fluxo com concorrência otimista

Fluxo esperado:

`carregar conta -> carregar último LedgerState -> aplicar lançamento -> validar saldo -> sequence + 1 -> INSERT entry`

Em caso de colisão em `UNIQUE(account_id, sequence_number)`: rollback, retry da transação inteira, releitura do estado e nova validação.

Erros de concorrência devem causar retry. Erros de negócio não:

- `40001` serialization failure -> retry
- `40P01` deadlock -> retry
- `23505 unique_entry_sequence_number` -> retry
- insufficient balance -> fail
- currency mismatch -> fail
- inactive account -> fail

---

## 4. Teste crítico: dois débitos concorrentes

Cenário obrigatório de integração real com PostgreSQL:

- saldo inicial = 100;
- sequence = 10;
- TX A debita 100;
- TX B debita 100.

Resultado esperado:

- 1 transação com sucesso;
- 1 transação com `INSUFFICIENT_BALANCE`;
- saldo final = 0;
- sequence final = 11.

Nunca permitir saldo final -100.

---

## 5. Invariantes do ledger

Adicionar testes explícitos para:

- double-entry: `debit.amount == credit.amount`;
- uma entry de cada lado esperado;
- `sequence[n+1] = sequence[n] + 1` por conta;
- nenhuma duplicidade de `(account_id, sequence_number)`;
- saldo atual equivalente ao último `running_balance`;
- idempotency replay para mesma key + fingerprint;
- idempotency conflict para mesma key + fingerprint diferente.

---

## 6. Reconciliation / auditoria

Criar rotina capaz de reconstruir o saldo aplicando sequencialmente todas as entries e comparar o resultado com o último `running_balance`.

Usos: auditoria, diagnóstico de corrupção, pós-migração, reconstrução de projeções e testes operacionais.

---

## 7. Outbox Relay — revisar shutdown/drain

O `OutboxRelay.drain()` precisa ser revisado. Durante shutdown ele pode fazer claim de registros sem efetivamente publicá-los.

Decidir entre:

- remover `drain()` se `publishBatch()` já aguarda todos os workers;
- implementar drain real: claim -> publish -> wait workers -> shutdown.

---

## 8. Testes de falha do Outbox

Cobrir:

- crash depois de claim e antes de publish;
- publish com sucesso e crash antes de marcar SUCCESS;
- retry após `RetryAfter`;
- `MaxAttempts`;
- DLQ;
- múltiplas instâncias usando `SKIP LOCKED`;
- deduplicação via `Nats-Msg-Id`;
- shutdown durante batch ativo.

---

## 9. Reader / CQRS

Depois de fechar as invariantes financeiras do writer, iniciar o `isura-ledger-reader-ms`.

Fluxo:

`Ledger Writer -> PostgreSQL append-only -> Transactional Outbox -> NATS JetStream -> Ledger Reader`

Possíveis projeções:

- saldo atual;
- extrato;
- histórico de transações;
- saldo diário;
- movimentações por período;
- agregações para relatórios.

O reader não deve ser fonte de verdade para decisões financeiras críticas do writer.

---

## 10. Reconstrução de estado

Avaliar operação explícita de replay das entries ordenadas por `sequence_number` para reconstruir o estado atual da conta.

---

## 11. Observabilidade de concorrência

Adicionar métricas como:

- `ledger_concurrency_retry_total`;
- `ledger_sequence_conflict_total`;
- `ledger_serialization_failure_total`;
- `ledger_deadlock_total`;
- `ledger_insufficient_balance_total`;
- `ledger_idempotency_replay_total`;
- `ledger_idempotency_conflict_total`.

Também considerar histogramas de número de retries e tempo total até sucesso.

---

## 12. Observabilidade financeira

Adicionar métricas operacionais sem expor dados sensíveis:

- transações concluídas;
- rejeições por saldo insuficiente;
- transações por operação;
- backlog da outbox;
- idade do item pendente mais antigo;
- eventos enviados para DLQ.

---

## 13. Atualizar README

Revisar documentação que ainda não representa completamente a implementação atual:

- Kafka -> NATS JetStream, onde aplicável;
- remover referências a `UPDATE accounts SET balance` se a decisão final for saldo append-only;
- documentar `sequence_number` + `running_balance` como estado derivado do stream;
- atualizar diagrama do fluxo Transaction -> Entries -> Outbox -> JetStream.

---

## 14. Atualizar TODO.md

O `TODO.md` contém itens já implementados pelos PRs recentes. Remover ou marcar como concluídos itens de idempotência, gRPC, `SKIP LOCKED`, validação de moeda e relay já existentes.

`TODO.md` deve representar trabalho executável atual. `IDEAS.md` deve representar ideias, riscos e decisões ainda não formalizadas.

---

## 15. Revisar nomenclatura de EntryLedgerState

Avaliar nomes mais próximos do domínio:

- `AccountLedgerState`;
- `LedgerState`;
- `AccountStreamState`.

O estado representa o acumulado da conta no stream, não uma propriedade exclusiva da `Entry`.

---

## 16. Manter sequence_number

Manter `sequence_number` como nome preferencial.

A chave `(account_id, sequence_number)` representa a posição monotônica da entry no stream da conta e funciona como versão lógica desse stream.

---

## 17. Separar persistência de cálculo de estado

Refatorar gradualmente `TransactionRepository.assignSequences()`.

Hoje ele concentra leitura de estado, geração de sequência, cálculo de saldo e preparação da entry.

Objetivo:

`repository.readLedgerState() -> domain.ApplyEntry() -> repository.Save()`

---

## 18. Erros de domínio estáveis

Garantir códigos estáveis para pelo menos:

- `INSUFFICIENT_BALANCE`;
- `CURRENCY_MISMATCH`;
- `ACCOUNT_INACTIVE`;
- `ACCOUNT_NOT_FOUND`;
- `IDEMPOTENCY_CONFLICT`;
- `TRANSACTION_CONFLICT`;
- `INVALID_ACCOUNTING_OPERATION`.

REST e gRPC devem traduzir esses erros sem expor detalhes técnicos do PostgreSQL.

---

## 19. Tipos de operação e políticas específicas

Avaliar regras distintas para:

- `TRANSFER`;
- `DEPOSIT`;
- `WITHDRAW`;
- `HOLD`;
- `RELEASE`;
- `REVERSAL`;
- `ADJUSTMENT`;
- `FEE`;
- `SETTLEMENT`.

Nem toda operação necessariamente deve aplicar a mesma política de saldo.

---

## 20. Holds / saldo disponível

Quando Hold/Release entrar no ledger, separar:

- accounting balance;
- available balance;
- held amount.

Evitar representar reservas apenas alterando o mesmo `running_balance`.

Possível regra: `available_balance = ledger_balance - active_holds`.

---

## 21. Reversals

Nunca editar nem apagar uma entry financeira efetivada. Correções devem gerar uma nova transação compensatória com referência à transação original.

---

## 22. Imutabilidade do ledger

Reforçar a decisão: financial entries são INSERT-only.

Evitar `UPDATE entries SET amount = ...` e `DELETE FROM entries ...`.

---

## 23. Fitness tests arquiteturais

Adicionar testes automatizados para proteger decisões importantes:

- domain não importar infra;
- transaction/entry não depender de PostgreSQL;
- transport não acessar repository diretamente;
- nenhuma atualização destrutiva em entries;
- schema preservar unique `(account_id, sequence_number)`;
- writer não depender do reader para validar saldo.

---

## 24. Ordem sugerida

1. modelar natureza contábil + BalancePolicy;
2. mover cálculo de `running_balance` para o domínio;
3. implementar saldo insuficiente;
4. criar teste concorrente real no PostgreSQL;
5. revisar invariantes;
6. corrigir Outbox drain;
7. atualizar README e TODO;
8. adicionar reconciliation;
9. fortalecer observabilidade;
10. iniciar `isura-ledger-reader-ms`.

O próximo PR deveria se concentrar principalmente nos itens 1–4.