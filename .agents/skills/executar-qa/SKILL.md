---
name: executar-qa
description: "QA — valide e estabilize uma funcionalidade implementada contra o PRD, a TechSpec e as tarefas: critérios de aceitação, testes de unidade, integração e E2E, acessibilidade, responsividade, correção de defeitos, regressão e relatório final com evidências. Use quando o usuário pedir para executar QA ou validar uma funcionalidade implementada. Não use para implementar novas funcionalidades ou tarefas (executar-task) nem para realizar revisão de código (executar-review)."
argument-hint: --prd nome-da-funcionalidade
---

O argumento `--prd` identifica o slug da funcionalidade.

Exemplo:

```text
/executar-qa --prd create-account
```

Sem argumento, localize a pasta correspondente em:

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

Leia também todos os arquivos:

```text
task_[num].md
```

relacionados à funcionalidade.

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

O QA valida o comportamento entregue contra as fontes de verdade existentes.

Use:

* `prd.md` como fonte de verdade para requisitos, critérios de aceitação, escopo e comportamento esperado;
* `techspec.md` como fonte de verdade para contratos, arquitetura, integrações e casos de teste;
* `tasks.md` e `task_[num].md` como registro do que deveria ter sido implementado;
* `qa.md` como registro do processo de validação, defeitos, correções, regressões e resultado final.

O QA não redefine requisitos.

O QA não altera silenciosamente PRD, TechSpec ou critérios de aceitação para acomodar o comportamento atual da aplicação.

---

## Estados possíveis

O resultado final do QA deve ser exatamente um destes estados:

```text
APROVADO
REPROVADO
BLOQUEADO
```

### APROVADO

Use quando:

* todos os critérios de aceitação aplicáveis estiverem verificados;
* todos os critérios aplicáveis estiverem atendidos;
* todos os testes obrigatórios aplicáveis tiverem passado;
* não houver defeito aberto que viole o PRD ou a TechSpec;
* nenhuma validação obrigatória estiver pendente.

### REPROVADO

Use quando:

* pelo menos um critério de aceitação falhar;
* existir defeito conhecido que viole comportamento obrigatório;
* um teste obrigatório falhar após investigação;
* a implementação não atender ao PRD ou à TechSpec.

### BLOQUEADO

Use quando o QA não puder chegar a uma conclusão válida por fatores externos à implementação, como:

* dependência externa indisponível;
* credencial necessária ausente;
* ambiente indisponível;
* serviço obrigatório inacessível;
* decisão de produto ou arquitetura pendente;
* requisito contraditório que impeça determinar o comportamento correto.

Não use `BLOQUEADO` para mascarar uma falha funcional conhecida.

Se o comportamento estiver comprovadamente incorreto, o resultado é `REPROVADO`.

---

## Critérios de severidade de bugs

Classifique defeitos usando:

### Crítica

Defeito que:

* causa perda ou corrupção de dados;
* compromete segurança de forma relevante;
* impede completamente um fluxo essencial;
* causa indisponibilidade grave da funcionalidade.

### Alta

Defeito que:

* viola requisito ou critério de aceitação importante;
* impede fluxo principal;
* não possui workaround aceitável;
* produz resultado funcionalmente incorreto relevante.

### Média

Defeito que:

* produz comportamento incorreto com impacto limitado;
* possui workaround razoável;
* afeta fluxo secundário;
* não impede o objetivo principal da funcionalidade.

### Baixa

Defeito que:

* possui impacto predominantemente visual, textual ou secundário;
* não impede o fluxo;
* não altera resultado funcional relevante.

A severidade não substitui o critério de aprovação.

Um bug de qualquer severidade que faça um `CA-*` falhar impede aprovação.

---

## Evidências

Mantenha todas as evidências em:

```text
./tasks/prd-[slug]/evidences/
```

Use nomes rastreáveis sempre que possível.

Exemplos:

```text
CA-01-success.png
CA-03-validation-error.png
E2E-02-final-state.png

BUG-01-before.png
BUG-01-after.png
```

Quando uma evidência visual não fizer sentido, registre outra evidência verificável no `qa.md`, como:

* saída de teste;
* status HTTP;
* payload;
* log relevante;
* métrica;
* resultado observado.

Não crie screenshots artificiais somente para ter uma evidência visual.

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
./tasks/prd-[slug]/prd.md
./tasks/prd-[slug]/techspec.md
./tasks/prd-[slug]/tasks.md
```

e todos os:

```text
./tasks/prd-[slug]/task_[num].md
```

relevantes.

Identifique:

* requisitos funcionais (`RF-*`);
* critérios de aceitação (`CA-*`);
* testes de unidade (`TU-*`);
* testes de integração (`TI-*`);
* testes E2E (`E2E-*`);
* fluxos principais;
* fluxos alternativos;
* estados de erro;
* integrações;
* requisitos de acessibilidade;
* requisitos de responsividade;
* requisitos de desempenho, quando aplicáveis;
* dependências externas.

Monte a rastreabilidade:

```text
CA-* → método de validação
CA-* → TU-*
CA-* → TI-*
CA-* → E2E-*
```

Nem todo critério de aceitação precisa possuir obrigatoriamente um teste automatizado.

Quando a TechSpec não definir um `TU-*`, `TI-*` ou `E2E-*` para determinado critério, identifique um método apropriado de validação:

* manual;
* visual;
* operacional;
* por API;
* por observação do estado persistido;
* por evidência equivalente.

Não invente novos casos de teste da TechSpec apenas para preencher uma associação ausente.

Se a ausência representar uma falha relevante na especificação, registre-a no `qa.md`.

**Conclua quando:** todo critério de aceitação possuir um método de verificação identificado e todos os casos de teste existentes na TechSpec estiverem mapeados.

---

### 2. Inicializar o relatório de QA

Antes de iniciar a execução dos testes, crie ou atualize:

```text
./tasks/prd-[slug]/qa.md
```

seguindo:

```text
./references/TEMPLATE.md
```

Preencha inicialmente:

* funcionalidade;
* data;
* estado inicial;
* ambiente;
* critérios de aceitação;
* testes planejados;
* eventuais bloqueios já conhecidos.

Durante toda a execução, atualize o `qa.md` progressivamente.

Não espere o fim do QA para registrar bugs e resultados importantes.

Isso permite preservar o estado mesmo se a execução for interrompida ou bloqueada.

---

### 3. Preparar o ambiente

Suba somente os serviços necessários para executar as validações.

Siga prioritariamente as regras de runtime, worktree, containers e portas definidas em:

1. `AGENTS.md`;
2. `.agents/rules/`.

Quando não houver convenção específica, garanta:

* isolamento da worktree;
* ausência de conflito com serviços existentes;
* identificação dos processos iniciados;
* configuração correta das URLs entre serviços;
* isolamento dos bancos e recursos temporários quando necessário.

Antes de utilizar uma porta:

1. verifique se está disponível;
2. não encerre automaticamente o processo que já a utiliza;
3. escolha outra porta quando permitido.

Registre no `qa.md`:

* serviços iniciados;
* URLs utilizadas;
* portas;
* bancos ou recursos temporários;
* ferramenta de navegador, quando aplicável.

Não encerre processos pertencentes ao usuário ou a outra worktree.

**Conclua quando:** os serviços necessários estiverem saudáveis e o ambiente estiver pronto para validação.

---

### 4. Executar validações automatizadas básicas

Antes dos fluxos manuais ou E2E, execute quando aplicável:

* build;
* lint;
* análise estática;
* testes rápidos obrigatórios;
* verificações definidas no projeto.

Use somente comandos definidos ou claramente suportados por:

* `AGENTS.md`;
* rules;
* configuração do projeto.

Não invente comandos.

Se uma validação básica falhar:

1. investigue a causa;
2. determine se a falha pertence à funcionalidade em QA;
3. registre o resultado no `qa.md`.

Uma falha relevante não deve ser ignorada apenas para prosseguir com E2E.

---

### 5. Executar testes de unidade e integração

Execute todos os casos aplicáveis definidos na TechSpec:

```text
TU-*
TI-*
```

Use os comandos definidos pelo projeto.

Para cada caso, registre:

* ID;
* resultado;
* comando utilizado;
* observação relevante.

Estados permitidos:

```text
PASSOU
FALHOU
NÃO APLICÁVEL
BLOQUEADO
```

Use `NÃO APLICÁVEL` somente quando a própria TechSpec ou o contexto da solução justificar a ausência daquela camada.

Use `BLOQUEADO` somente quando o teste deveria ser executável, mas uma condição externa impedir a execução.

Quando houver meta de cobertura definida em:

* `AGENTS.md`;
* rules;
* configuração do projeto;

verifique-a usando os mecanismos existentes da stack.

Não invente uma meta de cobertura.

**Conclua quando:** todos os `TU-*` e `TI-*` aplicáveis estiverem executados ou explicitamente bloqueados.

---

### 6. Testar fluxos E2E

Para cada critério de aceitação com fluxo de interface ou jornada completa, execute o caso `E2E-*` correspondente usando a ferramenta de navegador disponível no ambiente.

Pode ser utilizada, por exemplo:

* Playwright;
* ferramenta de browser disponível no agente;
* outra ferramenta equivalente permitida pelo projeto.

Durante cada fluxo, valide:

* estado inicial;
* ação do usuário;
* resultado observado;
* estado final;
* mensagens apresentadas;
* chamadas relevantes à API;
* efeitos persistidos quando necessário.

Quando houver comportamento inesperado, investigue antes de concluir que existe um bug.

Considere:

* console do navegador;
* requisições;
* respostas;
* logs do backend;
* estado persistido;
* integrações externas.

Para cada E2E:

1. execute o fluxo;
2. registre `PASSOU`, `FALHOU` ou `BLOQUEADO`;
3. capture evidência quando aplicável;
4. associe o resultado ao `CA-*` correspondente.

Não aprove um critério apenas porque a página carregou.

Valide o resultado observável definido pelo PRD.

**Conclua quando:** todos os fluxos E2E aplicáveis tiverem resultado registrado e evidência suficiente.

---

### 7. Validar critérios sem E2E

Para critérios que não dependam de interface ou fluxo E2E, utilize o método apropriado.

Exemplos:

* chamada direta à API;
* inspeção de resposta;
* validação de estado persistido;
* teste automatizado;
* verificação operacional;
* validação manual.

Registre no `qa.md`:

* critério;
* método utilizado;
* resultado;
* evidência.

Não crie fluxo de navegador quando ele não agregar valor à validação.

---

### 8. Verificar acessibilidade

Execute esta etapa somente para funcionalidades com interface de usuário.

Valide, quando aplicável:

* navegação por teclado;
* Tab;
* Enter;
* Esc;
* foco visível;
* ordem de foco coerente;
* rótulos descritivos;
* semântica dos elementos;
* associação entre labels e campos;
* texto alternativo de imagens relevantes;
* mensagens de erro acessíveis;
* contraste;
* tamanho e legibilidade de texto;
* estados de loading;
* estados disabled;
* feedback de ações.

Não declare conformidade formal com WCAG apenas com inspeção superficial.

Registre somente o que efetivamente foi verificado.

Para cada problema encontrado, registre um bug quando ele representar descumprimento do comportamento ou requisito esperado.

**Conclua quando:** todas as verificações aplicáveis tiverem resultado registrado.

---

### 9. Verificar visual e responsividade

Execute esta etapa somente quando houver interface visual.

Valide os principais estados definidos pelo produto, como:

* vazio;
* carregando;
* com dados;
* erro;
* sucesso;
* disabled;
* validação de formulário.

Valide os principais tamanhos ou breakpoints utilizados pelo projeto.

Não invente breakpoints quando o projeto já definir seus próprios.

Verifique:

* overflow;
* clipping;
* alinhamento;
* sobreposição;
* conteúdo inacessível;
* texto cortado;
* comportamento de componentes;
* legibilidade.

Capture evidências relevantes em:

```text
./tasks/prd-[slug]/evidences/
```

Registre inconsistências no `qa.md`.

**Conclua quando:** os estados e tamanhos relevantes estiverem verificados.

---

### 10. Registrar bugs

Todo comportamento que viole:

* PRD;
* critério de aceitação;
* TechSpec;
* contrato obrigatório;
* comportamento esperado definido pela funcionalidade;

deve ser registrado no `qa.md`.

Use IDs sequenciais:

```text
BUG-01
BUG-02
BUG-03
```

Para cada bug registre:

* descrição;
* severidade;
* critério afetado;
* comportamento esperado;
* comportamento observado;
* evidência;
* causa raiz, quando identificada;
* status.

Estados permitidos:

```text
Aberto
Em correção
Corrigido
Bloqueado
```

Não classifique como bug algo que seja apenas preferência pessoal não definida pelo produto, design ou projeto.

---

### 11. Corrigir bugs

Durante QA, corrija somente defeitos necessários para fazer a implementação existente atender ao PRD e à TechSpec.

Não use QA para:

* adicionar novos requisitos;
* implementar funcionalidades novas;
* expandir escopo;
* fazer refatorações não relacionadas;
* substituir arquitetura sem necessidade;
* alterar critérios de aceitação;
* modificar o PRD para acomodar o código atual.

Para cada bug corrigível dentro do escopo:

1. localize a causa raiz;
2. produza uma correção mínima e compatível com a arquitetura existente;
3. crie ou ajuste teste de regressão;
4. confirme que o teste falha sem a correção quando isso puder ser demonstrado de forma segura;
5. aplique a correção;
6. execute novamente o teste;
7. registre a alteração no `qa.md`.

Quando o bug puder ser coberto por teste de unidade ou integração, prefira um teste automatizado.

Quando o problema só puder ser validado adequadamente em E2E, registre o fluxo de regressão correspondente.

Se a correção exigir:

* novo requisito;
* mudança de escopo;
* alteração relevante de arquitetura;
* alteração de contrato não prevista;
* decisão de produto;

não faça a alteração silenciosamente.

Marque o bug como `Bloqueado` e registre a decisão necessária.

**Conclua quando:** cada bug possuir correção validada ou bloqueio explicitamente documentado.

---

### 12. Executar regressão

Após qualquer correção, execute não apenas o teste que detectou o bug, mas também os testes diretamente relacionados à área alterada.

Priorize:

* teste de regressão criado;
* `CA-*` afetado;
* testes da mesma unidade;
* integrações dependentes;
* fluxos E2E diretamente relacionados.

Não execute indiscriminadamente toda a suíte se o projeto possuir estratégia de testes mais adequada, exceto quando:

* `AGENTS.md` exigir;
* rules exigirem;
* a alteração possuir impacto amplo;
* for necessário para segurança da entrega.

Registre os resultados no `qa.md`.

---

### 13. Revalidar critérios de aceitação

Depois das correções, reavalie todos os `CA-*` afetados.

Para cada critério, determine:

```text
PASSOU
FALHOU
BLOQUEADO
```

Um critério só pode ser marcado `PASSOU` quando seu resultado esperado tiver sido efetivamente observado.

Não considere um `CA-*` aprovado apenas porque os testes relacionados passaram se o comportamento observável puder ser validado diretamente e estiver incorreto.

Se qualquer critério falhar novamente, retorne à investigação e correção.

**Conclua quando:** todo critério possuir status final.

---

### 14. Determinar resultado do QA

Determine o status final utilizando exclusivamente as regras definidas nesta skill.

#### APROVADO

Somente quando:

```text
todos os CA-* aplicáveis = PASSOU
+
todos os testes obrigatórios aplicáveis = PASSOU
+
nenhum bug impeditivo aberto
+
nenhuma validação obrigatória pendente
```

#### REPROVADO

Quando:

```text
algum CA-* = FALHOU
```

ou existir defeito aberto comprovadamente incompatível com o comportamento esperado.

#### BLOQUEADO

Quando não for possível determinar aprovação ou reprovação por uma condição externa ou decisão pendente.

Não declare `APROVADO COM RESSALVAS`.

Se existe ressalva que viola requisito obrigatório, o resultado é `REPROVADO`.

Se a ressalva não viola requisito e está fora do escopo, registre-a como observação sem alterar o status.

---

### 15. Finalizar o relatório

Atualize:

```text
./tasks/prd-[slug]/qa.md
```

usando:

```text
./references/TEMPLATE.md
```

O relatório final deve conter:

* status;
* ambiente utilizado;
* critérios verificados;
* método de validação;
* testes executados;
* cobertura, quando aplicável;
* acessibilidade, quando aplicável;
* validação visual/responsiva, quando aplicável;
* bugs;
* severidade;
* correções;
* testes de regressão;
* evidências;
* bloqueios;
* conclusão.

Não omita falhas que tenham sido corrigidas.

O relatório deve preservar histórico suficiente para mostrar:

```text
falha encontrada
        ↓
causa investigada
        ↓
correção aplicada
        ↓
regressão criada
        ↓
revalidação aprovada
```

---

### 16. Encerrar o ambiente

Desligue todos os serviços e processos iniciados especificamente por esta execução.

Encerre-os de forma graciosa quando possível.

Confirme que foram liberados:

* portas;
* bancos temporários;
* containers;
* processos auxiliares;
* demais recursos temporários.

Não encerre:

* processos pertencentes ao usuário;
* recursos de outra worktree;
* serviços compartilhados iniciados antes desta execução.

Execute essa limpeza também quando:

* QA for aprovado;
* QA for reprovado;
* QA ficar bloqueado;
* ocorrer falha;
* a execução for interrompida.

---

### 17. Reportar

Ao final, informe:

* status: `APROVADO`, `REPROVADO` ou `BLOQUEADO`;
* total de critérios;
* quantos passaram;
* bugs encontrados;
* bugs corrigidos;
* bugs bloqueados;
* testes executados;
* caminho do `qa.md`;
* diretório de evidências.

Quando aprovado, informe objetivamente que todos os critérios obrigatórios foram atendidos.

Quando reprovado, destaque quais critérios permanecem falhando.

Quando bloqueado, destaque qual condição impede concluir o QA.

---

## Regras de escopo

O QA pode:

```text
validar
investigar
reproduzir
corrigir bug
criar regressão
revalidar
documentar
```

O QA não deve:

```text
criar feature nova
expandir escopo
reescrever arquitetura
alterar requisito para fazer o teste passar
ignorar falha obrigatória
marcar validação não executada como aprovada
```

---

## Ordem de autoridade

Quando houver conflito, considere nesta ordem:

1. instrução explícita do usuário para a execução atual;
2. `AGENTS.md`;
3. rules em `.agents/rules/`;
4. PRD para comportamento e critérios de aceitação;
5. TechSpec para contratos e decisões técnicas;
6. tarefas para escopo implementado;
7. padrões existentes no projeto.

Quando fontes obrigatórias apresentarem contradição material, não escolha silenciosamente uma interpretação.

Registre a inconsistência e marque como `BLOQUEADO` quando ela impedir determinar o comportamento correto.

---

## Resultado esperado

```text
PRD + TechSpec + Tasks
          ↓
       analisar
          ↓
      criar qa.md
          ↓
 preparar ambiente
          ↓
 testes automatizados
          ↓
    E2E / manual
          ↓
a11y / responsividade
          ↓
    registrar bugs
          ↓
    corrigir bugs
          ↓
 criar regressões
          ↓
      revalidar
          ↓
 determinar status
          ↓
 finalizar qa.md
          ↓
 limpar ambiente
          ↓
      reportar
```

O QA termina somente quando houver evidência suficiente para classificar a funcionalidade como `APROVADO`, `REPROVADO` ou `BLOQUEADO`.
