# Padrões de testes

Estas regras são obrigatórias para todo código novo ou alterado. A cobertura não substitui a validação de comportamento.

## Organização de SUTs e mocks

Organize os auxiliares de teste em `tests/mocks`, espelhando os papéis das portas da aplicação:

```text
tests/
├── mocks/
│   ├── application/command/     # mocks de commands/use cases
│   ├── application/service/     # mocks de serviços de aplicação
│   ├── infra/adapter/            # logger, tracer, métricas e clientes externos
│   └── infra/repository/         # mocks dos repositories do domínio
├── unit/domain/<contexto>/       # testes de agregados/value objects
├── unit/application/<contexto>/  # testes de casos de uso
└── integration/                  # adaptadores reais com Testcontainers
```

SUT (System Under Test) é a unidade concreta sob teste. Construa-a explicitamente no início do cenário, injete dependências por interfaces e configure mocks apenas para as chamadas relevantes.

```go
type AccountRepositoryMock struct{ mock.Mock }

func (m *AccountRepositoryMock) Create(ctx context.Context, a account.Account) error {
	args := m.Called(ctx, a)
	return args.Error(0)
}

It("cria uma conta válida", func() {
	// Arrange: SUT e dependências isoladas.
	repo := new(AccountRepositoryMock)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil).Once()
	sut := command.NewCreateAccount(repo, publisherMock, loggerMock, tracerMock, metricsMock)

	// Act.
	err := sut.Execute(ctx, input)

	// Assert: comportamento e interação esperados.
	Expect(err).NotTo(HaveOccurred())
	repo.AssertExpectations(GinkgoT())
})
```

Cada mock deve implementar a interface correspondente e declarar a asserção de compilação (`var _ domain.AccountRepository = (*AccountRepositoryMock)(nil)`). Para métodos com múltiplos retornos, trate `nil` com segurança antes de fazer type assertion. Use `Once()`/`Times(n)` para validar cardinalidade e `mock.Anything` somente quando o valor não fizer parte do requisito.

Mocks de logger, tracer e métricas devem ser silenciosos por padrão e não introduzir efeitos externos. Fakes simples em memória são preferíveis quando tornam o estado observado mais claro que expectativas de mock.

Os testes unitários existentes usam Ginkgo/Gomega, arquivos de suíte separados com build tag `unit` e pacotes `*_test`; mantenha esse padrão ao adicionar novos contextos.

## Cobertura e prioridade

- Todo código de produção deve ter testes automatizados; esta regra é crítica e não deve ser ignorada.
- Mantenha cobertura mínima de 80% (`make unit-cover`/`make unit-report`).
- Priorize primeiro requisitos críticos: controle transacional/UoW, invariantes de partidas dobradas, idempotência, saldos e publicação da outbox.
- Cada teste deve focar um único conceito ou requisito; não misture comportamentos independentes no mesmo cenário.

## FIRST (sem Timely)

### Fast (rápidos)

Prefira testes unitários sem rede, banco ou serviços reais. Use mocks, stubs, fakes e in-memory conforme os padrões existentes em `auth-ms/tests` quando aplicável.

```go
repo := &fakeAccountRepository{account: account}
useCase := command.NewCreateAccount(repo, publisher, logger, tracer, metrics)
err := useCase.Execute(ctx, input)
Expect(err).NotTo(HaveOccurred())
```

### Independent (independentes)

Cada teste prepara os próprios dados e não depende da ordem, estado ou resultado de outro teste. Não use variáveis globais mutáveis nem compartilhe transações entre casos.

```go
It("rejeita saldo insuficiente", func() {
	account := newAccountWithBalance(100)
	// O cenário é completo e isolado deste e de outros testes.
	Expect(account.HasBalance(money(101))).To(BeFalse())
})
```

### Repeatable (repetíveis)

Executar o mesmo teste várias vezes deve produzir o mesmo resultado. Controle relógio, aleatoriedade e IDs; prefira relógio injetável e mocks para dependências externas.

```go
fixedNow := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
entity := newTransactionAt(fixedNow)
Expect(entity.CreatedAt()).To(Equal(fixedNow))
```

### Self-validated (autovalidados)

As asserções devem verificar resultado e efeitos relevantes, não apenas ausência de erro ou execução de linhas. Um teste deve falhar quando o comportamento exigido for quebrado.

```go
err := tx.AddEntry(debit)
Expect(err).NotTo(HaveOccurred())
Expect(tx.Entries()).To(HaveLen(1))
Expect(tx.Entries()[0].Direction()).To(Equal(domain.Debit))
```

## Estrutura dos testes

Use GIVEN/WHEN/THEN ou AAA (Arrange/Act/Assert), deixando as três etapas identificáveis. Nomeie o teste pelo requisito observado.

```go
It("completa uma transação pendente quando débito e crédito fecham", func() {
	// Arrange (Given)
	tx := newPendingTransactionWithBalancedEntries()

	// Act (When)
	err := tx.Complete()

	// Assert (Then)
	Expect(err).NotTo(HaveOccurred())
	Expect(tx.Status()).To(Equal(domain.Completed))
})
```

## Pirâmide de testes

Mantenha a maior parte na base (testes unitários rápidos), uma camada menor de integração e poucos testes de fluxo completo. Testes de integração devem validar adaptadores reais, especialmente PostgreSQL/UoW e persistência da outbox; use Testcontainers para subir dependências isoladas e descartáveis. Não transforme testes unitários em testes de integração.

## Execução e qualidade

- Rode `make unit` antes de entregar alterações.
- Para concorrência, use `make unit-verbose` (Ginkgo com race detector) quando aplicável.
- Verifique o limite de 80% com `make unit-cover` e investigue código crítico sem cobertura.
- Testes de integração devem ser identificáveis por build tag/ suíte própria e não podem exigir ambiente externo previamente configurado.
- Evite sleeps; aguarde condições com timeout explícito e limpe recursos ao final do teste.
