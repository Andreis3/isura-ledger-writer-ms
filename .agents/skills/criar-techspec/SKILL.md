---
name: criar-techspec
description: TechSpec — especificação técnica derivada de um PRD existente. Use quando o usuário pedir uma TechSpec ou quiser definir a arquitetura e a estratégia técnica de uma funcionalidade que já possua um PRD. Não use sem PRD (criar-prd) nem para decompor a solução em tarefas de implementação (criar-tasks).
argument-hint: --prd nome-da-funcionalidade | --file "./caminho/prd.md" 
---

A TechSpec define como os requisitos do PRD serão atendidos tecnicamente: arquitetura, componentes, contratos, persistência, integrações, eventos, observabilidade, riscos e estratégia de testes.

O problema, os objetivos, as histórias de usuário, os critérios de aceitação e o escopo pertencem ao PRD. Referencie essas informações em vez de repeti-las desnecessariamente.

A TechSpec deve especificar a solução sem implementá-la.

Não inclua código de produção. São permitidos somente exemplos curtos necessários para documentar:

* interfaces;
* contratos;
* payloads;
* schemas;
* queries;
* protocolos;
* eventos;
* exemplos previstos pelo template.

Prefira sempre uma arquitetura simples, evolutiva e compatível com os padrões já existentes no projeto.

Não introduza novos frameworks, abstrações, bibliotecas ou padrões arquiteturais sem necessidade concreta. Quando uma nova abordagem for necessária, registre a justificativa, as alternativas consideradas e os trade-offs em **Principais decisões**.

## Entrada

A skill pode receber o PRD de duas formas.

### Por slug

```text
--prd nome-da-funcionalidade
```

O argumento identifica o slug da funcionalidade.

O PRD esperado é:

```text
./tasks/prd-[slug]/prd.md
```

Exemplo:

```text
/criar-techspec --prd create-journal-entry
```

resolve para:

```text
./tasks/prd-create-journal-entry/prd.md
```

### Por arquivo

```text
--file "./caminho/prd.md"
```

Quando `--file` for informado, use esse arquivo como PRD de entrada independentemente de sua localização.

Exemplo:

```text
/criar-techspec --file "./docs/prd/create-journal-entry.md"
```

### Sem argumento

Se nenhum argumento for informado, localize os PRDs disponíveis em:

```text
./tasks/prd-*/prd.md
```

Se existir apenas um candidato evidente, utilize-o.

Se houver múltiplos PRDs e não for possível determinar com segurança qual utilizar, solicite a escolha do usuário.

### Regras de entrada

Se `--prd` e `--file` forem fornecidos simultaneamente, `--file` é a fonte do documento e `--prd` pode ser usado como slug de identificação e destino.

O PRD é obrigatório.

Se o arquivo informado não existir ou nenhum PRD puder ser localizado, interrompa a criação da TechSpec e indique que primeiro deve ser criado um PRD usando:

```text
/criar-prd
```

Não produza uma TechSpec baseada apenas em suposições sobre os requisitos do produto.

---

## Fluxo de trabalho

### 1. Analisar o PRD

Leia o PRD integralmente antes de explorar ou propor qualquer solução técnica.

Extraia e registre internamente:

* histórias de usuário relevantes;
* requisitos funcionais (`RF-*`);
* critérios de aceitação (`CA-*`);
* restrições técnicas de alto nível;
* integrações obrigatórias;
* requisitos regulatórios ou de segurança;
* metas de desempenho e escala;
* requisitos de privacidade;
* dependências;
* itens fora do escopo;
* métricas de sucesso que impactem decisões técnicas.

Não altere silenciosamente requisitos definidos pelo PRD.

Não transforme detalhes técnicos existentes no PRD em decisões definitivas sem verificar sua compatibilidade com o projeto atual.

Quando houver inconsistências internas no PRD, registre a ambiguidade para esclarecimento.

**Conclua quando:** todos os requisitos, critérios de aceitação, restrições, dependências e métricas relevantes para a solução técnica estiverem identificados.

---

### 2. Explorar o projeto

Antes de fazer perguntas ao usuário, explore o projeto existente.

Leia integralmente:

```text
./AGENTS.md
```

e todas as regras aplicáveis em:

```text
./.agents/rules/
```

Use o agente `Explore`, quando disponível, para investigar o código existente.

Examine somente o necessário para compreender a funcionalidade e suas dependências.

Investigue:

* estrutura de módulos e packages;
* arquitetura existente;
* componentes relacionados;
* interfaces existentes;
* serviços;
* handlers/controllers;
* casos de uso;
* domínio;
* repositories;
* gateways/adapters;
* persistência;
* migrations;
* modelos de dados;
* eventos;
* filas ou tópicos;
* integrações externas;
* configuração;
* autenticação e autorização;
* tratamento de erros;
* estratégias de retry;
* idempotência;
* concorrência;
* observabilidade;
* testes existentes;
* infraestrutura relacionada.

Determine:

* quem chama o componente afetado;
* quem é chamado por ele;
* quais contratos já existem;
* quais abstrações podem ser reutilizadas;
* quais arquivos provavelmente serão criados ou modificados;
* quais padrões arquiteturais já são adotados;
* quais convenções de nomenclatura são utilizadas.

Priorize reutilizar:

1. abstrações existentes;
2. bibliotecas já utilizadas pelo projeto;
3. padrões arquiteturais existentes;
4. mecanismos existentes de configuração, logs, métricas, erros e testes.

Não introduza uma nova abstração apenas para tornar a arquitetura aparentemente mais genérica.

Uma nova abstração deve resolver um problema concreto da funcionalidade ou eliminar uma duplicação estrutural relevante.

Quando houver bibliotecas, APIs externas, protocolos ou regras de negócio que precisem ser confirmados, consulte documentação oficial ou fontes confiáveis.

Para regras de negócio específicas do domínio que possam ser determinadas por documentação pública, pesquise antes de perguntar ao usuário.

**Conclua quando:** for possível nomear cada componente novo ou modificado, explicar sua responsabilidade e indicar onde ele se encaixa no código atual.

---

### 3. Identificar lacunas técnicas

Compare o PRD com o projeto existente.

Identifique somente decisões técnicas que ainda não possam ser determinadas a partir de:

* PRD;
* código;
* `AGENTS.md`;
* `.agents/rules/`;
* documentação do projeto;
* documentação oficial de dependências;
* padrões já adotados.

Considere especialmente lacunas relacionadas a:

* limites do domínio;
* ownership de dados;
* fluxo de dados;
* contratos;
* comportamento em falhas;
* consistência;
* concorrência;
* idempotência;
* retries;
* timeouts;
* integrações externas;
* persistência;
* eventos;
* ordenação;
* garantias de entrega;
* requisitos de desempenho;
* cenários críticos de teste.

Não pergunte novamente algo que já esteja claramente definido nas fontes acima.

---

### 4. Esclarecer

Se ainda existirem lacunas relevantes após analisar o PRD e explorar o projeto, faça perguntas ao usuário usando `AskUserQuestion`.

Pergunte somente sobre decisões que realmente dependam do usuário ou representem escolhas de produto/arquitetura sem resposta evidente.

Priorize questões relacionadas a:

* limites do domínio;
* comportamento esperado em casos ambíguos;
* contratos que não estejam definidos;
* ownership de dados;
* dependências externas;
* modos de falha;
* timeouts;
* idempotência;
* consistência;
* interfaces principais;
* requisitos não especificados;
* cenários de teste críticos;
* trade-offs sem resposta evidente no projeto.

Evite perguntar ao usuário qual tecnologia utilizar quando o projeto já estabelecer uma escolha clara.

Se uma decisão técnica puder ser inferida com segurança a partir dos padrões existentes, adote-a e registre a justificativa na TechSpec.

Se uma informação não for crítica e não puder ser determinada, utilize uma premissa explícita e registre-a no documento.

**Conclua quando:** não existirem lacunas relevantes sem tratamento ou cada uma possuir uma resposta, decisão fundamentada ou premissa explícita.

---

### 5. Projetar a solução

Antes de redigir o documento final, consolide mentalmente a solução técnica.

Para cada requisito funcional relevante, determine:

* componente responsável;
* fluxo principal;
* dados utilizados;
* dependências;
* persistência;
* contratos envolvidos;
* comportamento de erro;
* observabilidade;
* estratégia de teste.

Verifique se todos os `CA-*` possuem uma forma técnica de validação.

Quando aplicável, determine também:

#### Concorrência e consistência

* operações concorrentes relevantes;
* condições de corrida possíveis;
* nível de consistência necessário;
* estratégia de controle de concorrência;
* atomicidade necessária;
* transações envolvidas.

#### Idempotência

* quais operações precisam ser idempotentes;
* escopo da chave de idempotência;
* comportamento em repetição;
* duração ou persistência da chave;
* interação com retries.

#### Eventos e mensageria

* eventos produzidos;
* eventos consumidos;
* producer;
* consumer;
* fila/tópico;
* formato do payload;
* garantias de entrega;
* retry;
* dead-letter;
* ordenação;
* deduplicação;
* idempotência do consumidor.

#### Integrações externas

* autenticação;
* timeout;
* retry;
* circuit breaking, quando aplicável;
* rate limiting;
* comportamento em indisponibilidade;
* degradação parcial;
* mapeamento de erros.

Evite complexidade preventiva para cenários sem evidência no PRD ou no projeto.

---

### 6. Redigir

Leia integralmente:

```text
./references/TEMPLATE.md
```

e siga sua estrutura exatamente.

Não remova seções estruturais do template.

Quando uma seção não for aplicável, registre:

```text
Não aplicável — [justificativa curta].
```

Não crie componentes, APIs, tabelas, eventos ou testes fictícios apenas para preencher uma seção.

Preencha o documento com informações específicas da funcionalidade e compatíveis com o código existente.

### Arquitetura

Para cada componente novo ou modificado, descreva:

* responsabilidade;
* localização provável;
* relacionamentos;
* dependências;
* fluxo de dados.

Evite abstrações genéricas sem responsabilidade concreta.

### Interfaces

Documente apenas interfaces relevantes para os limites entre componentes.

Mantenha exemplos curtos.

Não implemente métodos.

### Modelos de dados

Documente os contratos necessários para implementação:

* entidades;
* value objects relevantes;
* DTOs;
* payloads;
* schemas;
* modelos persistidos;
* erros;
* contratos externos.

Para contratos JSON entre backend e UI, produza exemplos prontos para consumo.

Quando o projeto utilizar `null` como normalização de campos ausentes, preserve essa convenção.

### APIs

Se houver endpoints HTTP ou RPC, documente cada operação relevante.

Inclua:

* método;
* rota;
* entrada;
* validações;
* respostas;
* erros;
* cenários alternativos;
* comportamentos não óbvios.

Não duplique payloads já documentados em **Modelos de dados**; referencie-os.

### Eventos e mensageria

Se a solução produzir ou consumir eventos, documente-os com o mesmo rigor utilizado para APIs.

Para cada evento relevante registre:

* nome;
* producer;
* consumer;
* tópico/fila;
* momento de publicação;
* payload;
* semântica de entrega;
* retry;
* dead-letter;
* idempotência;
* ordenação, quando relevante;
* compatibilidade/evolução de schema, quando relevante.

Se o template não possuir uma seção dedicada para eventos, documente essas informações em **Pontos de integração** ou na subseção técnica mais apropriada.

### Persistência

Quando houver mudanças em persistência, documente:

* entidades/tabelas afetadas;
* campos;
* constraints;
* índices relevantes;
* migrations;
* ownership dos dados;
* estratégia de atualização;
* necessidades transacionais.

Não inclua SQL de implementação além de pequenos exemplos necessários para documentar schemas ou constraints.

### Concorrência, consistência e idempotência

Quando aplicável, registre explicitamente:

* operações idempotentes;
* chave de idempotência;
* condições de corrida;
* estratégia de controle;
* limites transacionais;
* semântica de retry;
* consistência esperada.

Se não forem relevantes para a funcionalidade, não crie complexidade artificial.

### Abordagem de testes

Defina somente as camadas aplicáveis.

Use os identificadores:

```text
TU-*  → testes de unidade
TI-*  → testes de integração
E2E-* → testes end-to-end
```

Associe cada caso de teste aos critérios de aceitação (`CA-*`) que ele verifica.

Exemplo conceitual:

```text
CA-01
 ├── TU-01
 ├── TI-02
 └── E2E-01
```

Mocks devem ser utilizados somente quando necessários, priorizando-os para dependências externas ou limites que não devam participar daquele nível de teste.

Não crie testes E2E quando a funcionalidade não possuir um fluxo que justifique essa camada.

Quando houver meta de cobertura, utilize a definida em:

1. `AGENTS.md`;
2. `.agents/rules/`;
3. configuração existente do projeto.

Não invente uma meta de cobertura.

### Sequenciamento

A ordem de construção deve refletir dependências técnicas reais.

Prefira uma sequência que permita validar partes da solução progressivamente.

Exemplo geral:

```text
domínio/contratos
        ↓
persistência
        ↓
caso de uso
        ↓
integrações/adapters
        ↓
transport
        ↓
integração
        ↓
testes finais
```

Adapte a sequência à arquitetura existente.

### Observabilidade

Utilize a infraestrutura existente do projeto.

Defina somente sinais que sejam úteis operacionalmente.

Considere:

* logs;
* métricas;
* traces;
* health checks;
* eventos operacionais;
* correlação;
* latência;
* erros;
* retries;
* filas;
* dependências externas.

Não introduza uma nova stack de observabilidade sem justificativa.

### Principais decisões

Registre decisões técnicas relevantes utilizando:

* decisão;
* motivo;
* alternativas consideradas;
* trade-offs.

Não transforme decisões triviais em ADRs dentro da TechSpec.

### Riscos conhecidos

Liste riscos concretos da implementação.

Para cada risco relevante, indique:

* impacto;
* causa;
* mitigação;
* necessidade de pesquisa adicional, quando aplicável.

### Conformidade com AGENTS.md e rules

Confirme explicitamente a leitura de:

```text
AGENTS.md
.agents/rules/*
```

Registre somente regras relevantes para a especificação.

Se existir conflito entre a proposta e alguma regra, não ignore o conflito.

Registre:

* regra;
* impacto;
* decisão tomada;
* justificativa de eventual desvio.

### Conformidade com skills

Verifique somente skills aplicáveis localizadas em:

```text
.agents/skills/
```

Não liste skills sem relação com a funcionalidade.

Se houver alguma orientação aplicável que não possa ser seguida, registre o desvio e sua justificativa.

### Arquivos relevantes e dependentes

Liste os arquivos identificados durante a exploração.

Sempre que possível, utilize a estrutura:

| Arquivo              | Ação esperada                     | Motivo    |
| -------------------- | --------------------------------- | --------- |
| `caminho/do/arquivo` | criar/modificar/remover/consultar | descrição |

Utilize somente ações:

```text
criar
modificar
remover
consultar
```

Não invente caminhos sem evidência suficiente no projeto.

Se um arquivo ainda não existir mas sua localização puder ser determinada pelas convenções do projeto, identifique-o como `criar`.

**Conclua quando:** toda seção do template estiver preenchida, marcada explicitamente como não aplicável quando necessário, e cada componente identificado durante a exploração estiver especificado.

---

### 7. Validar a TechSpec

Antes de salvar, faça uma revisão completa.

Verifique:

#### Rastreabilidade

* Todo requisito funcional relevante do PRD possui tratamento técnico.
* Todo `CA-*` relevante possui ao menos uma estratégia de validação.
* Testes apontam para critérios de aceitação existentes.
* Não existem `CA-*`, `RF-*`, `TU-*`, `TI-*` ou `E2E-*` inventados sem necessidade.

#### Arquitetura

* A solução segue os padrões existentes.
* Cada novo componente possui responsabilidade clara.
* Não há abstrações desnecessárias.
* Dependências estão explícitas.
* Ownership de dados está claro quando relevante.

#### Contratos

* Entradas e saídas importantes estão documentadas.
* Erros relevantes estão documentados.
* Payloads são consistentes.
* APIs e eventos não possuem contratos contraditórios.

#### Persistência

* Alterações de dados estão documentadas.
* Constraints relevantes foram consideradas.
* Transações necessárias foram identificadas.
* Concorrência foi considerada quando necessária.

#### Integrações

* Timeouts foram considerados.
* Retries foram considerados.
* Idempotência foi considerada.
* Modos de falha foram documentados.

#### Operação

* Logs relevantes estão definidos.
* Métricas importantes estão previstas.
* Falhas observáveis possuem sinais operacionais.

#### Escopo

* A TechSpec não adiciona funcionalidades fora do PRD.
* Decisões técnicas não alteram silenciosamente comportamento de produto.
* Itens fora do escopo permanecem fora do escopo.

#### Implementação

* Não existe código de produção no documento.
* Exemplos estão limitados à documentação de contratos e interfaces.
* A TechSpec explica suficientemente o que deverá ser construído sem executar a implementação.

**Conclua quando:** a TechSpec estiver consistente, rastreável, implementável e alinhada ao PRD e ao projeto existente.

---

### 8. Salvar e reportar

Quando a entrada tiver sido resolvida por slug, grave o documento em:

```text
./tasks/prd-[slug]/techspec.md
```

Quando a entrada tiver sido fornecida via `--file`, determine o slug a partir, nesta ordem:

1. `--prd`, se também informado;
2. diretório `prd-[slug]`, se o arquivo estiver dentro desse padrão;
3. nome ou título da funcionalidade definido no PRD.

O destino padrão continua sendo:

```text
./tasks/prd-[slug]/techspec.md
```

Crie o diretório quando necessário.

Ao concluir, informe:

* caminho do arquivo criado;
* nome da funcionalidade;
* resumo de uma linha da abordagem técnica;
* principais componentes que serão criados ou modificados.

Não inicie a implementação.

A próxima etapa do fluxo é:

```text
/criar-tasks
```

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
tasks
```