# Relatório de revisão de código — [Nome da funcionalidade]

## Resumo

* Data: [data]
* Branch: [branch]
* Commit: [commit ou não disponível]
* Status: APROVADO / APROVADO COM RESSALVAS / REPROVADO / BLOQUEADO
* Findings encontrados: [X]
* Findings corrigidos: [Y]
* Findings pendentes: [Z]

## Escopo da revisão

[Descreva brevemente os componentes, tarefas, arquivos ou mudanças cobertas pela revisão.]

### Referências

* PRD: `tasks/prd-[slug]/prd.md`
* TechSpec: `tasks/prd-[slug]/techspec.md`
* Tasks: `tasks/prd-[slug]/tasks.md`

## Ambiente de validação

* Ambiente/worktree: [descrição]
* Aplicações ou serviços envolvidos: [descrição]
* Comandos principais utilizados: [comandos ou referência ao AGENTS.md]
* Dependências relevantes: [quando aplicável]

## Conformidade com regras

| Regra   | Status                   | Evidência/Observação |
| ------- | ------------------------ | -------------------- |
| [regra] | OK / NOK / NÃO APLICÁVEL | [observação]         |

## Aderência à TechSpec

| Item ou decisão técnica | Status                                                                 | Observações  |
| ----------------------- | ---------------------------------------------------------------------- | ------------ |
| [decisão]               | CONFORME / DESVIO JUSTIFICADO / DESVIO NÃO JUSTIFICADO / NÃO APLICÁVEL | [observação] |

## Tarefas verificadas

| Tarefa | Status declarado | Status verificado     | Observações  |
| ------ | ---------------- | --------------------- | ------------ |
| [1.0]  | COMPLETA         | COMPLETA / INCOMPLETA | [observação] |

## Validações e testes

| Tipo       | Comando ou caso | Resultado                       | Observações  |
| ---------- | --------------- | ------------------------------- | ------------ |
| Build      | [comando]       | PASSOU / FALHOU / BLOQUEADO     | [observação] |
| Lint       | [comando]       | PASSOU / FALHOU / BLOQUEADO     | [observação] |
| Unidade    | [caso]          | PASSOU / FALHOU / NÃO APLICÁVEL | [observação] |
| Integração | [caso]          | PASSOU / FALHOU / NÃO APLICÁVEL | [observação] |

### Cobertura

* Cobertura obtida: [valor ou NÃO APLICÁVEL]
* Meta definida pelo projeto: [valor ou NÃO APLICÁVEL]
* Resultado: ATENDE / NÃO ATENDE / NÃO APLICÁVEL

## Findings

| ID     | Severidade                     | Categoria   | Arquivo/Linha     | Regra ou decisão relacionada | Descrição   | Impacto   | Status                                                              |
| ------ | ------------------------------ | ----------- | ----------------- | ---------------------------- | ----------- | --------- | ------------------------------------------------------------------- |
| REV-01 | Crítica / Alta / Média / Baixa | [categoria] | `[arquivo:linha]` | [rule/TechSpec]              | [descrição] | [impacto] | Aberto / Em correção / Corrigido / Bloqueado / Aceito como ressalva |

## Correções realizadas

| Finding | Correção            | Teste ou revalidação | Resultado       |
| ------- | ------------------- | -------------------- | --------------- |
| REV-01  | [correção aplicada] | [teste ou validação] | PASSOU / FALHOU |

## Bloqueios

| ID     | Descrição   | Impacto   | Decisão ou dependência necessária |
| ------ | ----------- | --------- | --------------------------------- |
| BLK-01 | [descrição] | [impacto] | [ação necessária]                 |

Se não houver bloqueios:

```text
Nenhum.
```

## Ressalvas

[Liste somente melhorias não bloqueantes que não violem requisitos, rules ou TechSpec.]

* [ressalva]

Se não houver:

```text
Nenhuma.
```

## Pontos positivos

* [Decisão ou implementação tecnicamente bem executada.]
* [Boa aderência a padrão relevante.]
* [Boa cobertura ou organização, quando aplicável.]

Não use elogios genéricos.

## Recomendações

[Inclua apenas melhorias futuras não obrigatórias.]

* [recomendação]

Não misture recomendações com findings que deveriam bloquear aprovação.

## Conclusão

### Veredito

**[APROVADO / APROVADO COM RESSALVAS / REPROVADO / BLOQUEADO]**

[Parecer objetivo sobre qualidade técnica, aderência à TechSpec, conformidade com rules, completude das tarefas e resultado das validações.]

### Resumo final

* Rules obrigatórias: ATENDIDAS / NÃO ATENDIDAS / BLOQUEADAS
* TechSpec: CONFORME / COM DESVIOS / BLOQUEADA
* Tasks: COMPLETAS / INCOMPLETAS / BLOQUEADAS
* Testes obrigatórios: PASSANDO / FALHANDO / BLOQUEADOS
* Findings bloqueantes abertos: [X]
* Ressalvas não bloqueantes: [Y]
