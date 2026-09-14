# AGENTS.md — contexto do isura-ledger-writer-ms

## Objetivo deste arquivo

Este arquivo define o contexto técnico estável do projeto, as convenções operacionais e as principais restrições que devem ser consideradas por agentes antes de criar, alterar, revisar ou validar código.

Regras detalhadas permanecem em `.agents/rules/`.

Quando houver regra específica em `.agents/rules/`, ela prevalece sobre orientações genéricas deste arquivo.

---

## Ordem de autoridade

Quando houver conflito entre fontes, considere nesta ordem:

1. instrução explícita do usuário para a execução atual;
2. `AGENTS.md`;
3. rules aplicáveis em `.agents/rules/`;
4. PRD para comportamento e requisitos de produto;
5. TechSpec para decisões técnicas;
6. `tasks.md` e `task_[num].md` para escopo de implementação;
7. skills aplicáveis;
8. padrões existentes no código.

Não use essa ordem para ignorar contradições materiais.

Quando duas fontes obrigatórias conflitarem de forma relevante, registre o conflito e interrompa somente a parte afetada quando não for possível decidir com segurança.

---

## Regras de codificação

As convenções obrigatórias estão em:

```text
.agents/rules/code-standards.md
```

Consulte antes de criar ou alterar código.

As regras gerais para código Go estão em:

```text
.agents/rules/golang.md
```

Consulte antes de criar ou alterar código Go, especialmente para:

* concorrência;
* graceful shutdown;
* tratamento de erros;
* factories;
* observabilidade;
* interfaces;
* context;
* goroutines;
* sincronização;
* cancelamento;
* recursos.

As regras obrigatórias de testes estão em:

```text
.agents/rules/tests.md
```

Consulte ao:

* criar testes;
* modificar testes;
* alterar código coberto por testes;
* executar validações de review ou QA.

Não invente convenções que não estejam definidas neste arquivo, nas rules ou no código existente.

---

## Visão geral

Microsserviço Go que implementa o ledger contábil da Isura Bank.

Responsabilidades principais:

* escrituração de partidas dobradas;
* saldos em tempo real;
* idempotência por `idempotency_key`;
* publicação de eventos;
* consumo assíncrono de eventos;
* persistência transacional;
* observabilidade de fluxos financeiros.

A arquitetura é hexagonal, com DDD tático.

A regra de dependências é:

```text
transport → application → domain ← infra
```

O diretório físico do projeto é `internal/infra`, embora conceitualmente represente a camada de infrastructure/adapters.

A camada `domain` não deve depender de:

* transport;
* application;
* infra;
* frameworks;
* banco de dados;
* protocolos externos.

---

## Tecnologias e integrações

* Go 1.26.x
* módulo `github.com/andreis3/isura-ledger-ms`
* PostgreSQL 18
* pgx/v5
* Atlas
* gRPC
* Protocol Buffers
* Buf v2
* Chi
* Sonic
* NATS JetStream
* `log/slog`
* Prometheus
* OpenTelemetry OTLP
* Grafana Tempo
* pprof
* Ginkgo v2
* Gomega
* Vegeta
* Docker
* Docker Compose
* Air
* buf e ginkgo estão instalados procure pelos bin

---

## Persistência

O banco principal é PostgreSQL.

Schemas e migrations são declarados em:

```text
db/*.pg.hcl
```

OBS: não uso go test ./... use sempre make unit, ao usar o go vet ./... sempre aponte para main do projeto em cmd/server 

O Atlas é a fonte de verdade para evolução do schema.

Não altere o banco manualmente como substituição de migration.

Ao alterar persistência:

* preserve invariantes do domínio;
* mantenha acesso ao banco atrás das interfaces de `internal/domain`;
* utilize pgx/v5;
* respeite transações e UoW existentes;
* considere concorrência e atomicidade;
* mantenha constraints relevantes no banco;
* evite duplicar validações críticas somente na aplicação quando puderem ser reforçadas no banco.

Quando houver mudança de schema:

1. altere os arquivos em `db/`;
2. valide a mudança;
3. aplique com `make migrate` quando o ambiente estiver disponível.

Não edite migrations ou schemas fora do fluxo previsto pelo projeto.

---

## Ledger e invariantes de domínio

As seguintes regras são invariantes do sistema e não devem ser relaxadas sem alteração explícita da especificação:

* escrituração é baseada em partidas dobradas;
* a soma dos postings de uma transação deve respeitar a invariância contábil definida pelo domínio;
* valores monetários devem manter a representação definida por `internal/domain/money`;
* regras de saldo devem respeitar o domínio de `balance`;
* transações devem preservar atomicidade;
* operações idempotentes devem respeitar `idempotency_key`;
* retries não podem criar lançamentos duplicados;
* eventos relacionados a operações persistidas devem respeitar a estratégia transacional definida pelo projeto.

Não contorne invariantes de domínio diretamente em `transport` ou `infra`.

---

## Idempotência

Operações que utilizam `idempotency_key` devem preservar a semântica existente.

Ao alterar fluxos idempotentes, verifique:

* escopo da chave;
* persistência da chave;
* repetição da mesma requisição;
* requests concorrentes com a mesma chave;
* retry;
* rollback;
* resposta retornada em repetição.

Não implemente idempotência somente em memória para fluxos que exigem garantia persistente.

---

## Transactional Outbox

O domínio possui suporte a transactional outbox em:

```text
internal/domain/outbox/
```

Quando uma operação exigir persistir estado e publicar evento como consequência lógica da mesma transação, prefira a estratégia de outbox existente em vez de publicação não transacional.

Ao alterar fluxos que utilizem outbox, preserve:

* atomicidade entre mudança de estado e registro do evento;
* idempotência;
* possibilidade de retry;
* rastreabilidade;
* consistência do payload.

Não publique evento diretamente após commit quando isso quebrar garantias já estabelecidas pela arquitetura.

---

## NATS JetStream

NATS JetStream é utilizado para publicação e consumo assíncrono.

Ao criar ou alterar producers/consumers, considere:

* subject;
* stream;
* consumer;
* durable name, quando aplicável;
* ack;
* retry;
* redelivery;
* idempotência;
* deduplicação;
* dead-letter ou estratégia equivalente existente;
* ordenação, quando necessária;
* timeout;
* shutdown gracioso.

Consumers devem assumir que uma mensagem pode ser entregue mais de uma vez quando a semântica utilizada permitir redelivery.

Não escreva consumers que dependam implicitamente de exactly-once sem suporte explícito da infraestrutura.

---

## gRPC e Protocol Buffers

Contratos fonte ficam em:

```text
proto/ledger/v1/
```

Stubs gerados ficam em:

```text
internal/transport/grpc/pb/
```

`internal/transport/grpc/pb` é código gerado.

Nunca edite arquivos gerados manualmente.

Ao alterar contratos `.proto`:

1. edite somente os arquivos fonte em `proto/ledger/v1`;
2. execute:

```text
make proto-lint
```

3. gere novamente os stubs com:

```text
make proto-gen
```

4. ajuste handlers, DTOs, adapters e testes afetados.

Os arquivos `.proto` são a fonte de verdade para contratos gRPC.

---

## REST HTTP

A API HTTP utiliza:

* Chi;
* Sonic;
* middlewares existentes;
* handlers em `internal/transport/rest`.

O transport deve:

* traduzir protocolo para DTO/command;
* validar somente requisitos de protocolo;
* delegar regra de negócio;
* traduzir erros para resposta apropriada.

Não implemente regra de negócio de domínio dentro de handlers HTTP.

---

## Observabilidade

A stack existente utiliza:

* `log/slog`;
* Prometheus;
* OpenTelemetry;
* OTLP;
* Grafana Tempo;
* pprof.

Ao adicionar observabilidade:

* reutilize a infraestrutura existente;
* mantenha correlação entre requests e operações;
* registre erros com contexto suficiente;
* não registre secrets;
* não registre dados sensíveis desnecessariamente;
* evite logs duplicados em várias camadas para o mesmo erro;
* adicione métricas somente quando houver utilidade operacional clara;
* preserve context propagation.

Não introduza uma nova stack de observabilidade sem decisão arquitetural explícita.

---

## Portas e serviços

| Serviço            | Porta padrão | Uso                                 |
| ------------------ | -----------: | ----------------------------------- |
| Ledger HTTP        |       `8080` | REST, `/health`, `/metrics` e pprof |
| Ledger gRPC        |      `50051` | API definida nos `.proto`           |
| PostgreSQL         |       `5432` | banco `isura_ledger_main`           |
| NATS               |       `4222` | clientes                            |
| NATS monitoramento |       `8222` | monitoramento HTTP                  |
| Tempo OTLP HTTP    |       `4318` | traces enviados pelo ledger         |
| Tempo OTLP gRPC    |       `4317` | ingestão OTLP gRPC                  |
| Tempo              |       `3200` | API/UI                              |
| Prometheus         |       `9091` | host → container `9090`             |
| Grafana            |       `3000` | dashboards                          |

Credenciais locais do Compose:

```text
PostgreSQL:
admin/admin

Grafana:
admin/admin
```

Essas credenciais são exclusivamente de ambiente local.

---

## Configuração

As portas do ledger e endpoints de dependências vêm de:

```text
config.json
```

`config.json` não é versionado.

Para criar configuração local:

```text
cp config.example.json config.json
```

Valores podem ser sobrescritos por variáveis de ambiente como:

```text
GRPC_PORT
HTTP_PORT
POSTGRES_*
NATS_*
OTEL_HOST
```

Não versionar:

* `config.json`;
* secrets;
* credenciais reais;
* tokens;
* configurações privadas.

Não altere `config.example.json` com secrets.

---

## Isolamento de worktree e runtime

Ao executar aplicação, testes E2E, review ou QA em worktree, não presuma que as portas padrão estejam livres.

Antes de iniciar qualquer serviço:

1. verifique a porta;
2. não encerre processo existente;
3. use override por variável de ambiente ou configuração quando disponível;
4. registre internamente os processos iniciados;
5. inicie somente serviços necessários.

Para execuções isoladas de agentes, prefira quando possível:

```text
HTTP: 3000–3099
Frontend/UI auxiliar: 5100–5199
```

Para PostgreSQL, NATS ou outros serviços adicionais, utilize portas próprias e não conflitantes quando houver múltiplos ambientes simultâneos.

As faixas acima são convenções de isolamento, não substituem as portas padrão do projeto.

Nunca encerre automaticamente processo pertencente:

* ao usuário;
* a outra worktree;
* a outro agente;
* a serviço previamente iniciado.

Ao final de uma execução, encerre somente processos criados naquela execução.

A limpeza deve ocorrer também quando a execução:

* falhar;
* for bloqueada;
* for interrompida;
* for reprovada.

---

## Árvore atual e contextos de implementação

Árvore baseada no conteúdo presente no repositório.

Arquivos gerados e diretórios de IDE/temporários estão omitidos.

```text
.
├── cmd/server/main.go
├── internal/
│   ├── domain/
│   │   ├── account/
│   │   ├── balance/
│   │   ├── entity/
│   │   ├── event/
│   │   ├── fault/
│   │   ├── money/
│   │   ├── outbox/
│   │   ├── shared/
│   │   ├── tax/
│   │   ├── transaction/
│   │   └── validator/
│   ├── application/
│   │   ├── command/
│   │   ├── dto/
│   │   └── *.go
│   ├── infra/
│   │   ├── configs/
│   │   ├── dependency/
│   │   ├── factory/
│   │   ├── logger/
│   │   ├── observability/
│   │   ├── nats/
│   │   ├── postgres/
│   │   └── server/
│   ├── transport/
│   │   ├── rest/
│   │   ├── grpc/
│   │   └── queue/
│   └── util/
├── proto/ledger/v1/
├── db/
├── tests/unit/domain/
├── vegeta/
│   ├── account/
│   ├── transaction/
│   └── nats/
├── docs/swagger.yaml
├── docker/tempo/tempo.yaml
├── docker-compose.yml
├── Dockerfile*
├── Makefile
└── config.example.json
```

---

## Fluxo arquitetural

O fluxo esperado é:

```text
transport
   ↓
DTO / command
   ↓
application
   ↓
domain
   ↓
interfaces
   ↓
infra
```

Em operações assíncronas:

```text
NATS
 ↓
transport/queue
 ↓
application
 ↓
domain
 ↓
infra
```

A camada `transport` traduz protocolo.

A camada `application` orquestra casos de uso.

A camada `domain` contém invariantes e contratos.

A camada `infra` implementa adaptadores concretos.

Não acople `domain` a detalhes de infraestrutura.

---

## Arquivos gerados

Arquivos gerados não devem ser editados manualmente.

Incluem, no mínimo:

```text
internal/transport/grpc/pb/
```

Sempre altere a fonte correspondente e execute o gerador apropriado.

Antes de modificar qualquer arquivo potencialmente gerado, confirme sua origem.

---

## Como executar

Pré-requisitos principais:

* Go;
* Docker Compose.

Ferramentas adicionais são necessárias somente para seus alvos correspondentes:

* `buf`;
* `atlas`;
* `air`;
* `ginkgo`.

Instale dependências Go com:

```text
go mod download
```

Use:

```text
go mod tidy
```

somente quando houver alteração real de dependências ou necessidade de normalização do módulo.

---

## Inicialização local

1. Crie a configuração:

```text
cp config.example.json config.json
```

2. Ajuste portas e credenciais ou utilize variáveis de ambiente.

3. Suba dependências:

```text
make up
```

O serviço `ledger` está comentado no Compose.

Para desenvolvimento local no host:

```text
make run-app
```

4. Depois que o PostgreSQL estiver disponível:

```text
make migrate
```

---

## Comandos principais

### Aplicação

```text
make run-app
```

Executa a aplicação localmente.

```text
make run-race
```

Executa a aplicação com race detector.

Use `make run-race` quando a alteração envolver:

* goroutines;
* concorrência;
* consumers;
* compartilhamento de estado;
* lifecycle;
* shutdown.

### Hot reload

```text
make air
```

### Docker

```text
make up
make down
make logs
make restart
```

### Testes unitários

```text
make unit
```

Use como validação unitária padrão.

```text
make unit-verbose
```

Use quando precisar de:

* saída detalhada;
* Ginkgo;
* race detector;
* investigação de falhas.

### Cobertura

```text
make unit-cover
make unit-report
```

Use quando:

* `tests.md` exigir;
* review exigir validação de cobertura;
* QA exigir cobertura;
* a alteração afetar área coberta por meta explícita.

Não invente meta de cobertura quando ela não estiver definida.

### Build

```text
make build
```

Executa build da imagem utilizando o fluxo Docker definido pelo projeto.

### Protocol Buffers

```text
make proto-lint
make proto-gen
```

Obrigatórios quando contratos `.proto` forem alterados.

### Banco

```text
make migrate
```

Aplica o schema Atlas usando o `DB_URL` definido no Makefile:

```text
postgres://admin:admin@localhost:5432/isura_ledger_main
```

### Carga

```text
make test-load
```

Parâmetros configuráveis:

```text
PATH_VEGETA
URL
RATE
CONNECTIONS
WORKERS
DURATION
```

Não execute teste de carga como validação padrão de toda alteração.

Use somente quando a tarefa, TechSpec, review ou QA exigir desempenho/carga.

---

## Validação mínima antes de considerar implementação concluída

Antes de marcar uma tarefa de implementação como concluída, execute as validações aplicáveis definidas nas rules.

Como baseline, considere:

```text
make unit
```

e os comandos adicionais exigidos pelo tipo de alteração.

### Se alterar Go com concorrência

Considere:

```text
make unit-verbose
make run-race
```

conforme aplicável.

### Se alterar `.proto`

Execute:

```text
make proto-lint
make proto-gen
```

e depois os testes afetados.

### Se alterar schema

Execute a validação/migration definida pelo projeto e testes de integração aplicáveis.

### Se alterar comportamento de API

Atualize quando aplicável:

```text
docs/swagger.yaml
```

e valide os testes correspondentes.

### Se alterar NATS

Valide:

* publicação;
* consumo;
* retry;
* idempotência;
* shutdown;
* testes relacionados.

Não marque uma tarefa como concluída apenas porque o código compila.

---

## Review

`executar-review` deve validar principalmente:

* rules;
* TechSpec;
* arquitetura;
* qualidade técnica;
* segurança;
* completude das tasks;
* testes;
* cobertura, quando exigida.

Review responde:

```text
"O código está tecnicamente correto e aderente ao projeto?"
```

---

## QA

`executar-qa` deve validar principalmente:

* critérios de aceitação;
* comportamento observável;
* fluxos E2E;
* regressão;
* acessibilidade;
* responsividade;
* evidências.

QA responde:

```text
"A funcionalidade se comporta como o produto exige?"
```

Não use review como substituto de QA.

Não use QA como substituto de review.

---

## Restrições importantes

Nunca:

* edite stubs protobuf gerados;
* coloque regra de negócio em transport;
* acople domain a infraestrutura;
* ignore erro retornado sem justificativa;
* quebre idempotência;
* publique eventos de forma incompatível com as garantias existentes;
* versionar secrets;
* matar processos de outra worktree;
* alterar PRD ou TechSpec silenciosamente durante implementação;
* reduzir teste ou validação apenas para fazer pipeline passar.

Prefira mudanças pequenas, coesas e compatíveis com a arquitetura existente.
