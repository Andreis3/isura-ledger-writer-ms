---
name: criar-tasks
description: Tarefas — decomposição de uma funcionalidade em tarefas de implementação a partir do PRD e da TechSpec existentes em `tasks/prd-*/`. Use quando o usuário pedir para decompor uma funcionalidade em tarefas ou planejar sua execução. Não use para redigir o PRD (criar-prd) nem a TechSpec (criar-techspec).
argument-hint: --prd nome-da-funcionalidade
---

O argumento `--prd` identifica o slug da funcionalidade. Sem argumento, localize a pasta em `./tasks/prd-*/`. Os arquivos obrigatórios são `tasks/prd-[slug]/prd.md` e `tasks/prd-[slug]/techspec.md`; se algum estiver ausente, pare e indique a skill correspondente (`/criar-prd` ou `/criar-techspec`).

Cada tarefa é uma **entrega incremental**, com escopo claro, dependências explícitas e testes próprios. Referencie os critérios de aceitação do PRD e use os casos de teste definidos na TechSpec como fonte de verdade para os testes das tarefas. Referencie o `techspec.md` em vez de repetir detalhes de implementação.

## Fluxo

1. **Analisar** — leia o `AGENTS.md`, todas as rules em `.agents/rules/` e as skills em `.agents/skills/golang`, o PRD e a TechSpec; inventarie os requisitos, os critérios de aceitação (`CA-*`), as decisões técnicas, os componentes e todos os casos de teste definidos na TechSpec. Identifique as skills em `.agents/skills/` aplicáveis a cada tarefa.
   **Conclua quando:** requisitos e testes existentes estiverem inventariados, lacunas de cobertura estiverem identificadas e as skills aplicáveis tiverem sido reconhecidas; cobertura incompleta não impede planejar tarefas de resolução.

2. **Propor a estrutura** — monte uma lista de tarefas de alto nível, preferencialmente com no máximo 10 itens. Liste as dependências antes das tarefas que dependem delas, como backend antes de frontend quando o frontend depender dele, e ambos antes dos testes E2E. Mostre a lista ao usuário para aprovação antes de gerar qualquer arquivo.
   **Conclua quando:** o usuário aprovar a lista.

3. **Gerar os arquivos** — em `./tasks/prd-[slug]/`:
    - `tasks.md` seguindo `./references/TEMPLATE_TASKS.md` desta skill
    - um arquivo `task_[num].md` para cada tarefa, seguindo `./references/TEMPLATE_TASK.md` desta skill. Use números sequenciais a partir de 1 (`task_1.md`, `task_2.md`, …) e inclua subtarefas (`[num].1`, `[num].2`, …), referências aos critérios de aceitação (`CA-*`) e os testes correspondentes aos casos mapeados da TechSpec
      **Conclua quando:** cada critério e teste já definido estiver mapeado, lacunas estiverem explicitadas em tarefas preparatórias e tarefas dependentes estiverem marcadas como BLOQUEADAS até a resolução do gate.

4. **Reportar** — apresente os arquivos gerados, os gates pendentes, seus responsáveis quando conhecidos e quais tarefas podem ou não ser executadas. Aguarde a confirmação do usuário antes de iniciar qualquer implementação.


## Gate de cobertura de invariantes

Ao inventariar o PRD e a TechSpec, inclua `INV-*` além de `RF-*`, `CA-*` e `TU-*`/`TI-*`/`E2E-*`. Cada invariante afetada deve aparecer em pelo menos uma tarefa que implemente sua proteção e outra atividade de verificação (podem estar na mesma tarefa, mas com subtarefas distintas). Inclua testes de concorrência, persistência e falha para garantias financeiras; mocks isolados não demonstram atomicidade real. Se a TechSpec não cobrir uma invariante crítica, não a considere resolvida: identifique a lacuna, crie uma tarefa preparatória de correção/validação da TechSpec e bloqueie a implementação dependente até a cobertura ser aprovada. Não invente casos de teste ou decisões para preencher lacunas.


## Separação obrigatória: gate de planejamento vs. gate de implementação

A geração de tarefas é **planejamento**, não autorização para implementar. Um catálogo de produto ainda indefinido, aprovação de segurança pendente, contrato externo não aprovado ou caso de teste ausente **não bloqueiam automaticamente o planejamento completo**. Registre o bloqueio e as dependências sem preencher decisões por suposição.

Ao avaliar lacunas, classifique:

- **BLOQUEIO DE PLANEJAMENTO:** PRD/TechSpec ausente, ilegível ou contraditório a ponto de impedir identificar sequer entregas responsáveis e dependências. Pare e indique a origem do problema.
- **BLOQUEIO DE IMPLEMENTAÇÃO:** decisão de produto, aprovação de segurança, contrato, invariante ou teste necessário ainda não resolvido. **Gere as tarefas preparatórias e as demais tarefas planejáveis**, mas marque as tarefas dependentes como `BLOQUEADA`. Não autorize `/executar-task` nelas.

Para cada gate documente: identificador estável `GATE-*`, decisão/evidência pendente, origem no PRD/TechSpec, responsável ou `a definir`, critério objetivo para liberar, dependências e lista de tarefas bloqueadas. O gate só passa para `RESOLVIDO` após evidência da decisão/aprovação; a criação de uma tarefa preparatória não resolve o gate automaticamente.

Se faltar um teste especificado para `INV-*` ou `CA-*`, inclua tarefa preparatória **atualizar TechSpec e aprovar novo caso de teste**. Não invente um `TI-*` definitivo na task: use uma pendência identificada e deixe os testes dependentes bloqueados. A task implementadora só pode prosseguir depois que o teste estiver definido e rastreado na TechSpec.

Coloque tarefas de resolução de gates antes das tarefas dependentes. Quando tarefas independentes forem seguras e satisfizerem seus próprios gates, sinalize-as como `PRONTA`. O planejamento deve continuar sendo apresentado ao usuário para aprovação antes de gerar arquivos.

Estados padronizados no planejamento: `PRONTA` (sem gates pendentes), `BLOQUEADA` (dependência pendente), `CONCLUÍDA` (implementada e validada). Um status `PRONTA` não é aprovação para execução automática.
