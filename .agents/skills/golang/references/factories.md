# Factories e composição

As factories em `internal/infra/factory` são o ponto de composição dos handlers e commands da aplicação. Elas recebem `*dependency.BaseDeps`, obtêm repositories pelo `dependency.Composer`, criam publishers ou UoW específicos quando necessário e devolvem o handler pronto para o transporte.

## Responsabilidades

- Uma factory por command/handler, com nome no formato `create_account_factory.go`.
- Receber as dependências compartilhadas por `*dependency.BaseDeps`; não criar configuração, PostgreSQL, NATS, logger, tracer ou Prometheus dentro da factory.
- Usar `dependency.NewComposer(baseDeps)` para construir repositories decorados com observabilidade.
- Montar o command com suas portas de domínio/aplicação e suas dependências técnicas.
- Criar o handler por último e injetar nele o command, logger e tracer.
- Respeitar o transporte do handler: handlers REST ficam em `internal/transport/rest/handler`; handlers de fila ficam em `internal/transport/queue/handler`.

A instanciação das ferramentas-base pertence a `internal/infra/dependency/base_deps.go`; a composição de repositories pertence a `internal/infra/dependency/composer_repository.go`; a factory apenas conecta essas peças ao caso de uso.

## Factory REST com publisher

Este é o padrão usado por `create_account_factory.go`:

```go
func NewCreateAccountFactory(
	baseDeps *dependency.BaseDeps,
) *handler.CreateAccountHandler {
	composeBuild := dependency.NewComposer(baseDeps)
	natsClient := nats.NewJetStreamPublisher(baseDeps.Nats.JS, baseDeps.Tracer)
	accountCommand := command.NewCreateAccount(
		composeBuild.BuildAccountRepo(),
		natsClient,
		baseDeps.Log,
		baseDeps.Tracer,
		baseDeps.Prom,
	)

	createAccountHandler := handler.NewCreateAccountHandler(
		accountCommand,
		baseDeps.Log,
		baseDeps.Tracer,
	)

	return createAccountHandler
}
```

O publisher é criado a partir do cliente NATS já inicializado em `BaseDeps`. Não abra uma nova conexão NATS a cada factory.

## Factory de handler de fila

`create_balance_factory.go` demonstra a composição de um handler de fila sem publisher direto:

```go
func NewCreateBalanceFactory(
	baseDeps *dependency.BaseDeps,
) *handler.CreateBalanceHandler {
	composeBuild := dependency.NewComposer(baseDeps)
	balanceCommand := command.NewCreateBalance(
		composeBuild.BuildBalance(),
		composeBuild.BuildAccountRepo(),
		baseDeps.Log,
		baseDeps.Tracer,
		baseDeps.Prom,
	)

	return handler.NewCreateBalanceHandler(
		balanceCommand,
		baseDeps.Log,
		baseDeps.Tracer,
	)
}
```

## Factory com Unit of Work

Quando o command precisa coordenar uma transação, a factory cria o UoW usando o pool PostgreSQL existente e compõe todos os repositories necessários:

```go
func NewCreateTransactionFactory(
	baseDeps *dependency.BaseDeps,
) *handler.CreateTransactionHandler {
	composeBuild := dependency.NewComposer(baseDeps)
	uowDep := uow.NewUnitOfWork(baseDeps.Pg.Pool())
	transactionCommand := command.NewCreateTransaction(
		uowDep,
		composeBuild.BuildAccountRepo(),
		composeBuild.BuildTransactionRepo(),
		composeBuild.BuildOutboxRepo(),
		baseDeps.Tracer,
		baseDeps.Log,
		baseDeps.Prom,
	)

	return handler.NewCreateTransactionHandler(
		transactionCommand,
		baseDeps.Log,
		baseDeps.Tracer,
	)
}
```

O UoW recebe o pool já criado em `BaseDeps`; a factory não deve abrir ou fechar o pool. O lifecycle das dependências-base é responsabilidade do bootstrap/shutdown da aplicação.

## Checklist ao criar uma factory

1. Confirme o tipo de handler e o transporte correto.
2. Crie `composeBuild := dependency.NewComposer(baseDeps)` quando precisar de repositories.
3. Use os métodos `BuildAccountRepo`, `BuildBalance`, `BuildTransactionRepo` e `BuildOutboxRepo` em vez de instanciar repositories diretamente.
4. Reutilize `baseDeps.Log`, `baseDeps.Tracer` e `baseDeps.Prom`.
5. Use `baseDeps.Nats` para publishers e `baseDeps.Pg.Pool()` para UoW.
6. Mantenha a ordem de composição: dependências técnicas, command, handler.
7. Não coloque regra de negócio, validação de domínio ou lógica de transporte na factory.
