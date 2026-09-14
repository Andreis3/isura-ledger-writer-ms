---
name: executar-review
description: Revisão de código — revise e estabilize a implementação de uma funcionalidade quanto à conformidade com as regras do projeto, aderência à TechSpec, completude das tarefas, qualidade técnica, segurança e testes, produzindo relatório final e veredito. Use quando o usuário pedir para revisar código, executar code review, validar conformidade técnica ou corrigir problemas encontrados durante a revisão. Não use para validar comportamento funcional de produto em QA (executar-qa) nem para implementar novas tarefas.
argument-hint: --prd nome-da-funcionalidade
---

O argumento `--prd` identifica o slug da funcionalidade.

Sem argumento, localize a pasta correspondente em:

```text
./tasks/prd-*/
```

Os arquivos obrigatórios são:

```text
./tasks/prd-[slug]/techspec.md
./tasks/prd-[slug]/tasks.md
```

Consulte também:

```text
./tasks/prd-[slug]/prd.md
./tasks/prd-[slug]/task_[num].md
```

quando necessário para esclarecer requisitos, escopo ou tarefas implementadas.

Leia obrigatoriamente:

```text
./AGENTS.md
./.agents/rules/
```

Carregue somente as skills aplicáveis em:

```text
./.agents/skills/
```

Gere e mantenha o relatório em:

```text
./tasks/prd-[slug]/codereview.md
```

A revisão deve utilizar:

* `AGENTS.md` e `.agents/rules/` como fonte das convenções e exigências do projeto;
* `techspec.md` como fonte das decisões técnicas esperadas;
* `tasks.md` e `task_[num].md` como fonte do escopo implementado;
* `prd.md` apenas para esclarecer comportamento ou requisito de produto quando necessário.

Não presuma tecnologias, ferramentas, comandos, portas ou processos que não estejam definidos pelo projeto.

---

## Responsabilidade da revisão

A revisão de código valida principalmente:

* conformidade com regras do projeto;
* aderência à TechSpec;
* completude das tarefas;
* qualidade técnica;
* consistência arquitetural;
* contratos;
* tratamento de erros;
* persistência;
* concorrência;
* idempotência;
* segurança;
* observabilidade;
* testes;
* manutenção;
* riscos técnicos.

A validação funcional completa dos critérios de aceitação pertence ao QA.

Use `executar-review` para responder:

```text
"O código está tecnicamente correto e aderente ao projeto?"
```

Use `executar-qa` para responder:

```text
"A funcionalidade entregue se comporta como o produto exige?"
```

A revisão pode consultar critérios de aceitação para compreender o contexto, mas não substitui a validação funcional sistemática do QA.

---

## Limites de escopo

Durante a revisão, corrija somente problemas necessários para fazer a implementação:

* respeitar as regras do projeto;
* aderir à TechSpec;
* completar corretamente a tarefa;
* preservar segurança;
* preservar corretude;
* passar as validações obrigatórias.

Não use a revisão para:

* implementar nova funcionalidade;
* expandir escopo;
* antecipar tarefas futuras;
* alterar requisitos;
* reescrever arquitetura sem necessidade;
* realizar refatoração ampla não relacionada;
* modificar a TechSpec apenas para justificar o código existente.

Quando uma correção exigir mudança relevante de produto, arquitetura ou escopo, registre como bloqueio.

---

## Estados possíveis

O resultado final deve ser um destes estados:

```text
APROVADO
APROVADO COM RESSALVAS
REPROVADO
BLOQUEADO
```

### APROVADO

Use quando:

* não houver findings bloqueantes;
* a implementação estiver aderente à TechSpec;
* regras obrigatórias estiverem atendidas;
* tarefas concluídas estiverem realmente completas;
* testes e validações obrigatórias passarem;
* não houver problema relevante de segurança ou corretude.

### APROVADO COM RESSALVAS

Use somente quando existirem melhorias não bloqueantes que:

* não violem requisitos;
* não violem regras obrigatórias;
* não contradigam a TechSpec;
* não comprometam segurança;
* não comprometam corretude;
* não façam testes obrigatórios falhar.

### REPROVADO

Use quando houver:

* teste obrigatório falhando;
* violação de regra obrigatória;
* task incompleta marcada como concluída;
* desvio não justificado da TechSpec;
* vulnerabilidade relevante;
* problema de corretude;
* finding bloqueante não resolvido.

### BLOQUEADO

Use quando a revisão não puder chegar a um veredito válido por:

* ambiente indisponível;
* dependência externa necessária;
* ausência de credencial;
* conflito entre documentos;
* decisão técnica pendente;
* informação obrigatória ausente.

Não use `BLOQUEADO` para mascarar implementação comprovadamente incorreta.

---

## Severidade dos findings

Classifique cada finding como:

### Crítica

Problema que pode causar:

* perda ou corrupção de dados;
* vulnerabilidade grave;
* quebra de invariantes críticas;
* indisponibilidade severa;
* execução incorreta com impacto grave.

### Alta

Problema que:

* viola TechSpec ou regra obrigatória;
* compromete comportamento técnico importante;
* causa risco relevante de segurança;
* quebra contrato;
* introduz condição de corrida séria;
* torna a tarefa incompleta.

### Média

Problema que:

* reduz manutenção;
* cria inconsistência relevante;
* introduz duplicação significativa;
* aumenta risco técnico futuro;
* possui impacto limitado sem quebrar requisito atual.

### Baixa

Problema relacionado principalmente a:

* legibilidade;
* nomenclatura;
* organização;
* simplificação;
* pequenas inconsistências sem impacto funcional relevante.

Recomendação opcional não precisa ser registrada como finding.

---

## Fluxo

### 1. Analisar

Leia integralmente:

```text
./AGENTS.md
```

e todas as rules aplicáveis em:

```text
./.agents/rules/
```

Leia:

```text
./tasks/prd-[slug]/techspec.md
./tasks/prd-[slug]/tasks.md
```

e todos os arquivos:

```text
./tasks/prd-[slug]/task_[num].md
```

relevantes para as tarefas concluídas ou para o escopo da revisão.

Consulte:

```text
./tasks/prd-[slug]/prd.md
```

somente quando necessário para esclarecer:

* requisito;
* comportamento esperado;
* escopo;
* critério de aceitação.

Identifique:

* arquitetura esperada;
* componentes;
* contratos;
* persistência;
* eventos;
* integrações;
* concorrência;
* idempotência;
* observabilidade;
* segurança;
* decisões técnicas;
* arquivos relevantes;
* testes esperados;
* rules aplicáveis;
* skills aplicáveis.

**Conclua quando:** arquitetura, escopo, regras e expectativas técnicas estiverem claros.

---

### 2. Inicializar o relatório

Crie ou atualize:

```text
./tasks/prd-[slug]/codereview.md
```

seguindo:

```text
./references/TEMPLATE.md
```

Registre inicialmente:

* data;
* branch;
* commit, quando disponível;
* escopo da revisão;
* tasks verificadas;
* rules aplicáveis;
* ambiente.

Atualize o relatório durante a revisão.

Não espere o fim para registrar findings relevantes.

---

### 3. Identificar mudanças

Determine quais mudanças pertencem à funcionalidade.

Use, quando disponível:

* diff da branch;
* diff da worktree;
* histórico associado;
* arquivos listados na TechSpec;
* arquivos listados nas tarefas.

Não revise o repositório inteiro quando o escopo da funcionalidade puder ser identificado.

Inclua dependências diretamente afetadas quando necessário para compreender o impacto.

**Conclua quando:** o conjunto principal de mudanças estiver identificado.

---

### 4. Conformidade com regras

Confira cada mudança contra:

```text
AGENTS.md
.agents/rules/*
```

Para cada violação, registre:

* regra;
* arquivo;
* linha ou região;
* descrição;
* severidade;
* impacto;
* correção necessária.

Não registre preferência pessoal como violação.

Não invente convenções que não estejam definidas no projeto.

**Conclua quando:** todas as mudanças relevantes tiverem sido verificadas contra as regras aplicáveis.

---

### 5. Aderência à TechSpec

Compare a implementação com a TechSpec.

Verifique, quando aplicável:

* arquitetura;
* componentes;
* responsabilidades;
* interfaces;
* contratos;
* modelos de dados;
* persistência;
* endpoints;
* eventos;
* mensageria;
* integrações;
* idempotência;
* concorrência;
* tratamento de erros;
* observabilidade;
* segurança;
* estratégia de testes.

Classifique cada item como:

```text
CONFORME
DESVIO JUSTIFICADO
DESVIO NÃO JUSTIFICADO
NÃO APLICÁVEL
```

Pequenas adaptações tecnicamente equivalentes podem ser consideradas justificadas.

Mudança relevante de decisão arquitetural sem justificativa deve gerar finding.

**Conclua quando:** cada decisão técnica relevante estiver confirmada ou classificada.

---

### 6. Completude das tarefas

Para cada tarefa marcada como concluída:

```text
[x]
```

verifique:

* objetivo implementado;
* subtarefas concluídas;
* arquivos esperados presentes;
* testes previstos presentes;
* definição de concluído atendida;
* critérios relacionados rastreados;
* itens obrigatórios concluídos.

A validação funcional completa dos critérios continua sendo responsabilidade do QA.

Se uma tarefa estiver marcada como concluída, mas possuir implementação ou teste obrigatório ausente, registre finding.

**Conclua quando:** todas as tarefas concluídas tiverem sido verificadas.

---

### 7. Revisar qualidade técnica

Avalie somente aspectos aplicáveis à implementação.

Considere:

#### Design

* responsabilidades claras;
* coesão;
* acoplamento;
* abstrações necessárias;
* ausência de abstrações artificiais;
* aderência aos padrões existentes.

#### Legibilidade

* nomenclatura;
* fluxo de controle;
* complexidade;
* duplicação relevante;
* comentários úteis.

#### Tratamento de erros

* propagação;
* wrapping;
* tipos ou códigos;
* erros ignorados;
* consistência com o padrão do projeto.

#### Persistência

Quando aplicável:

* atomicidade;
* transações;
* constraints;
* índices;
* consistência;
* migrations;
* concorrência.

#### Concorrência

Quando aplicável:

* race conditions;
* deadlocks;
* acesso compartilhado;
* sincronização;
* cancelamento;
* timeouts;
* leaks.

#### Idempotência

Quando aplicável:

* chave;
* escopo;
* repetição;
* persistência;
* retry;
* concorrência.

#### Eventos e mensageria

Quando aplicável:

* contrato;
* producer;
* consumer;
* retry;
* ordenação;
* duplicidade;
* idempotência;
* tratamento de falha.

#### Segurança

Quando aplicável:

* autenticação;
* autorização;
* input;
* exposição de dados;
* secrets;
* injection;
* permissões;
* logging de dados sensíveis.

#### Observabilidade

Quando aplicável:

* logs;
* contexto;
* métricas;
* traces;
* correlação;
* ausência de dados sensíveis.

Não crie findings para categorias não aplicáveis.

---

### 8. Executar testes e validações

Leia os comandos definidos em:

```text
AGENTS.md
.agents/rules/
```

e na configuração do projeto.

Execute somente as validações aplicáveis definidas pelo projeto.

Podem incluir:

* format;
* geração;
* lint;
* análise estática;
* build;
* testes de unidade;
* testes de integração;
* cobertura.

Não invente comandos, ferramentas, tecnologias ou processos.

Quando algum comando exigir execução da aplicação, siga as regras de isolamento e runtime definidas no projeto.

Quando houver meta de cobertura definida pelo projeto, valide-a.

Não invente meta de cobertura.

Registre:

* comando ou validação;
* resultado;
* observações;
* cobertura, quando aplicável.

**Conclua quando:** todas as validações obrigatórias tiverem sido executadas ou explicitamente bloqueadas.

---

### 9. Registrar findings

Use IDs sequenciais:

```text
REV-01
REV-02
REV-03
```

Para cada finding registre:

* severidade;
* categoria;
* arquivo;
* linha ou região;
* regra ou decisão relacionada;
* descrição;
* impacto;
* correção sugerida;
* status.

Categorias sugeridas:

```text
Rule
Arquitetura
Contrato
Persistência
Concorrência
Idempotência
Segurança
Erro
Observabilidade
Teste
Manutenção
Task incompleta
```

Estados permitidos:

```text
Aberto
Em correção
Corrigido
Bloqueado
Aceito como ressalva
```

Todo finding deve possuir fundamento concreto.

Evite comentários vagos ou baseados apenas em preferência.

---

### 10. Corrigir e revalidar

Corrija findings quando:

* estiverem dentro do escopo;
* a correção for tecnicamente clara;
* não exigir mudança relevante de produto;
* não exigir alteração arquitetural fora da TechSpec.

Para cada correção:

1. identifique a causa raiz;
2. aplique a menor alteração adequada;
3. ajuste ou crie testes quando necessário;
4. execute novamente as validações afetadas;
5. atualize o finding.

Não use review para refatorar amplamente o módulo.

Se a correção exigir:

* mudança do PRD;
* mudança relevante da TechSpec;
* novo requisito;
* mudança de escopo;

marque como `Bloqueado`.

**Conclua quando:** findings corrigíveis estiverem resolvidos e os demais classificados.

---

### 11. Revalidar

Depois das correções, execute novamente as validações relevantes:

* testes afetados;
* lint;
* build;
* análise estática;
* rules afetadas;
* aderência à TechSpec afetada.

Não considere um finding corrigido somente porque o código foi alterado.

Ele deve ser revalidado.

Se uma correção introduzir regressão, registre novo finding ou reabra o existente.

---

### 12. Determinar o veredito

#### APROVADO

Use quando:

```text
0 findings bloqueantes
+
testes obrigatórios passando
+
rules obrigatórias atendidas
+
TechSpec respeitada
+
tasks concluídas realmente completas
```

#### APROVADO COM RESSALVAS

Use somente quando existirem melhorias não bloqueantes e todos os requisitos obrigatórios continuarem atendidos.

#### REPROVADO

Use quando houver:

* finding crítico ou alto aberto;
* teste obrigatório falhando;
* regra obrigatória violada;
* desvio relevante da TechSpec;
* task incompleta marcada como concluída;
* problema relevante de segurança ou corretude.

#### BLOQUEADO

Use quando não for possível concluir a revisão por dependência ou decisão externa.

---

### 13. Finalizar o relatório

Atualize:

```text
./tasks/prd-[slug]/codereview.md
```

seguindo:

```text
./references/TEMPLATE.md
```

O relatório final deve conter:

* escopo;
* ambiente;
* conformidade com rules;
* aderência à TechSpec;
* tarefas verificadas;
* testes;
* cobertura;
* findings;
* correções;
* bloqueios;
* ressalvas;
* pontos positivos;
* recomendações;
* veredito.

Não remova findings corrigidos.

Mantenha histórico suficiente para demonstrar:

```text
problema identificado
        ↓
correção aplicada
        ↓
teste/revalidação
        ↓
status corrigido
```

---

### 14. Encerrar o ambiente

Desligue somente os serviços e processos iniciados por esta execução.

Siga as regras de cleanup definidas no `AGENTS.md` e nas rules.

Não encerre:

* processos do usuário;
* recursos de outra worktree;
* serviços compartilhados já existentes.

Faça a limpeza também quando a revisão:

* for aprovada;
* for reprovada;
* ficar bloqueada;
* for interrompida.

---

### 15. Reportar

Informe:

* veredito;
* total de findings;
* findings por severidade;
* quantidade corrigida;
* quantidade pendente;
* testes e validações executados;
* cobertura, quando aplicável;
* caminho do `codereview.md`.

Se houver findings bloqueantes, destaque-os.

Se houver somente ressalvas, deixe explícito que não impedem aprovação.

---

## Resultado esperado

```text
TechSpec + Tasks + Código
          ↓
       analisar
          ↓
 criar codereview.md
          ↓
 identificar mudanças
          ↓
 validar rules
          ↓
 validar TechSpec
          ↓
 validar tasks
          ↓
 revisar qualidade
          ↓
 executar testes
          ↓
 registrar findings
          ↓
 corrigir
          ↓
 revalidar
          ↓
 determinar veredito
          ↓
 finalizar relatório
          ↓
 limpar ambiente
          ↓
 reportar
```

A revisão termina somente quando houver evidência suficiente para classificar a implementação como `APROVADO`, `APROVADO COM RESSALVAS`, `REPROVADO` ou `BLOQUEADO`.
