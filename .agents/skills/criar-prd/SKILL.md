---
name: criar-prd
description: PRD — Documento de Requisitos de Produto. Use quando o usuário pedir um PRD ou quiser definir os requisitos e o escopo de uma nova funcionalidade ou produto (primeira etapa do fluxo PRD → TechSpec → tasks). Não use para especificações técnicas (criar-techspec) nem para decompor requisitos em tarefas (criar-tasks).
argument-hint: --prompt "descrição da funcionalidade" | --file "./caminho/ideia.md"
---

O PRD define o problema, os objetivos, os resultados esperados, as restrições e o escopo.
Objetivos e resultados devem ter critérios mensuráveis.

Os detalhes de implementação — como arquitetura, banco de dados, estruturas internas,
frameworks e código — pertencem à TechSpec e ficam fora do PRD.

## Entrada

A skill pode receber a ideia da funcionalidade de duas formas:

- `--prompt "descrição"` — descrição textual fornecida diretamente pelo usuário.
- `--file "./caminho/arquivo.md"` — arquivo Markdown contendo contexto, ideia,
  requisitos iniciais, regras de negócio, anotações ou decisões já tomadas.

Quando `--file` for informado:

1. Leia o arquivo integralmente antes de iniciar o PRD.
2. Considere o conteúdo do arquivo como a principal fonte de contexto.
3. Extraia dele:
   - Problema a ser resolvido
   - Usuários envolvidos
   - Objetivos
   - Regras de negócio
   - Funcionalidades desejadas
   - Restrições
   - Dependências
   - Itens fora do escopo
4. Não assuma que o arquivo está completo ou correto.
5. Identifique lacunas necessárias para preencher o PRD.
6. Não copie automaticamente detalhes técnicos para o PRD. Informações de
   implementação devem ser reservadas para a TechSpec.

Se `--prompt` e `--file` forem fornecidos juntos, use o arquivo como contexto base
e o prompt como instrução complementar ou refinamento.

## Fluxo de trabalho

### 1. Coletar contexto

Se `--file` tiver sido informado, leia o arquivo integralmente.

Se `--prompt` tiver sido informado, analise a descrição fornecida.

Consolide as informações conhecidas antes de fazer perguntas.

### 2. Esclarecer

Faça perguntas ao usuário usando `AskUserQuestion` somente para informações
necessárias que não possam ser determinadas a partir do prompt ou do arquivo.

Busque esclarecer:

- Problema a ser resolvido
- Metas mensuráveis
- Usuários principais e secundários
- Histórias de usuário
- Fluxos principais
- Funcionalidades centrais
- Entradas, saídas e ações
- Itens fora do escopo
- Dependências
- Diretrizes de UI/UX e acessibilidade

Não pergunte novamente algo que já esteja claramente definido no arquivo de entrada.

Para regras de negócio específicas do domínio que possam ser verificadas em fontes
externas, pesquise na web em vez de perguntar ao usuário.

**Conclua quando:** cada seção do template tiver uma resposta, uma premissa explícita
ou estiver marcada como não aplicável.

### 3. Redigir

Leia `./references/TEMPLATE.md` desta skill na íntegra e siga sua estrutura exatamente.

Transforme o contexto coletado em requisitos de produto.

Mantenha o PRD focado em:

- problema;
- comportamento esperado;
- valor para o usuário;
- objetivos;
- métricas;
- regras observáveis;
- restrições;
- escopo.

Evite incluir:

- arquitetura detalhada;
- implementação;
- código;
- estruturas de banco;
- nomes de classes;
- nomes de packages;
- detalhes internos de APIs;
- algoritmos.

Esses itens pertencem à TechSpec.

**Conclua quando:** toda seção do template estiver preenchida com informações
específicas da funcionalidade ou produto.

### 4. Validar

Antes de salvar, verifique:

- Todos os requisitos funcionais possuem identificadores `RF`.
- Todos os critérios de aceitação possuem identificadores `CA`.
- Critérios de aceitação são observáveis e testáveis.
- Objetivos possuem métricas ou critérios mensuráveis sempre que possível.
- Não existem decisões de implementação indevidamente colocadas no PRD.
- O conteúdo não contradiz o arquivo de entrada.

### 5. Salvar e reportar

Grave o documento em:

`./tasks/prd-[slug]/prd.md`

Use um slug da funcionalidade em kebab-case.

Informe ao usuário:

- caminho do arquivo criado;
- nome da funcionalidade;
- resumo de uma linha do escopo definido.