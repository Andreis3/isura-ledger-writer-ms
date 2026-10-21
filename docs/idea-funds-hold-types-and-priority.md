# Ideia de implementação — FundsHold: tipos de bloqueio de valores e hierarquia

> **Status:** proposta para implementação com Codex — **não é código implementado**.
>
> **Branch de trabalho:** `feat/009-create-blocks-types`
>
> **Escopo deste documento:** modelagem de tipos de bloqueio financeiro, política de prioridade e critérios de aceitação. A implementação deve ser precedida da inspeção da `main` e dos contratos atuais; não interpretar exemplos como regras jurídicas automáticas.

## 1. Problema

O Ledger precisa distinguir **bloqueio do estado da conta** (`ACTIVE`, `BLOCKED`, `CLOSED`) de **bloqueio de um valor específico** dentro de uma conta. Uma conta pode continuar ativa e receber créditos enquanto parte dos recursos está indisponível.

Exemplos de origem: ordem judicial, restrição regulatória, garantia constituída, retenção contratual autorizada e reserva operacional de liquidação. Pode haver vários bloqueios simultâneos sobre uma mesma conta; o Ledger não pode permitir que o mesmo dinheiro seja alocado duas vezes, nem apagar o histórico para resolver conflitos.

O Writer permanece a fonte de verdade das `Entries` e dos respectivos `running_balance`. Um bloqueio de disponibilidade **não é, por si só, um débito/transferência**. Uma execução que efetivamente movimenta valores deve ocorrer por `Transaction + Movements + Entries`, com partidas dobradas.

## 2. Modelo conceitual proposto

Criar uma capacidade `FundsHold` dentro do **mesmo Bounded Context Ledger**, independente de `Account.Status`, com identidade e ciclo de vida próprios. O nome, diretórios e interfaces definitivos devem seguir as convenções verificadas no repositório (Go, DDD, UoW, PostgreSQL, Outbox).

### Tipos iniciais (proposta)

| Tipo | Significado | Autoridade/origem típica |
|---|---|---|
| `JUDICIAL` | Ordem judicial de indisponibilidade | Plataforma jurídica/Compliance após validação da ordem |
| `REGULATORY` | Restrição específica imposta por obrigação legal/regulatória | Compliance/autoridade competente |
| `GUARANTEE` | Recursos vinculados a garantia | Plataforma de crédito/garantias |
| `CONTRACTUAL` | Retenção prevista em contrato e juridicamente autorizada | Crédito/recuperação ou outro serviço responsável |
| `OPERATIONAL` | Reserva temporária para processamento de operações | Pagamentos/liquidação |

**Não inferir que `JUDICIAL > REGULATORY > GUARANTEE > CONTRACTUAL > OPERATIONAL` seja uma ordem legal universal.** Esta é, no máximo, uma **precedência operacional configurável** para avaliar conflitos de alocação de fundos **livres e elegíveis**. Competição entre ordens, gravames, garantias e restrições legais exige decisão de uma política/autoridade competente.

### Identidade e atributos mínimos sugeridos

- Identificação do bloqueio, conta contábil, moeda e tipo;
- Referência externa estável à instrução/origem, identificação da autoridade/sistema solicitante e chave idempotente;
- Valor solicitado, valor efetivamente bloqueado, valor liberado/executado e valor pendente, conforme modelo escolhido;
- Status e versão da política aplicada;
- Instantes de criação/alteração, referência à justificativa/instrução e metadados auditáveis;
- Referências a movimentações financeiras decorrentes de **execução**, se existirem.

Usar inteiros em unidades mínimas da moeda (sem `float`). Evitar dados pessoais sensíveis em logs/eventos por padrão.

### Ciclo de vida sugerido

`REQUESTED` → `ACTIVE` / `PARTIALLY_HELD` / `PENDING_REVIEW` / `REJECTED`, depois `PARTIALLY_RELEASED`, `RELEASED`, `PARTIALLY_EXECUTED` ou `EXECUTED` conforme instruções válidas. **A máquina de estados definitiva deve ser explicitamente especificada**, com transições autorizadas por tipo/origem e sem exclusão retroativa do histórico. Não misturar automaticamente "quantia solicitada maior que fundos elegíveis" com falha técnica: pode ser retenção parcial ou ordem pendente, segundo a regra autorizada.

## 3. Hierarquia: separar precedência de elegibilidade

Não espalhar prioridades numéricas ou `switch` jurídico dentro do agregado. Propor uma **política de alocação** explícita e versionada, que avalia:

1. Origem, validade, vigência e escopo da instrução;
2. Recursos elegíveis, restrições existentes e saldo disponível;
3. Se a instrução permite cobertura parcial e/ou bloqueio de entradas futuras;
4. Se há conflito que exige decisão externa em vez de preempção automática;
5. Quem pode autorizar liberação, execução ou substituição.

**Nunca sobrescrever/excluir um hold anterior para favorecer outro.** Se uma realocação for autorizada, registrar explicitamente a autorização, a movimentação lógica e a trilha de auditoria. Entre duas ordens `JUDICIAL`, não decidir precedência apenas pela data, ID ou prioridade numérica.

Uma API de domínio ilustrativa poderia receber `AccountFundsState`, holds ativos e `HoldRequest` e retornar `AllocationDecision` (quantia alocada, pendente, conflitos e decisão requerida). Codex deve propor interfaces mínimas, evitando criar abstrações desnecessárias na primeira entrega.

## 4. Saldos e invariantes

Distinguir, no mínimo:

- **Saldo contábil**: último `running_balance` confirmado da conta;
- **Total indisponível**: soma das alocações efetivas e não liberadas dos holds sobre recursos elegíveis, sem dupla contagem;
- **Saldo disponível**: saldo contábil menos total indisponível (respeitando política de saldo, moeda e eventuais limitações adicionais).

Invariantes esperadas:

- Quantias não negativas e todas as operações de uma mesma alocação na moeda da conta;
- Valor efetivamente bloqueado não excede valor solicitado, salvo instrução formalmente alterada;
- Sem dupla alocação do mesmo recurso;
- Liberações e execuções não excedem o restante efetivamente bloqueado;
- Idempotência para instruções de criar, alterar, liberar e executar bloqueio;
- Histórico auditável, inclusive rejeições relevantes e alterações de instrução;
- O fluxo de débito da conta considera holds ativos antes de confirmar uma `Transaction`;
- Alterações em holds e postagens concorrentes na mesma conta devem respeitar um protocolo transacional coerente.

**Concorrência é requisito crítico.** Reaproveitar PostgreSQL e a abordagem UoW/retry existentes, mas **não assumir** que apenas `SERIALIZABLE` resolve invariantes se as transações envolvidas não leem/escrevem os mesmos fatos relevantes. Definir testes de corrida: criação de hold × débito, dois holds simultâneos, liberação × execução e evento duplicado.

## 5. Fronteiras de responsabilidade e eventos

- **Kafka (cross-platform):** sistemas de Jurídico, Compliance, Crédito e Pagamentos podem enviar instruções/eventos autorizados para o Ledger. O contrato de origem deve autenticar/autorizar a instrução; o Ledger **não inventa** fundamento jurídico.
- **Writer (Go):** valida idempotência e invariantes financeiras, mantém holds, executa operações transacionais e persiste Outbox.
- **NATS JetStream (intra-Ledger):** publica fatos do Ledger para projeção por consumidores internos, incluindo o futuro Reader Java.
- **Reader (Java, futuro):** materializa `account_balance`, `held_amount` e `available_balance` além dos movimentos/extratos, sem ler o banco do Writer diretamente.

Avaliar, sem implementar nesta primeira fase, contratos versionados como `FundsHoldCreated`, `FundsHoldPartiallyAllocated`, `FundsHoldReleased` e `FundsHoldExecuted`. Publicar eventos somente após a persistência atômica da intenção no Outbox. Consumidores devem ser idempotentes.

## 6. Entregas em fases para Codex

### Fase 1 — Tipos, regra de prioridade e testes de domínio (escopo prioritário desta branch)

- Inspecionar código existente de `account`, `balance`, `transaction`, `money`, `validator` e padrões de testes;
- Propor `HoldType`, classificações, política operacional versionável e resultado explícito de conflitos;
- Implementar **somente** o modelo de domínio necessário à definição e validação dos tipos, caso a tarefa Codex seja executar a fase 1;
- Cobrir tipos válidos/inválidos, alocação parcial, mesma prioridade, conflito judicial, imutabilidade do histórico e ausência de precedência jurídica hardcoded;
- Não alterar schema, APIs, Kafka, NATS ou `CreateTransaction` sem tarefa posterior específica.

### Fase 2 — Persistência e integração com o Writer (futura)

- Definir migrations Atlas, repositório, constraints e idempotência;
- UoW para criar/liberar/executar holds, logs e Outbox;
- Garantir que `CreateTransaction` avalie a disponibilidade na mesma estratégia de concorrência das alterações de hold;
- Testes integrados de saldo insuficiente, transações concorrentes, rollback e recuperação.

### Fase 3 — Interfaces e projeções (futura)

- Consumidor das instruções cross-platform Kafka, autenticação/autorização e tratamento fora de ordem;
- Eventos internos NATS, contrato versionado e testes de compatibilidade;
- Reader Java com projeções e processamento idempotente;
- Runbooks operacionais, alertas, conciliação de holds e procedimentos jurídicos aprovados.

## 7. Critérios de aceitação da fase 1

1. Os cinco tipos são definidos e validados sem depender do transporte.
2. `Account.Status` e bloqueio financeiro são conceitos separados.
3. A hierarquia operacional é configurável/testável e não afirma uma prioridade jurídica absoluta.
4. Dois holds sobre o mesmo saldo não podem gerar dupla disponibilidade/alocação quando a política for aplicada.
5. Conflitos não resolvíveis pela política retornam decisão explícita de `PENDING_REVIEW` ou equivalente, nunca preempção silenciosa.
6. Tipos e políticas têm testes unitários reproduzíveis.
7. Não há alteração desnecessária no agregado `Transaction` nem no fluxo de Outbox nesta fase.
8. A implementação não gera débitos contábeis ao simplesmente bloquear disponibilidade.

## 8. Decisões de negócio pendentes (não assumir por conta própria)

- Fonte autorizada e documento/instrução válido por tipo;
- Tratamento de recursos legalmente protegidos e fundos previamente dados em garantia;
- Bloqueio parcial vs. rejeição quando o saldo é insuficiente;
- Efeito sobre créditos futuros e expiração de holds operacionais;
- Política para ordens concorrentes do mesmo tipo, inclusive judiciais;
- Regras de alteração, liberação, execução e compensação;
- Como auditar políticas antigas após troca de versão.

## Prompt sugerido para Codex

> Trabalhe na branch `feat/009-create-blocks-types` do Isura Ledger Writer. Leia integralmente `docs/idea-funds-hold-types-and-priority.md` e inspecione os modelos reais antes de codificar. Execute **apenas a Fase 1** (tipos de bloqueio, política explícita e testes unitários), preservando as invariantes do Writer. Não introduza uma hierarquia jurídica fixa e não implemente Kafka, NATS, migrations ou integração com `CreateTransaction` nesta tarefa. Apresente decisões de modelagem, testes executados e questões ainda abertas antes de abrir PR.
