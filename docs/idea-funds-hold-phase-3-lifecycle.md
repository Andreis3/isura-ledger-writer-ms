# Fase 3 — Fechamento funcional de FundsHold no Ledger Writer

> **Status:** proposta para execução pelo Codex. Este arquivo é uma especificação, não prova de implementação.
>
> **Branch:** `feat/011-funds-hold-lifecycle` (baseada em `main` após o merge do PR #61).
>
> **Objetivo:** fechar os fluxos internos do Writer para revisão de bloqueios pendentes, governança de elegibilidade e execução contábil autorizada de valores retidos. **Sem Kafka externo nem Reader Java nesta fase.**

## 1. Contexto verificado

- O Bounded Context é **Ledger**, com Writer em Go e Reader Java ainda planejado. Comunicação interna projetada por NATS JetStream com Transactional Outbox; Kafka é destinado a eventos entre contextos/plataformas.
- O domínio `internal/domain/hold` já define `Instruction`, `HoldStatus` (`ALLOCATED`, `PARTIALLY_ALLOCATED`, `PENDING_REVIEW`, `RELEASED`), política de alocação, validação, `ApplyDecision` e `Release`.
- `CreateHold` registra instruções e decisões, inclusive `PENDING_REVIEW`; `ReleaseHold` suporta liberação autorizada e idempotente.
- A Fase 2 adicionou `holds`, `hold_history` e `hold_releases`, `holds_eligible` nas contas e a validação de saldo disponível em `CreateTransaction`.
- O estado contábil oficial é calculado pelas `entries` append-only e `running_balance`; `AssignLedgerEntries` atribui sequências e saldos. `Account.ApplyEntry` respeita a natureza contábil, não se deve presumir que todo débito diminui o saldo.
- A UoW PostgreSQL é `SERIALIZABLE` com retentativas para conflitos. Todos os novos comandos devem compartilhar a mesma transação de domínio, histórico e Outbox.

## 2. Decisões fundamentais

**Bloqueio financeiro não é bloqueio operacional da conta.** `Account.Status=BLOCKED` e `FundsHold` são mecanismos diferentes.

**Bloqueio não é lançamento financeiro.** A retenção e liberação alteram disponibilidade, não `entries`. **Execução** que movimenta dinheiro requer `Transaction` com movimentos e lançamentos de partidas dobradas.

**O Ledger não escolhe precedência judicial nem verifica sozinho o fundamento jurídico.** Cada instrução recebe referência, origem, autorização e política previamente validadas por um sistema responsável. O Writer verifica consistência financeira, permissão técnica, idempotência, sequenciamento e auditoria. Prioridade operacional entre tipos não autoriza preempção automática de bloqueios existentes.

**Não liberar recursos por falha de configuração.** Se há hold efetivo, desabilitar `holds_eligible` não pode fazer `CreateTransaction` ignorar a retenção. Esse é um requisito de segurança crítico.

## 3. Entregas por subfase

### 3A — Resolver `PENDING_REVIEW` (primeira entrega, recomendada como PR independente)

Novo comando de aplicação proposto: `ResolvePendingHold`.

**Entrada mínima:** `hold_id`, `decision_id`/chave idempotente, referência da autorização, política + versão efetivamente aprovada, ação (`ALLOCATE`, `PARTIALLY_ALLOCATE`, `REJECT` ou `KEEP_PENDING`) e motivação rastreável. A ação final não deve depender apenas do texto de um status vindo do cliente.

**Fluxo:**

1. Validar instrução e autorização já atestada pelo serviço responsável; não construir uma API pública de Compliance dentro do Ledger.
2. UoW `SERIALIZABLE`: carregar hold por ID, verificar status e versão do estado, conta, saldo e outros holds efetivos; aplicar `hold.Evaluate` quando cabível.
3. Para `ALLOCATE`/`PARTIALLY_ALLOCATE`, alocar somente recursos realmente livres e elegíveis, com a mesma regra de concorrência usada por criação de hold e transações de saída.
4. Para `REJECT`, distinguir encerramento sem alocação de `RELEASED` (liberação de fundos que chegaram a estar bloqueados); avaliar um estado `REJECTED` com migration e transição de domínio explícita.
5. Persistir decisão, versão da política, histórico imutável e Outbox na mesma transação. Replays não podem alterar valores nem duplicar histórico/evento.
6. Concorrências `ResolvePendingHold` × `CreateTransaction`, `CreateHold` e `ReleaseHold` devem preservar disponibilidade.

**Não presumir que um `PENDING_REVIEW` consome saldo.** Se ele precisar impedir novas operações por decisão regulatória, isso será uma regra separada e expressa, não uma soma indevida no `held_amount`.

### 3B — Governança de `holds_eligible`

Criar mecanismo controlado para habilitar/desabilitar elegibilidade em contas e auditar mudanças.

- Identificar se a elegibilidade representa capacidade estrutural de uma conta receber holds ou uma configuração operacional. De preferência, contas onde fundos bloqueáveis existem devem ter política explícita e consistente desde a abertura.
- **Impedir `true → false` quando existe qualquer hold efetivo (`allocated - released > 0`).** Repetições idempotentes e erros devem ter códigos previsíveis.
- Decidir política para holds pendentes ao desabilitar (normalmente exigir resolução ou validação manual).
- **Defense in depth:** a validação de saldo disponível nunca deve ignorar holds existentes apenas porque `holds_eligible=false`. Alternativas: rejeitar a transação em estado inconsistente ou continuar respeitando os holds efetivos e sinalizar incidente. Provar por teste, inclusive para dados legados e mudanças fora do comando.
- Evitar `UPDATE accounts SET holds_eligible=false` silencioso; prover trilha auditável, autorização e, se houver impacto em projeção, Outbox.
- Garantir lock/isolamento compatível com `CreateHold`, `ReleaseHold` e `CreateTransaction`.
- Não confundir isto com transições de `Account.Status` (`ACTIVE/BLOCKED/CLOSED`).

### 3C — Execução contábil de fundos retidos (PR separado, depois das subfases 3A e 3B)

Novo comando proposto: `ExecuteFundsHold`, sem assumir transferência automática ao destinatário. A execução precisa receber **instrução autorizada e validada** com destino contábil definido.

- Especificar dados: `hold_id`, `execution_id`/idempotência, referência de autorização, quantia, moeda, contas de origem/destino, operação e metadados mínimos de auditoria.
- Permitir execução parcial somente quando aprovada; `executed_amount` deve ser rastreado **separadamente** de `released_amount`. Montante remanescente: `allocated - released - executed`, após acordar a semântica e migrar o schema.
- **Regra contábil:** execução reduz o valor retido e cria `Transaction + Movements + Entries` balanceadas em uma única UoW; nunca atualizar/deletar postings históricos.
- A execução autorizada não pode falhar apenas porque o dinheiro está bloqueado pelo próprio hold executado: contabilizar a redução da reserva e a transação no mesmo contexto atômico, **sem permitir que outros holds sejam consumidos**. Não contornar globalmente `validateAvailableFunds`; introduzir um mecanismo explícito e restrito ao hold específico, com verificação de identidade e valores.
- Definir de maneira explícita o destino contábil: transferência a conta vinculada, liquidação de garantia ou outra operação aprovada; **não codificar um destino judicial padrão**.
- Registrar vínculo auditável `hold_execution ↔ transaction_id`, histórico e Outbox no mesmo commit.
- Idempotência global por instrução de execução, fingerprint canônico e tratamento de resultado de commit desconhecido, conforme padrões do Writer.
- Executar testes concorrentes de `ExecuteFundsHold` × `ReleaseHold`, `CreateTransaction`, `CreateHold` e duas execuções simultâneas; manter invariantes de partidas dobradas, sequência por conta e `running_balance`.

## 4. Invariantes contábeis e financeiras

1. `requested >= allocated >= 0`, `pending = requested - allocated`, `released >= 0`, `executed >= 0`, `released + executed <= allocated`.
2. `effective_hold = allocated - released - executed`; esse montante e apenas ele limita o saldo livre.
3. Não existir dupla alocação do mesmo fundo em operações concorrentes; toda alteração relevante deve ser atômica com a transação financeira quando houver.
4. `JUDICIAL` só pode ser liberado/executado mediante instrução externa autorizada; o Ledger não inventa critérios de precedência jurídica.
5. Operações idempotentes não duplicam histórico, Outbox, movimentos ou entries.
6. O saldo oficial permanece sendo o `running_balance` confirmado; o Reader futuro materializará saldo disponível e bloqueado com eventos.
7. A falha em atualizar hold, histórico, ledger ou Outbox aborta **todo** o efeito financeiro da operação.
8. A política `BALANCE_UNRESTRICTED` não deve desativar restrições de disponibilidade de fundos bloqueados quando aplicáveis; validar também a natureza contábil da conta.
9. Nenhum evento de um hold é tratado como confirmação de execução contábil se não existir `transaction_id` correspondente confirmado.

## 5. Concurrency & data consistency

O protocolo atual depende de UoW `SERIALIZABLE` e leitura do último ledger state e dos holds. O Codex deve inspecionar os testes já existentes de concorrência e tornar explícito **qual unidade é serializada por conta**, especialmente em operações sem `Entry`.

Exigir testes com PostgreSQL real, barreiras de concorrência e assertions do estado final:
- Duas resoluções da mesma instrução; resolução × débito; resolução × criação de outro hold.
- Tentativa de desabilitar elegibilidade × criação de hold; mudança forçada de flag com hold efetivo × débito.
- Execução total × liberação total; duas execuções parciais somadas acima do retido; execução × débito ordinário.
- Commit incerto, erro no Outbox, `40001`, `40P01` e replay idempotente.

Não confiar em testes com fakes para demonstrar isolamento transacional.

## 6. Contratos e limites

- Reutilizar `internal/domain/hold`, `internal/application/command`, `internal/infra/postgres/repository`, factory/composer, migration Atlas, métricas e testes existentes.
- Novo schema apenas quando necessário: `REJECTED`, campos de `executed_amount`, `hold_executions`, auditoria de elegibilidade e constraints correspondentes.
- Eventos no Outbox são fatos versionados com identidade, `event_version`, `occurred_at` e dados suficientes para projeção futura; verificar convenções reais antes de alterar `outbox.EventType`.
- Não iniciar consumidor Kafka, cliente SISBAJUD, gRPC público para ordens judiciais nem Reader Java nesta fase.
- Proteger PII e informações de ordens; métricas sem IDs de conta/hold como labels.

## 7. Plano de entrega e aceite

**PR A (3A):** resolução de pendências, status/máquina de estados e idempotência, migration e integração com UoW/Outbox, testes concorrentes.

**PR B (3B):** governança de elegibilidade, repositório/comando auditável, prevenção de bypass mesmo sob inconsistência, testes.

**PR C (3C):** execução contábil autorizada, novos estados/valores, vínculos, integração estrita com `CreateTransaction`, histórico e eventos, testes de isolamento e partidas dobradas.

Aceitar cada PR somente com suíte de domínio, testes unitários `-race`, integração PostgreSQL, cobertura exigida pelo CI e Quality Gate verde. Não enfraquecer limites de cobertura para liberar merge. Documentar decisões em aberto e comandos reais executados.

## 8. Prompt para Codex — começar pelo PR A

> Trabalhe na branch `feat/011-funds-hold-lifecycle` do repositório Isura Ledger Writer. Leia `docs/idea-funds-hold-phase-3-lifecycle.md`, inspecione os contratos atuais e implemente **apenas a subfase 3A — ResolvePendingHold**. Inclua validação de autorização externa (referência/identidade, sem decidir mérito jurídico), máquina de estados explícita, idempotência, migração se necessária, histórico e Outbox atômicos e testes com PostgreSQL real para corrida contra débito/criação/liberação de hold. Preserve `entries` imutáveis e não implemente ainda controle administrativo de elegibilidade (3B), execução financeira (3C), Kafka, SISBAJUD ou Reader Java. Apresente decisões e limitações, resultados dos testes e riscos antes de abrir PR.
