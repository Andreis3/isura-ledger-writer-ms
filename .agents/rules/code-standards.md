# Padrões de codificação

Estas regras se aplicam a todo código novo ou alterado neste repositório. Os exemplos usam Go.

## Tamanho dos arquivos

Não crie arquivos `.go` com mais de 500 linhas. Separe responsabilidades em pacotes/arquivos coesos quando o limite estiver próximo.

```go
// Regras de negócio em transaction.go; persistência em repository.go.
```

## Tamanho de métodos e funções

Métodos e funções devem ter no máximo 50 linhas. Se o comportamento ultrapassar esse limite, extraia etapas com nomes significativos.

```go
func (s *Service) Create(ctx context.Context, in Input) error {
	if err := validateInput(in); err != nil {
		return err
	}
	entity := buildEntity(in)
	if err := s.persist(ctx, entity); err != nil {
		return err
	}
	return s.publish(entity)
}
```

## Condicionais e cláusulas de guarda

Não aninhe mais de três blocos `if/else`. Prefira early return para manter o caminho principal legível.

```go
func authorize(account Account, amount Money) error {
	if account.IsBlocked() { return ErrAccountBlocked }
	if amount.IsNegative() || amount.IsZero() { return ErrInvalidAmount }
	if !account.HasBalance(amount) { return ErrInsufficientBalance }
	return nil
}
```

## Quantidade de parâmetros

Evite mais de três parâmetros. Agrupe dados relacionados em uma struct.

```go
type CreateTransactionParams struct {
	FromAccount AccountID
	ToAccount AccountID
	Amount Money
	Key string
}

func (r *Repository) Create(ctx context.Context, p CreateTransactionParams) error { return nil }
```

## Constantes e valores mágicos

Extraia números e strings mágicos para constantes que expressem o conceito.

```go
const (
	defaultAckWait = 30 * time.Second
	ledgerSubject = "ledger.writer.event"
)
```

## Declaração de variáveis

Declare variáveis próximas do ponto de uso e mantenha o escopo mínimo.

```go
for _, entry := range entries {
	amount := entry.Amount()
	total = total.Add(amount)
}
```

## Dados sensíveis e configuração

Nunca coloque chaves de API, senhas, tokens ou credenciais no código, testes versionados ou logs. Leia valores de `config.json` (fora do versionamento) ou variáveis de ambiente; use `config.example.json` como modelo.

```go
db, err := postgres.Connect(cfg.DataBase.Postgres)
if err != nil { return err }
```

```go
// Incorreto: password := "senha-real-do-banco"
```

Masque dados sensíveis em logs e não os inclua em erros retornados ao cliente.

## Regras do projeto

- Respeite `transport → application → domain ← infrastructure`; o domínio não depende de PostgreSQL, NATS ou frameworks.
- Valores monetários usam `Money`/centavos (`int64`), nunca `float64`.
- Preserve invariantes de partidas dobradas e idempotência antes de persistir.
- Não edite stubs em `internal/transport/grpc/pb`; altere `proto` e rode `make proto-gen`.
- Cubra alterações de domínio em `tests/unit/domain` e execute `make unit`.
