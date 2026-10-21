# Fase 2 — FundsHold: persistência, UoW e saldo disponível

> **Status:** proposta de implementação para Codex. **Este documento não implementa código.**
>
> **Branch:** `feat/010-funds-hold-persistence` (criada a partir da `main`, após o merge da Fase 1 / PR #60).
>
> **Escopo:** persistir alocações de bloqueios, executar comandos idempotentes e integrar a indisponibilidade de recursos ao fluxo de lançamentos do Writer, com garantias de concorrência. **Não implementar consumidores Kafka, Reader Java, contratos finais de NATS ou execução judicial nessa fase.**

## 1. Contexto confirmado no repositório

- `internal/domain/hold` já contém `HoldType`, `HoldRequest`, `ActiveAllocation`, `AllocationPolicy`, `HoldEvaluationInput`, `HoldDecision` e `Evaluate` (Fase 1).
- `internal/application/command/create_transaction.go` coordena a criação transacional, idempotência e Outbox; `internal/application/service/ledger_entry_assigner.go` atribui `sequence_number` e `running_balance` para cada `Entry`.
- A verdade contábil do saldo de uma conta é o último `running_balance` em `entries`, consultado via `FindLatestLedgerState`; não assumir que a tabela `balances` é a fonte oficial.
- `account.Account.ApplyEntry` aplica a natureza contábil de débito/crédito de acordo com `AccountType` e `BalancePolicy` (`BALANCE_NON_NEGATIVE` ou `BALANCE_UNRESTRICTED`). **Não presumir que `DEBIT` reduz o saldo em todo tipo de conta.**
- A UoW já utiliza transações PostgreSQL e retry. A Fase 2 deve aderir ao padrão existente, sem criar um segundo mecanismo de transação independente.

## 2. Objetivo funcional

Uma conta pode ter saldo contábil e uma parte de seus fundos estar indisponível. Exemplo: saldo contábil de R$ 10.000; hold judicial efetivo de R$ 3.000; saldo disponível para saídas autorizadas de R$ 7.000. Criar/alterar/liberar hold **não insere uma Entry financeira**; somente uma execução que transfira recursos deve registrar `Transaction/Movements/Entries` balanceadas, em fase posterior.

A conta pode ter vários holds em paralelo, inclusive de tipos distintos, mas as alocações efetivas não podem comprometer duas vezes os mesmos fundos. A hierarquia da Fase 1 é **operacional e configurável**, nunca autoridade jurídica universal.

## 3. Entregáveis mínimos

### 3.1. Modelo persistente e migrations (Atlas / PostgreSQL)

Propor migração compatível com as convenções `db/*.pg.hcl` e mecanismo de aplicação já utilizado. Avaliar duas estruturas, justificando a escolha:

1. `funds_holds`: estado corrente, identidade, `account_id`, tipo, moeda, referência externa/origem, chave idempotente, valor solicitado, valor efetivamente alocado, valor pendente, valor liberado/executado, status, versão da política, timestamps e metadados mínimos;
2. `funds_hold_history` (ou eventos/audit trail imutáveis): registrar instruções, mudanças de alocação, liberações, decisões rejeitadas/conflitos e autoria/causa com referências idempotentes.

Não colocar `blocked_amount` como campo mutável sem trilha de origem em `accounts`. Se optar por projeção/cache do total bloqueado por conta, definir reconciliação e garantir atualização atômica.

Constraints mínimas a especificar: FK para a conta, quantias não negativas, `allocated <= requested` (ou regra equivalente formalizada para alterações da instrução), saldos internos coerentes, unicidade da instrução por emissor + referência/chave idempotente, índices por `account_id/status`. Dinheiro em unidades menores da moeda (`BIGINT`) e sem `float`. Identificadores e nomenclatura devem seguir o repositório; não inventar restrições que impeçam múltiplas instruções válidas sobre uma conta.

### 3.2. Repositório de domínio e adaptador PostgreSQL

Definir interface mínima próxima do domínio `hold` e implementação em `internal/infra/postgres/repository`, preferencialmente no mesmo contexto de transação do Writer.

Operações candidatas: consultar holds ativos por conta, consultar por referência idempotente, inserir novo hold e histórico, atualizar estado/versionamento com proteção contra atualização perdida, consultar soma alocada por conta. Proibir mutação silenciosa de registros históricos. Definir como bloquear/serializar escritores concorrentes e documentar SQL/índices após implementação.

### 3.3. Use cases na Application

Implementar comandos estritamente necessários ao MVP de bloqueios:

- `CreateFundsHold`: validar conta/moeda/origem, deduplicar instrução, obter snapshot consistente, aplicar `hold.Evaluate`, persistir resultado e histórico na mesma UoW; persistir decisão `PENDING_REVIEW` quando aplicável, sem reservar fundos indevidamente.
- `ReleaseFundsHold`: liberar apenas valor efetivamente retido, de acordo com instrução autorizada; suportar liberação parcial, se negócio aprovar; idempotência e histórico.
- **Não implementar `ExecuteFundsHold` nesta fase**: execução financeira requer contrato de transferência, política jurídica e partidas dobradas. Também deixar alteração de valor e substituição/realocação explícita para decisão posterior.

Não usar `HoldDecisionStatus` (`ALLOCATED`, `PARTIALLY_ALLOCATED`, `PENDING_REVIEW`) como se fosse automaticamente a máquina de estados definitiva do agregado persistido: definir separadamente o status da instrução e a decisão pontual de avaliação.

### 3.4. Integração com `CreateTransaction`

Criar validação **dentro da transação PostgreSQL** antes da persistência de lançamentos, compartilhando o protocolo de concorrência com criação/liberação de holds:

- Para cada conta relevante, calcular o **efeito líquido** dos lançamentos na mesma conta (seguindo `AccountType` e `BalancePolicy`) e/ou validar a **evolução sequencial** quando a regra de negócio exigir não ultrapassar saldo disponível em nenhum ponto intermediário;
- Para contas sujeitas à restrição de disponibilidade, assegurar que o saldo após cada saída financeira permitida não viole a parte efetivamente retida;
- Preservar regras existentes para contas contábeis com saldo irrestrito e contas onde a natureza contábil difere; **não aplicar indiscriminadamente `balance - holds` a todas as contas**;
- Manter o fluxo de `AssignLedgerEntries`, idempotência, retry e Outbox, sem alterar o formato dos eventos existentes sem versionamento explícito;
- Não deslocar holds já alocados por simples prioridade numérica — conflitos jurídicos e garantias exigem instrução/decisão autorizada.

**Regra essencial:** se um hold e uma saída financeira concorrerem sobre os mesmos recursos, não podem ambos confirmar baseados no mesmo saldo livre inicial. Fazer criação/liberação de holds e lançamentos financeiros participarem do **mesmo protocolo de serialização por conta**. Avaliar bloqueio determinístico de linhas de conta (`SELECT ... FOR UPDATE`, em ordem estável para múltiplas contas) dentro de UoW, ou outra estratégia equivalente explicitamente comprovada. `SERIALIZABLE` e retry continuam relevantes, mas **não substituem** o alinhamento dos fatos lidos/escritos. Verificar efeitos sobre throughput de contas quentes e ausência de deadlocks.

### 3.5. Outbox e observabilidade

Persistir eventos de mudanças efetivas de hold pelo padrão Transactional Outbox existente. Definir somente payloads internos provisórios compatíveis com futura RFC; não presumir integração com Reader Java já existente. Idempotência para replay do comando, duplicações e commit incerto (mesma semântica cuidadosa do Writer). Métricas de comandos, conflitos, retries e idade dos holds pendentes de revisão sem cardinalidade alta em labels. Incluir logs com ID de operação/hold, sem expor conteúdo de ordens judiciais ou PII.

## 4. Invariantes obrigatórias

1. `held_amount >= 0`, `pending_amount >= 0`, `released_amount >= 0`; soma/limites conservados por uma máquina de estados clara.
2. Uma instrução idempotente repetida não aumenta o valor bloqueado; mesma chave com conteúdo incompatível resulta em conflito explícito.
3. `sum(active allocations) <= eligible funds` para o escopo de recursos cobertos pela política, salvo política expressa e aprovada para retenção de créditos futuros (fora do MVP).
4. Liberações jamais excedem o montante efetivamente retido remanescente.
5. Conta/hold/moeda devem ser consistentes, inclusive em concorrência.
6. Criação de hold e transação financeira não confirmam simultaneamente consumo do mesmo saldo livre.
7. Nenhum `UPDATE` altera ou apaga `entries` históricas; holds não criam postings contábeis fictícios.
8. Regras de `Account.BalancePolicy` continuam válidas; o saldo oficial continua sendo derivado de `entries`.
9. Mudanças de hold deixam trilha imutável e Outbox transacional.
10. Instruções sem política configurada ou conflitos indecidíveis não reservam automaticamente fundos: retornar/persistir situação de revisão.

## 5. Testes exigidos

**Domínio:** regras de status, liberação parcial e total, validação de valores/monedas, conversão entre decisão de alocação e estado persistente, política não configurada.

**Integração PostgreSQL:** migration up/down se suportado; FK, unicidade e checks; rollback entre hold e Outbox; recuperação de idempotência após commit incerto; dois holds disputando o mesmo saldo; débito × hold simultâneos; liberação × débito; múltiplas contas acessadas em ordens diferentes; snapshots obsoletos; validação com `-race` nos testes aplicáveis. Preferir testes reais de PostgreSQL para garantias de isolamento (mocks não comprovam serialização).

**Regressão:** `CreateTransaction` sem holds continua funcionando; idempotência e retries originais; transações de débito/crédito em contas dos diferentes tipos contábeis; nenhuma regressão de balanceamento de movimentos ou `sequence_number`.

Documentar comandos reais usados para executar os testes e resultados obtidos; não afirmar CI verde sem evidência.

## 6. Fora do escopo desta fase

- Receber e validar ordens reais de SISBAJUD, regras de impenhorabilidade, autorização jurídica ou precedência entre credores;
- Consumo Kafka de outros Bounded Contexts;
- Execução/transferência de valores bloqueados, estorno judicial e efeitos contábeis de cumprimento;
- Garantia legal de preempção entre holds e realocação automática de fundos já comprometidos;
- Leitor Java, projeções NATS finais, API pública de extrato/saldo;
- Bloqueio de créditos futuros, expiração automática e variações de moeda sem especificação;
- Criação de benchmark ou SLO sem medição.

## 7. Critérios de aceite

- Migrations e repositório funcionam em PostgreSQL com testes de constraints e concorrência.
- Criar/repetir/liberar hold funciona de maneira idempotente, auditável e atômica com Outbox.
- Débitos/saídas sujeitos à regra de disponibilidade respeitam holds efetivos, inclusive sob execução concorrente.
- Não há dupla reserva de recursos nem atualização de `entries` históricas.
- O protocolo de concorrência por conta e as limitações das políticas estão documentados.
- A suíte existente continua passando; novos testes de integração demonstram as invariantes.
- Alterações seguem DDD/Clean já existentes e não introduzem Kafka/Reader fora do escopo.

## 8. Pontos a decidir antes da codificação

- Quais contas e saldos são efetivamente elegíveis a holds (não necessariamente toda conta contábil)?
- Permitir hold parcial de acordo com tipo e instrução, ou somente alocação integral?
- Quando `PENDING_REVIEW` deve bloquear outros pedidos: sempre, ou apenas se comprometer recursos?
- Quem autoriza `ReleaseFundsHold`, e como representar a instrução sem atribuir ao Ledger a decisão jurídica?
- Como tratar redução de saldo contábil por processos extraordinários/reversões quando já há hold efetivo?
- Modelo de eventos internos e de audit trail para permitir replay de estado dos holds.

**Codex deve registrar explicitamente as decisões assumidas e não preencher lacunas legais com prioridade hardcoded.**

## 9. Prompt para Codex

> Trabalhe na branch `feat/010-funds-hold-persistence`. Leia `docs/idea-funds-hold-phase-2-persistence.md` e os arquivos atuais de `hold`, `account`, `transaction`, `balance`, `CreateTransaction`, UoW, PostgreSQL repository, migrations Atlas, Outbox e testes. Implemente **somente a Fase 2** com migração, repositório, comandos de criação/liberação e validação do saldo disponível integrada a `CreateTransaction`, demonstrando a serialização correta de operações concorrentes sobre a mesma conta. Preserve o histórico de `entries`, a lógica de `running_balance`, idempotência e a separação entre prioridade operacional e jurídica. Não implemente Kafka nem Reader Java. Execute testes reais de PostgreSQL e reporte evidências e decisões pendentes antes de abrir PR.
