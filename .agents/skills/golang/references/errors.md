# Erros e `internal/domain/fault`

Neste projeto, erros atravessam as camadas preservando duas informações diferentes:

- a causa técnica original, usada para diagnóstico e encadeamento com `errors.Is`/`errors.As`;
- a classificação de domínio (`fault.Code`) e a mensagem segura (`FriendlyMessage`), usadas pelos translators REST/gRPC.

## Regras de uso

- Sempre trate erros; não ignore retornos com `_` nem use blocos `if err != nil` vazios.
- Ao propagar um erro técnico dentro da mesma camada, adicione contexto com `%w`:

```go
if err := repo.Save(ctx, entry); err != nil {
	return fmt.Errorf("salvar lançamento %s: %w", entry.ID, err)
}
```

- Ao atravessar uma fronteira de domínio, use um construtor de `fault`, como `fault.InvalidEntityError`, `fault.FindAccountError` ou `fault.SaveModelError`, em vez de criar `DomainError` manualmente.
- Preserve a causa original no campo `Cause`; `DomainError.Unwrap` permite que a cadeia continue sendo inspecionada.
- Use `errors.Is` para condições conhecidas e `errors.As`/`errors.AsType` para extrair tipos. Não compare mensagens de erro.
- Em `Close`, `Shutdown` e `Flush`, trate também os erros de limpeza.

## Erros de domínio

As funções de `internal/domain/fault/errors.go` padronizam código, mensagem segura, causa, origem e campos de validação:

```go
func CreateAccount(ctx context.Context, input AccountInput) error {
	account, err := account.New(input)
	if err != nil {
		return fault.InvalidEntityError(err, input.ValidationErrors())
	}

	if err := repo.Save(ctx, account); err != nil {
		return fault.SaveAccountError(err)
	}
	return nil
}
```

Use `fault.New`, `fault.NewWithFields` ou `fault.Wrap` quando não existir um construtor específico, escolhendo o `fault.Code` correto. Não exponha detalhes da causa técnica na resposta ao cliente; `FriendlyMessage` é a mensagem destinada à borda.

## Sentinelas e comparação semântica

As sentinelas de `internal/domain/fault/sentinel.go` são `*fault.DomainError` e podem ser comparadas por código sem depender do texto:

```go
if errors.Is(err, fault.ErrAccountNotFound) {
	return fault.FindAccountNotFoundError(err)
}

if errors.Is(err, fault.ErrAccountAlreadyExists) {
	return fault.SaveAccountAlreadyExistsError(err)
}
```

O `DomainError.Is` compara `fault.Code`. Portanto, qualquer erro encadeado com o mesmo código semântico pode ser reconhecido por `errors.Is`. Para erros locais de agregados ou infraestrutura que ainda não sejam `DomainError`, mantenha sentinelas no pacote responsável e envolva-as ao convertê-las para o domínio.

## Extração, logs e transporte

Para registrar um erro de domínio, use `fault.Attrs`, que transforma código, causa, campos, origem e mensagem segura em atributos estruturados:

```go
if err := command.Execute(ctx, input); err != nil {
	logger.ErrorContext(ctx, "falha ao executar command",
		"command", "CreateAccount",
		"error", err,
		fault.Attrs(err)...,
	)
	return err
}
```

Na borda REST/gRPC, extraia `*fault.DomainError` com `errors.As` e converta `fault.Code` para o status de protocolo. A camada de domínio não deve conhecer HTTP, gRPC ou o formato de resposta.

```go
var domainErr *fault.DomainError
if errors.As(err, &domainErr) {
	return responseError{
		Code:    string(domainErr.Code),
		Message: domainErr.FriendlyMessage,
	}
}
```

Nunca registre ou devolva credenciais, tokens, dados sensíveis ou a mensagem técnica completa quando ela puder conter informações internas.
