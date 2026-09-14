---
name: executar-task
description: Tarefa — identifique e implemente a próxima tarefa executável de uma funcionalidade a partir do PRD, da TechSpec e do tasks.md, respeitando dependências, executando as validações aplicáveis e marcando a tarefa como concluída somente quando todos os critérios de conclusão forem atendidos. Use quando o usuário pedir para executar, implementar ou começar uma tarefa/subtarefa, ou dar continuidade à implementação de uma funcionalidade. Não use para revisar uma implementação concluída (executar-review) nem para realizar validação de QA independente (executar-qa).
argument-hint: --prd nome-da-funcionalidade [--task numero-da-tarefa]
---

O argumento `--prd` identifica o slug da funcionalidade.

Exemplo:

```text
/executar-task --prd create-account
```

Quando `--task` for informado, execute especificamente a tarefa solicitada.

Exemplo:

```text
/executar-task --prd create-account --task 3
```

Sem `--task`, selecione automaticamente a próxima tarefa executável.

Sem `--prd`, localize a pasta correspondente em:

```text
./tasks/prd-*/
```

Os arquivos obrigatórios em:

```text
./tasks/prd-[slug]/
```

são:

```text
prd.md
techspec.md
tasks.md
```

Se o PRD estiver ausente, interrompa e indique:

```text
/criar-prd
```

Se a TechSpec estiver ausente, interrompa e indique:

```text
/criar-techspec
```

Se o plano de tarefas estiver ausente, interrompa e indique:

```text
/criar-tasks
```

Cada tarefa é uma **entrega incremental, implementável e verificável**, com dependências explícitas, subtarefas, critérios de aceitação relacionados e testes próprios.

Use:

* `prd.md` como fonte de verdade para requisitos, critérios de aceitação, escopo e itens fora do escopo;
* `techspec.md` como fonte de verdade para arquitetura, contratos, persistência, integrações, decisões técnicas e estratégia de testes;
* `tasks.md` como fonte de verdade para ordem, status e dependências entre tarefas;
* `task_[num].md` como definição operacional da tarefa atual.

Referencie o `techspec.md` em vez de repetir detalhes técnicos extensos.

Não altere silenciosamente requisitos, critérios de aceitação ou decisões técnicas definidas nos documentos anteriores.

---

## Princípios de execução

Implemente somente o necessário para concluir a tarefa atual.

Não antecipe tarefas futuras, exceto quando uma pequena alteração compartilhada for tecnicamente necessária para concluir corretamente a tarefa atual.

Quando isso ocorrer:

* mantenha a alteração mínima;
* registre o motivo;
* não marque a tarefa futura como concluída;
* não implemente funcionalidades adicionais além do necessário.

Não refatore áreas não relacionadas apenas porque foram encontradas durante a implementação.

Não introduza novos frameworks, padrões arquiteturais ou abstrações quando a TechSpec e o projeto já definirem uma abordagem suficiente.

Prefira sempre:

1. padrões existentes do projeto;
2. abstrações já utilizadas;
3. bibliotecas já adotadas;
4. comandos já definidos para build, lint e testes.

---

## Fluxo

### 1. Selecionar a tarefa

Leia integralmente:

```text
./tasks/prd-[slug]/tasks.md
```

Se `--task` tiver sido informado:

1. localize a tarefa correspondente;
2. confirme que ela ainda não está concluída;
3. valide suas dependências antes de executá-la.

Se `--task` não tiver sido informado:

1. percorra as tarefas na ordem recomendada;
2. ignore tarefas já concluídas;
3. selecione a primeira tarefa cujas dependências obrigatórias estejam concluídas.

Não selecione simplesmente a primeira tarefa não concluída se ela estiver bloqueada.

Se nenhuma tarefa estiver desbloqueada, informe quais dependências impedem o avanço.

Abra integralmente:

```text
./tasks/prd-[slug]/task_[num].md
```

Identifique:

* objetivo;
* requisitos relacionados;
* dependências;
* subtarefas;
* critérios de aceitação;
* testes;
* arquivos relevantes;
* definição de concluído;
* skills aplicáveis;
* rules relevantes.

Confirme também relações como:

```text
RF-* → tarefa
CA-* → tarefa
TU-* → tarefa
TI-* → tarefa
E2E-* → tarefa
```

Não execute uma tarefa com dependências obrigatórias pendentes.

**Conclua quando:** uma tarefa executável estiver selecionada e todo o seu escopo, dependências, subtarefas e critérios de conclusão estiverem identificados.

---

### 2. Preparar o contexto

Leia integralmente:

```text
./AGENTS.md
```

e todas as rules aplicáveis em:

```text
./.agents/rules/
```

Carregue somente as skills relevantes em:

```text
./.agents/skills/
```

Revise no PRD apenas as seções relacionadas à tarefa atual.

Revise na TechSpec apenas as seções necessárias para compreender:

* arquitetura envolvida;
* componentes afetados;
* contratos;
* persistência;
* integrações;
* eventos;
* erros;
* idempotência;
* concorrência;
* observabilidade;
* testes;
* arquivos relevantes.

Não replique toda a análise da TechSpec se a tarefa já apontar claramente as seções pertinentes.

---

### 3. Validar o estado atual do projeto

Antes de modificar código, confirme que os elementos referenciados pela tarefa ainda correspondem ao projeto atual.

Verifique:

* arquivos citados;
* packages;
* interfaces;
* structs/classes;
* funções ou métodos;
* schemas;
* migrations;
* endpoints;
* eventos;
* dependências;
* configurações;
* testes existentes.

Se a estrutura atual divergir significativamente da TechSpec ou da tarefa:

* não altere silenciosamente a arquitetura;
* identifique a divergência;
* determine se ela pode ser tratada sem mudar o escopo;
* siga as rules do projeto para decidir se a implementação pode prosseguir.

Se a divergência tornar a tarefa inválida ou exigir uma nova decisão arquitetural relevante, interrompa a parte afetada e reporte o bloqueio.

Pequenas diferenças de nomes, localização de arquivos ou detalhes já evoluídos no código podem ser adaptadas quando não alterarem o comportamento ou as decisões técnicas fundamentais.

**Conclua quando:** a tarefa estiver compatível com o estado atual do projeto ou todas as divergências relevantes estiverem tratadas.

---

### 4. Planejar a implementação imediata

Antes de editar arquivos, determine a menor sequência de alterações necessária para concluir a tarefa.

Organize mentalmente a execução por subtarefa.

Exemplo:

```text
2.1 adicionar contrato
    ↓
2.2 implementar caso de uso
    ↓
2.3 integrar adapter
    ↓
2.4 adicionar testes
```

Não transforme essa etapa em um novo documento de planejamento.

Use a TechSpec e o `task_[num].md` como plano oficial.

Se a abordagem já estiver clara, passe diretamente à implementação.

Não peça confirmação ao usuário para decisões técnicas já resolvidas pelo PRD, TechSpec, AGENTS.md, rules ou código existente.

---

### 5. Preparar ambiente de execução

Quando a tarefa exigir apenas alteração estática de código, não inicie serviços desnecessariamente.

Quando for necessário executar:

* aplicação;
* backend;
* frontend;
* banco de dados;
* broker;
* cache;
* serviços auxiliares;

siga as regras de isolamento definidas no `AGENTS.md` e nas rules do projeto.

Quando o projeto não definir uma convenção específica, garanta que:

* processos iniciados por esta execução sejam identificáveis;
* serviços necessários não conflitem com processos existentes;
* URLs entre serviços apontem para a instância correta;
* recursos compartilhados não afetem outra worktree;
* somente os serviços necessários sejam iniciados.

Antes de iniciar um serviço em uma porta:

1. verifique se a porta está disponível;
2. não encerre automaticamente o processo que já a utiliza;
3. escolha outra porta quando permitido;
4. registre internamente os processos iniciados por esta execução.

Não altere configurações permanentes do usuário apenas para executar a tarefa.

Não encerre processos pertencentes ao usuário ou a outra worktree.

**Conclua quando:** todo ambiente necessário para implementar e validar a tarefa estiver disponível e isolado de forma segura.

---

### 6. Implementar

Implemente as subtarefas na ordem definida, respeitando dependências internas.

Para cada subtarefa:

1. identifique os arquivos afetados;
2. aplique somente as alterações necessárias;
3. preserve padrões existentes;
4. mantenha compatibilidade com a TechSpec;
5. adicione ou ajuste testes correspondentes quando fizer parte da subtarefa.

Não marque a subtarefa como concluída imediatamente após editar o código.

Uma subtarefa só deve ser considerada concluída quando sua implementação estiver funcional dentro do escopo da tarefa.

Quando encontrar código pré-existente defeituoso que impeça a tarefa:

* corrija-o somente se a correção estiver diretamente relacionada ao escopo;
* caso contrário, reporte o bloqueio;
* não transforme a execução em uma refatoração ampla.

---

## Regras de implementação

### Escopo

Não implemente requisitos que não estejam relacionados à tarefa atual.

Não adicione funcionalidades “úteis para o futuro” sem necessidade atual.

Não altere comportamento fora do escopo definido pelo PRD.

### Arquitetura

Siga a TechSpec.

Quando o código real exigir uma pequena adaptação que não altere a decisão arquitetural, ajuste de forma compatível.

Quando for necessária uma mudança arquitetural significativa, não faça a alteração silenciosamente.

### Dependências

Não adicione dependências externas quando uma solução adequada já existir no projeto.

Quando uma nova dependência for necessária:

* confirme que a TechSpec permite ou exige essa abordagem;
* use versão compatível com o projeto;
* siga as rules aplicáveis.

### Persistência

Quando houver alterações de banco:

* preserve migrations existentes;
* não edite migrations já aplicadas se as rules proibirem;
* mantenha constraints e índices definidos na TechSpec;
* considere atomicidade e rollback.

### Concorrência e idempotência

Quando aplicáveis à tarefa, preserve:

* garantias de idempotência;
* controle de concorrência;
* limites transacionais;
* semântica de retry;
* consistência definida na TechSpec.

Não simplifique esses mecanismos apenas para fazer testes passarem.

### Eventos e mensageria

Quando houver eventos:

* preserve contrato;
* producer;
* consumer;
* tópico/fila;
* semântica de entrega;
* retry;
* dead-letter;
* idempotência;
* ordenação;

conforme definido na TechSpec.

### Erros

Siga o padrão de erro já existente no projeto.

Não crie um segundo padrão de erros para a nova funcionalidade.

### Observabilidade

Implemente somente logs, métricas, traces ou health checks previstos pela tarefa, pela TechSpec ou pelos padrões do projeto.

Não introduza uma nova stack de observabilidade.

---

### 7. Executar validações

Após implementar todas as subtarefas, execute as validações aplicáveis definidas em:

1. `AGENTS.md`;
2. `.agents/rules/`;
3. `task_[num].md`;
4. configuração do projeto.

Considere, quando aplicável:

* formatação;
* geração de código;
* lint;
* análise estática;
* build;
* testes de unidade;
* testes de integração;
* testes E2E.

Use somente comandos existentes ou claramente suportados pelo projeto.

Não invente comandos de build ou teste.

---

## Testes da tarefa

Use os casos definidos no `task_[num].md`.

Os identificadores podem incluir:

```text
TU-*
TI-*
E2E-*
```

Execute todos os casos aplicáveis à tarefa atual.

Não considere um teste concluído apenas porque código de teste foi escrito.

O teste deve efetivamente passar.

Se um teste não puder ser executado por uma limitação legítima de ambiente:

* não marque o teste como `[x]`;
* registre o motivo;
* não marque a tarefa como concluída se esse teste fizer parte obrigatória da definição de concluído.

Se uma camada estiver explicitamente marcada como não aplicável na TechSpec ou na task, não tente inventar um teste para ela.

---

### 8. Validar definição de concluído

Antes de marcar qualquer tarefa como concluída, verifique integralmente a seção de definição de concluído do `task_[num].md`.

No mínimo, confirme:

* todas as subtarefas foram implementadas;
* todos os testes obrigatórios foram executados;
* todos os testes obrigatórios passaram;
* critérios de aceitação relacionados foram atendidos dentro do escopo da tarefa;
* build passou, quando aplicável;
* lint/checks passaram, quando aplicável;
* rules aplicáveis foram respeitadas;
* skills aplicáveis foram respeitadas;
* nenhum desvio não documentado da TechSpec foi introduzido;
* nenhuma funcionalidade fora do escopo foi adicionada.

Não marque a tarefa como concluída enquanto qualquer item obrigatório estiver pendente.

---

### 9. Atualizar os arquivos da tarefa

Somente após todas as validações obrigatórias passarem:

No arquivo:

```text
task_[num].md
```

marque como concluídas:

```text
[x]
```

as subtarefas efetivamente concluídas.

Marque também como `[x]` somente os testes efetivamente executados e aprovados.

Não marque como concluído:

* teste não executado;
* teste com falha;
* subtarefa parcialmente implementada;
* item bloqueado;
* validação ignorada.

Depois, em:

```text
tasks.md
```

marque a tarefa principal como:

```text
[x]
```

somente quando sua definição de concluído estiver integralmente atendida.

Se houver falha ou bloqueio:

* mantenha itens pendentes como `[ ]`;
* preserve a tarefa principal como não concluída;
* informe qual validação falhou;
* informe o motivo do bloqueio;
* não masque o problema alterando testes ou requisitos.

---

### 10. Limpar o ambiente

Desligue todos os serviços e processos iniciados especificamente por esta execução.

Encerre-os de forma graciosa quando possível.

Confirme que:

* processos iniciados por esta execução foram encerrados;
* portas utilizadas foram liberadas;
* containers temporários foram tratados conforme as rules;
* arquivos temporários foram removidos quando apropriado.

Não encerre:

* processos do usuário;
* serviços iniciados antes desta execução;
* processos pertencentes a outra worktree;
* recursos compartilhados que não sejam de responsabilidade desta execução.

Faça essa limpeza também quando:

* a implementação falhar;
* um teste falhar;
* a execução ficar bloqueada;
* a tarefa não puder ser concluída.

---

### 11. Reportar

Ao concluir com sucesso, informe de forma objetiva:

* tarefa executada;
* principais alterações realizadas;
* testes e validações executados;
* arquivos de controle atualizados;
* próxima tarefa desbloqueada, quando existir.

Exemplo conceitual:

```text
Tarefa 2.0 concluída — fluxo de criação de conta implementado,
TU-03 e TI-02 passando, task_2.md e tasks.md atualizados.
Próxima tarefa desbloqueada: 3.0.
```

Quando a tarefa não puder ser concluída, informe:

* tarefa parcialmente executada;
* o que foi concluído;
* o que permanece pendente;
* teste ou validação que falhou;
* bloqueio encontrado.

Não declare a tarefa como concluída quando os arquivos de controle continuarem com itens pendentes.

---

## Regras de status

Use os estados representados pelos checkboxes de forma estrita:

```text
[ ] pendente
[x] concluído e validado
```

Não use `[x]` como sinônimo de:

* código escrito;
* implementação parcial;
* teste criado;
* “parece funcionar”;
* validação ainda pendente.

Uma tarefa concluída significa:

```text
implementada
    +
validada
    +
testada
    +
documentos de controle atualizados
```

---

## Ordem de autoridade

Quando houver conflito de orientação, considere nesta ordem:

1. requisitos explícitos do usuário para a execução atual;
2. `AGENTS.md`;
3. rules aplicáveis em `.agents/rules/`;
4. PRD para requisitos e comportamento de produto;
5. TechSpec para decisões técnicas;
6. `tasks.md` para ordem e dependências;
7. `task_[num].md` para escopo operacional da tarefa;
8. skills aplicáveis;
9. padrões existentes no código.

Não use essa ordem para ignorar inconsistências relevantes.

Quando duas fontes obrigatórias se contradisserem de forma material, registre o conflito em vez de escolher silenciosamente uma delas.

---

## Resultado esperado

O fluxo esperado é:

```text
prd.md
   ↓
techspec.md
   ↓
tasks.md
   ↓
selecionar tarefa desbloqueada
   ↓
task_[num].md
   ↓
preparar contexto
   ↓
implementar subtarefas
   ↓
executar testes e validações
   ↓
validar definição de concluído
   ↓
atualizar task_[num].md
   ↓
atualizar tasks.md
   ↓
limpar ambiente
   ↓
reportar
```

A execução de uma tarefa termina somente quando a tarefa estiver realmente implementada e validada, ou quando um bloqueio concreto impedir sua conclusão.
