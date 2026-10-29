# Relatório de QA — [Nome da funcionalidade]

As evidências da ferramenta de navegador devem estar salvas em `tasks/prd-[slug]/evidences/`.

## Resumo
- Data: [data]
- Status: APROVADO / REPROVADO / BLOQUEADO
- Total de critérios de aceitação: [X]
- Critérios de aceitação atendidos: [Y]
- Bugs encontrados: [Z]

## Critérios de aceitação verificados
| ID | Critério de aceitação | Casos de teste | Status | Evidência |
|----|-----------------------|----------------|--------|-----------|
| CA-01 | [descrição] | [TU-01, TI-01, E2E-01] | PASSOU/FALHOU | [captura de tela] |

## Testes E2E executados
| ID | Fluxo | Resultado | Observações |
|----|-------|-----------|-------------|
| E2E-01 | [fluxo] | PASSOU/FALHOU | [obs] |

## Testes automatizados e cobertura
| Camada | ID | Resultado | Validação/comando | Observações |
|--------|----|-----------|-------------------|------------|
| Unidade/Integração | TU-01/TI-01 | PASSOU/FALHOU/NÃO APLICÁVEL | [definido no AGENTS.md] | [obs] |

- Cobertura: [valor, meta ou NÃO APLICÁVEL]

## Acessibilidade
- [checklist de a11y]

## Bugs encontrados e corrigidos
| ID | Descrição | Severidade | Status | Correção | Teste de regressão | Evidência |
|----|-----------|------------|--------|----------|--------------------|-----------|
| BUG-01 | [descrição] | Alta/Média/Baixa | Corrigido/Bloqueado | [correção ou motivo do bloqueio] | [teste] | [link da evidência] |

## Conclusão
[Parecer final do QA]


## Perfil e invariantes

- Perfil validado: backend-financeiro / backend-api / mensageria-worker / frontend / combinado
- Recursos usados: [PostgreSQL, broker, HTTP, browser, conforme aplicável]
- Fluxos de UI: [APLICÁVEL / NÃO APLICÁVEL — serviço sem UI]

| Invariante | Critério de aceitação | Caso de teste | Resultado | Evidência verificável |
|---|---|---|---|---|
| INV-01 | CA-01 | TI-01 | PASSOU / FALHOU / BLOQUEADO | [assertiva, log sanitizado, relatório] |

## Falhas e concorrência

| Cenário | Resultado esperado | Resultado observado | Teste | Evidência |
|---|---|---|---|---|
| [concorrência, retry, pós-COMMIT, reconciliação, se aplicável] | [garantia] | [resultado] | TI-02 | [evidência] |

Em serviços sem UI, as seções de acessibilidade, responsividade e evidências de navegador devem ser marcadas **Não aplicável — serviço sem interface visual**. Não invente screenshots.
