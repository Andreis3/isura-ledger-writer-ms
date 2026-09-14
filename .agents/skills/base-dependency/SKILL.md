---
name: base-dependency
description: Criar, alterar ou revisar as dependências-base da aplicação Go em internal/infra/dependency/base_deps.go, centralizando configuração, clientes de infraestrutura, observabilidade e seus recursos de encerramento.
metadata:
  short-description: Composição das dependências-base da aplicação
---

# Base dependencies

As instâncias das ferramentas-base da aplicação devem ser criadas e mantidas em `internal/infra/dependency/base_deps.go`, dentro de `BaseDeps` e `BuildBaseDeps`.

## Escopo

Centralize nesse ponto as dependências compartilhadas pelos adapters e factories, incluindo:

- configuração (`configs.Configs`);
- logger;
- métricas e tracing;
- PostgreSQL;
- NATS/JetStream;
- novos clientes de infraestrutura, como Redis, quando forem adicionados.

Não crie clientes compartilhados dentro de handlers, commands, repositories ou factories específicas. As factories devem receber `*dependency.BaseDeps` e compor seus casos de uso a partir dele.

## Ao adicionar uma dependência

1. Adicione o campo tipado ao `BaseDeps`.
2. Inicialize o cliente em `BuildBaseDeps`, usando a configuração carregada por `configs.LoadConfig()`.
3. Trate imediatamente falhas de inicialização com log operacional e encerre o startup quando a dependência for obrigatória.
4. Injete o cliente nas factories ou composers que precisam dele; não exponha configuração global mutável.
5. Se o cliente possuir `Close`, `Shutdown` ou `Flush`, adicione seu encerramento ao lifecycle da aplicação e trate o erro de limpeza.
6. Atualize os testes e a composição de dependências afetados.

## Invariantes

- `BuildBaseDeps` é o único lugar para instanciar clientes compartilhados de infraestrutura.
- Mantenha tipos concretos no container de dependências; abstrações devem ser definidas nas portas consumidoras.
- A ordem de inicialização deve respeitar dependências: configuração antes dos clientes e observabilidade antes de componentes que a utilizam.
- Não use `init`, singletons globais ou criação preguiçosa sem sincronização explícita.
- Não faça `os.Exit` fora do bootstrap de inicialização; erros de runtime devem ser retornados às camadas apropriadas.
- Credenciais, tokens e strings de conexão não podem ser registrados nos logs.

## Exemplo para uma futura dependência

Se o projeto passar a usar Redis, o padrão esperado é:

```go
type BaseDeps struct {
	Cfg   *configs.Configs
	Pg    *postgres.Postgres
	Nats  *nats.ClientNats
	Redis *redis.Client
}
```

E a conexão deve ser construída em `BuildBaseDeps`, junto com PostgreSQL e NATS, usando os valores de configuração apropriados. A factory consumidora acessa `baseDeps.Redis`, enquanto o contrato de domínio/aplicação continua atrás de uma interface definida pelo consumidor.

## Verificação

Ao alterar essa composição, execute pelo menos:

```bash
gofmt -w internal/infra/dependency/base_deps.go
go test ./...
go vet ./...
```

Se a alteração envolver lifecycle, concorrência ou encerramento de clientes, execute também:

```bash
go test -race ./...
```
