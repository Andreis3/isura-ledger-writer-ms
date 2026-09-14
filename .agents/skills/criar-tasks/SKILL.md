---
name: criar-tasks
description: Tarefas — decomposição de uma funcionalidade em tarefas de implementação a partir do PRD e da TechSpec existentes em `tasks/prd-*/`. Use quando o usuário pedir para decompor uma funcionalidade em tarefas ou planejar sua execução. Não use para redigir o PRD (criar-prd) nem a TechSpec (criar-techspec).
argument-hint: --prd nome-da-funcionalidade
---

O argumento `--prd` identifica o slug da funcionalidade.

Sem argumento, localize a pasta correspondente em:

```text
./tasks/prd-*/
```

Os arquivos obrigatórios são:

```text
./tasks/prd-[slug]/prd.md
./tasks/prd-[slug]/techspec.md
```

Se o PRD estiver ausente, interrompa e indique:

```text
/criar-prd
```

Se a TechSpec estiver ausente, interrompa e indique:

```text
/criar-techspec
```

Não gere tarefas diretamente a partir de uma ideia ou somente do PRD. A TechSpec é necessária para definir corretamente componentes, contratos, dependências técnicas e casos de teste.

Cada tarefa deve representar uma **entrega incremental, implementável e verificável**, com:

* objetivo claro;
* escopo limitado;
* dependências explícitas;
* requisitos relacionados;
* critérios de aceitação relacionados;
* testes próprios;
* arquivos relevantes;
* definição clara de conclusão.

Use o PRD como fonte de verdade para:

* requisitos funcionais (`RF-*`);
* critérios de aceitação (`CA-*`);
* escopo;
* itens fora do escopo.

Use a TechSpec como fonte de verdade para:

* arquitetura;
* componentes;
* contratos;
* persistência;
* integrações;
* decisões técnicas;
* arquivos afetados;
* testes (`TU-*`, `TI-*`, `E2E-*`).

Referencie o `techspec.md` em vez de repetir detalhes técnicos extensos dentro das tarefas.

---

## Princípios de decomposição

Prefira decompor a funcionalidade por **incrementos implementáveis e verificáveis**, e não apenas por camadas arquiteturais.

Evite, quando possível, decomposições como:

1. criar models;
2. criar repository;
3. criar service;
4. criar handler;
5. criar testes.

Esse tipo de divisão cria tarefas fortemente dependentes entre si e que frequentemente não entregam comportamento verificável isoladamente.

Prefira tarefas orientadas a capacidades ou fluxos completos.

Exemplo:

1. implementar domínio e persistência necessários para criação de conta;
2. implementar fluxo de criação de conta e contrato HTTP;
3. implementar idempotência e tratamento de conflitos;
4. implementar integração necessária ao fluxo;
5. concluir testes de integração e cenários E2E.

Uma tarefa pode atravessar mais de uma camada arquitetural quando isso for necessário para produzir uma entrega coesa.

Ao mesmo tempo, não crie tarefas excessivamente grandes.

Uma tarefa deve possuir um objetivo principal claramente identificável.

Não agrupe trabalhos não relacionados apenas para reduzir a quantidade total de tarefas.

Como orientação, funcionalidades comuns normalmente resultam em aproximadamente 3 a 10 tarefas, mas não trate esse intervalo como limite obrigatório.

---

## Fluxo

### 1. Analisar

Leia integralmente:

```text
./AGENTS.md
```

todas as rules aplicáveis em:

```text
./.agents/rules/
```

o PRD:

```text
./tasks/prd-[slug]/prd.md
```

e a TechSpec:

```text
./tasks/prd-[slug]/techspec.md
```

Identifique também somente as skills relevantes em:

```text
./.agents/skills/
```

Durante a análise, inventarie:

* requisitos funcionais (`RF-*`);
* critérios de aceitação (`CA-*`);
* componentes novos ou modificados;
* contratos;
* modelos de dados;
* endpoints;
* eventos;
* integrações;
* alterações de persistência;
* decisões técnicas;
* dependências;
* riscos que afetem implementação;
* arquivos relevantes;
* casos de teste de unidade (`TU-*`);
* casos de teste de integração (`TI-*`);
* casos de teste E2E (`E2E-*`).

Identifique também relações importantes como:

```text
RF-* → CA-*
CA-* → TU-*
CA-* → TI-*
CA-* → E2E-*
```

Não invente novos critérios de aceitação durante esta etapa.

Não altere os casos de teste definidos pela TechSpec sem uma inconsistência explícita que precise ser registrada.

**Conclua quando:** todos os requisitos, critérios de aceitação, componentes, dependências e casos de teste relevantes estiverem inventariados.

---

### 2. Definir dependências

Antes de criar as tarefas, determine as dependências entre as entregas.

Classifique cada relação como:

```text
depende de
bloqueia
independente
```

Liste dependências estruturais antes de tarefas que dependem delas.

Exemplos:

```text
persistência → caso de uso
caso de uso → transport HTTP
producer → consumer dependente do contrato
backend → frontend dependente da API
backend + frontend → E2E
```

Não imponha uma ordem artificial quando tarefas puderem ser executadas independentemente.

Quando possível, identifique tarefas que possam ser executadas em paralelo.

**Conclua quando:** for possível ordenar as tarefas sem violar dependências técnicas.

---

### 3. Propor a estrutura

Monte uma lista de tarefas de alto nível.

Cada tarefa deve possuir:

* ID;
* título;
* objetivo principal;
* dependências;
* requisitos ou critérios principais relacionados.

Exemplo:

```text
1.0 Implementar criação persistente de conta
2.0 Expor fluxo de criação via API
3.0 Implementar idempotência e conflitos
4.0 Integrar publicação de eventos
5.0 Concluir integração e cenários E2E
```

Prefira a menor quantidade de tarefas que preserve:

* coesão;
* clareza;
* verificabilidade;
* isolamento razoável;
* dependências compreensíveis.

Apresente a estrutura ao usuário para aprovação quando:

* houver mais de uma decomposição razoável;
* houver trade-offs relevantes na ordem de implementação;
* alguma tarefa puder ser dividida de maneiras significativamente diferentes;
* o usuário tiver solicitado revisão ou aprovação antes da geração.

Quando a decomposição for inequívoca a partir do PRD e da TechSpec, prossiga diretamente, salvo instrução explícita em contrário.

**Conclua quando:** a decomposição estiver definida e, quando necessário, aprovada pelo usuário.

---

### 4. Gerar `tasks.md`

Crie:

```text
./tasks/prd-[slug]/tasks.md
```

seguindo:

```text
./references/TEMPLATE_TASKS.md
```

O arquivo deve funcionar como índice central do plano de implementação.

Ele deve permitir identificar rapidamente:

* todas as tarefas;
* ordem sugerida;
* dependências;
* tarefas paralelizáveis;
* requisitos cobertos;
* critérios de aceitação cobertos;
* casos de teste associados.

Garanta rastreabilidade mínima:

```text
RF-* → task
CA-* → task
TU-* → task
TI-* → task
E2E-* → task
```

Um requisito ou critério pode aparecer em mais de uma tarefa quando necessário.

Um caso de teste deve ser associado à tarefa responsável por implementá-lo ou torná-lo executável.

---

### 5. Gerar tarefas individuais

Para cada tarefa, crie:

```text
task_1.md
task_2.md
task_3.md
...
```

usando números sequenciais a partir de `1`.

Siga:

```text
./references/TEMPLATE_TASK.md
```

Cada arquivo deve conter pelo menos:

* visão geral;
* skills aplicáveis;
* rules relevantes;
* requisitos relacionados;
* dependências;
* subtarefas;
* referências à TechSpec;
* critérios de aceitação relacionados;
* testes;
* arquivos relevantes;
* definição de concluído.

Use subtarefas no formato:

```text
1.1
1.2
1.3
```

para `task_1.md`.

Para `task_2.md`:

```text
2.1
2.2
2.3
```

e assim sucessivamente.

---

## Regras para subtarefas

Uma subtarefa deve representar uma ação concreta de implementação.

Prefira verbos objetivos:

```text
Adicionar...
Criar...
Modificar...
Persistir...
Validar...
Publicar...
Consumir...
Registrar...
Cobrir...
```

Evite subtarefas vagas como:

```text
Ajustar backend
Implementar lógica
Fazer integração
Corrigir testes
Finalizar feature
```

As subtarefas devem ser suficientes para orientar a implementação, mas não devem copiar a TechSpec.

Não transforme cada linha de código esperada em uma subtarefa.

---

## Dependências das tarefas

Cada tarefa deve informar explicitamente:

```text
Depende de
Bloqueia
```

Quando não houver dependência:

```text
Depende de: nenhuma
```

Quando não bloquear outras tarefas:

```text
Bloqueia: nenhuma
```

Não crie dependências apenas por ordem numérica.

A numeração representa a ordem recomendada, mas a dependência deve refletir uma necessidade técnica real.

---

## Requisitos e critérios de aceitação

Liste somente IDs existentes no PRD.

Exemplo:

```text
RF-01
RF-03

CA-01
CA-02
```

Não copie suas descrições integralmente quando a referência for suficiente.

Não crie novos `RF-*` ou `CA-*` dentro de uma task.

Quando um requisito não possuir critério de aceitação associado e isso representar uma inconsistência relevante, registre a lacuna em vez de inventar um critério.

---

## Testes

Use exclusivamente os casos de teste definidos na TechSpec como fonte de verdade.

Os identificadores válidos são:

```text
TU-*
TI-*
E2E-*
```

Associe cada teste à tarefa responsável por sua implementação ou habilitação.

Não transforme automaticamente cada tarefa em um teste E2E.

Não duplique um teste em várias tarefas sem necessidade concreta.

Se um teste depender de múltiplas tarefas, associe-o preferencialmente à última tarefa necessária para torná-lo executável.

Garanta que todos os casos de teste definidos na TechSpec estejam mapeados em pelo menos uma tarefa.

---

## Arquivos relevantes

Use como base a seção de arquivos relevantes da TechSpec e a exploração existente do projeto.

Para cada arquivo, registre quando possível:

```text
arquivo
ação
motivo
```

As ações válidas são:

```text
criar
modificar
remover
consultar
```

Não invente caminhos sem evidência suficiente.

---

## Definição de concluído

Cada tarefa deve possuir uma definição objetiva de conclusão.

No mínimo, considere:

* todas as subtarefas concluídas;
* testes associados implementados;
* testes associados passando;
* critérios relacionados atendidos;
* build passando;
* lint e checks obrigatórios passando;
* nenhum desvio não documentado da TechSpec;
* rules e skills aplicáveis respeitadas.

Adapte a definição aos comandos e padrões existentes no projeto.

Não invente comandos de build, lint ou teste que não existam no projeto.

---

### 6. Validar rastreabilidade

Antes de finalizar, valide o conjunto completo.

Confirme que:

* todo `RF-*` relevante está coberto por uma ou mais tarefas;
* todo `CA-*` está coberto por uma ou mais tarefas;
* todo `TU-*` está associado a uma tarefa;
* todo `TI-*` está associado a uma tarefa;
* todo `E2E-*` está associado a uma tarefa;
* todas as dependências são coerentes;
* não existem dependências circulares;
* nenhuma tarefa depende implicitamente de trabalho ausente;
* itens fora do escopo do PRD não foram adicionados;
* nenhuma decisão técnica da TechSpec foi silenciosamente alterada;
* as tarefas representam entregas implementáveis;
* as subtarefas são concretas;
* nenhuma task é apenas uma divisão artificial por camada.

Quando houver inconsistência entre PRD e TechSpec, não tente corrigi-la silenciosamente nas tarefas.

Registre o problema e interrompa a decomposição da parte afetada quando a inconsistência impedir uma decisão segura.

**Conclua quando:** cada requisito, critério de aceitação e caso de teste aplicável estiver rastreado e o grafo de dependências estiver consistente.

---

### 7. Reportar

Apresente:

* caminho do `tasks.md`;
* arquivos `task_[num].md` criados;
* quantidade total de tarefas;
* ordem sugerida;
* tarefas que podem ser executadas em paralelo;
* eventuais premissas ou inconsistências identificadas.

Não inicie automaticamente a implementação das tarefas.

Quando o usuário solicitar a implementação posteriormente, utilize os arquivos gerados como fonte de verdade.

Fluxo esperado:

```text
ideia
  ↓
criar-prd
  ↓
prd.md
  ↓
criar-techspec
  ↓
techspec.md
  ↓
criar-tasks
  ↓
tasks.md
  ├── task_1.md
  ├── task_2.md
  └── ...
  ↓
implementação
```
