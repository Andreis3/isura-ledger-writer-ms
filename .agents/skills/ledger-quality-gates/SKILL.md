---
name: ledger-quality-gates
description: Critérios transversais de integridade financeira, rastreabilidade e evidência para PRD, TechSpec, implementação, review e QA do Isura Ledger.
metadata:
  short-description: Gates financeiros verificáveis
---

# Gates de qualidade do Isura Ledger

Esta skill **não redefine** o modelo contábil. A fonte de verdade do comportamento atual é `../golang/references/ledger-engineering-guide.md`, em conjunto com `AGENTS.md`, rules aplicáveis e PRD/TechSpec aprovados. Quando houver divergência, registre o conflito e solicite decisão; não altere silenciosamente uma invariante.

## Quando aplicar

Carregue quando uma mudança afetar account, transaction, entry, saldo, política contábil, sequence_number, running_balance, idempotência, fingerprint, UoW, retry transacional, outbox, reconstrução histórica ou entrega de eventos financeiros.

Para mudanças sem impacto financeiro, documente apenas os gates aplicáveis; não gere casos fictícios.

## Matriz única de rastreabilidade

Use os mesmos IDs do planejamento à validação:

| Campo | Definição |
|---|---|
| `RF-*` | requisito funcional do PRD |
| `CA-*` | critério observável de aceitação |
| `INV-*` | invariante de negócio/garantia técnica |
| `TU-*` | teste unitário |
| `TI-*` | teste de integração, incluindo PostgreSQL real quando necessário |
| `E2E-*` | teste ponta a ponta, sem pressupor frontend |
| `task_*.md` | entrega de implementação, com evidência de testes |

Todo `INV-*` afetado precisa de `CA-*` correspondente (ou justificativa de ser exclusivamente técnico), mecanismo de proteção, caso de teste e evidência. Falta de evidência de uma invariante financeira bloqueia aprovação; testes não executados não equivalem a testes passando.

## Garantias atuais a verificar quando afetadas

1. **Append-only:** entradas confirmadas não sofrem UPDATE/DELETE no caminho normal; correções contábeis são novos lançamentos.
2. **Dinheiro válido:** amount positivo, moeda compatível com a conta, direction válido, sem ponto flutuante para saldo e amount; overflow de int64 é erro.
3. **Semântica da conta:** impacto de DEBIT/CREDIT depende do tipo de conta conforme `Account.ApplyEntry`; não presumir que DEBIT sempre diminui saldo.
4. **Sequência por conta:** numeração monotônica, `UNIQUE(account_id, sequence_number)` e running_balance derivado corretamente do estado anterior.
5. **Política de saldo:** `BALANCE_NON_NEGATIVE` impede estado confirmado negativo, inclusive sob concorrência.
6. **Idempotência:** chave ausente executa; mesma chave + fingerprint igual reproduz resultado; mesma chave + fingerprint diferente conflita.
7. **Concorrência transacional:** PostgreSQL SERIALIZABLE; retry completo e limitado para SQLSTATE 40001/40P01 e 23505 **somente** na constraint de sequência prevista; demais 23505 têm tratamento próprio.
8. **Atomicidade:** transaction + entries + outbox confirmam ou revertem juntos, conforme operação; publicação externa só após intenção persistida.
9. **Reconciliação:** replay histórico reproduz sequence_number/running_balance sem usar `ApplyHistoricalEntry` para contornar a política atual em novas transações.
10. **Limites do agregado:** a restrição atual de até duas entries e o caso TRANSFER de duas pernas são limitações específicas do modelo atual; não representam toda a contabilidade de partidas dobradas.

## Cenários mínimos, condicionados ao escopo

| Cenário | Verificação exigida |
|---|---|
| Dois débitos concorrentes na mesma conta | Só resultados serializáveis confirmam; sem gasto duplo; operação perdedora reavalia saldo após retry |
| Mesmo idempotency key/fingerprint | Uma operação financeira efetiva e replay consistente |
| Mesmo key, fingerprint distinto | Erro de conflito, sem novo lançamento |
| Erro antes do COMMIT | Nenhum efeito financeiro parcial persistido |
| COMMIT confirmado, resposta perdida | Retry da solicitação retorna resultado existente |
| Broker indisponível após COMMIT | Outbox persistida, reentrega posterior segura; não afirmar exactly-once de transporte |
| Falha de transação retryable | Rollback, releitura, recálculo, backoff limitado e respeito ao contexto |
| Reconcile/replay | Saldos e sequência reconstituídos corretamente |
| Overflow, valor inválido, moeda incompatível | Falha explícita sem persistência parcial |

Adapte aos recursos disponíveis; marque cenários fora do escopo como não aplicáveis e justifique. Não simule as garantias do banco apenas com mocks; use PostgreSQL real/isolado (por exemplo, Testcontainers), com asserts sobre registros persistidos. Não invente sucesso quando Docker, rede ou ambiente estiver indisponível.

## Gates por etapa

- **PRD:** expõe invariantes observáveis e CA sem antecipar SQL/algoritmo.
- **TechSpec:** identifica mecanismo, transação, erro, concorrência, testes e alternativas.
- **Tasks:** mapeia construção e validação de cada INV; mantém a cadeia RF/CA/INV/teste.
- **Implementação:** preserva domínio, SQLSTATE e constraints específicos, acrescenta testes e não marca testes ausentes como aprovados.
- **Review:** audita evidências, invariantes e trade-offs; reprova violações críticas.
- **QA:** executa cenários reais por perfil (API, worker, backend financeiro, UI quando existir) e relata os resultados.

## Registro de evidências

Para cada teste registre: ID, cenário, comandos realmente executados, ambiente (incluindo versão do PostgreSQL quando relevante), resultado esperado, resultado observado, evidência reproduzível e limitação. Remova credenciais, tokens e dados bancários sensíveis dos registros.

## Política de aprovação

**APROVADO:** toda invariante afetada está coberta por teste executado com sucesso, e nenhum bloqueador permanece.

**APROVADO COM RESSALVAS (somente review):** mesmas garantias de aprovação, mas com observações não bloqueantes de severidade média/baixa.

**REPROVADO:** violação de invariante, teste obrigatório falho ou implementação divergente sem autorização.

**BLOQUEADO:** validação obrigatória inviável por falta de ambiente, decisão de domínio ou integração; descreva o impedimento. Nunca converta ausência de evidência em aprovação.
