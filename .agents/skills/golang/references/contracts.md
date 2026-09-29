# Contratos e payloads

Contratos entre transporte, aplicação, domínio e integrações devem permanecer explícitos e tipados. O objetivo é impedir que detalhes de protocolo ou payload dinâmico atravessem as fronteiras e enfraqueçam as invariantes do domínio.

## Fluxo esperado

```text
payload externo
    ↓ decode + validação estrutural
DTO de transporte
    ↓ mapeamento explícito
input/command da aplicação
    ↓ construção/validação
entidade ou value object de domínio
```

Não passe diretamente `map[string]any`, JSON cru, request HTTP, mensagem NATS ou struct gerada por protocolo para o domínio.

## Contrato tipado

```go
type AccountCreated struct {
	AccountID string `json:"account_id"`
}

func Publish(ctx context.Context, event AccountCreated) error {
	// O evento possui contrato conhecido em tempo de compilação.
	return nil
}
```

Prefira tipos distintos quando os conceitos possuem semânticas distintas, mesmo que a representação primitiva seja igual. Em especial, não use `any` para amount, currency, direction, account identifiers, transaction status ou idempotency data.

## `any` restrito à borda

```go
func DecodePayload(raw []byte) (AccountCreated, error) {
	var payload map[string]any // Exceção: entrada externa sem esquema confiável.
	if err := json.Unmarshal(raw, &payload); err != nil {
		return AccountCreated{}, fmt.Errorf("decodificar payload: %w", err)
	}

	accountID, ok := payload["account_id"].(string)
	if !ok || accountID == "" {
		return AccountCreated{}, errors.New("payload sem account_id válido")
	}

	return AccountCreated{AccountID: accountID}, nil
}
```

Quando uma integração obrigar o uso de `any`:

1. limite-o ao adapter/borda;
2. faça type assertion/decoding imediatamente;
3. valide campos obrigatórios;
4. converta para contrato tipado antes de chamar application/domain;
5. retorne erro explícito em vez de aceitar valor parcialmente interpretado.

## DTO não é entidade

DTO representa o contrato de entrada/saída. Entidade representa comportamento e invariantes do domínio.

Não adicione tags JSON, regras HTTP, códigos gRPC ou detalhes de broker às entidades apenas para evitar mapeamento.

O mapeamento explícito é desejável quando impede acoplamento entre evolução do protocolo e evolução do modelo.

## Dinheiro e ledger

No núcleo financeiro:

- amount deve continuar em representação inteira definida pelo domínio;
- não converta dinheiro para `float32`/`float64` entre camadas;
- currency deve permanecer explícita;
- direction e status devem usar os tipos/valores reconhecidos pelo domínio;
- idempotency key e request fingerprint possuem semânticas diferentes e não devem ser tratados como strings intercambiáveis.

Validação estrutural da requisição pode acontecer na borda, mas invariantes financeiras precisam continuar protegidas pelo domínio.

## Eventos

Eventos publicados devem possuir schema explícito e versionável. Não publique entity inteira por conveniência quando consumidores precisam apenas de um contrato estável.

Ao evoluir evento existente, avalie compatibilidade dos consumidores. Mudança de nome, remoção de campo, alteração de tipo ou alteração semântica não deve ser tratada automaticamente como refactor interno.

## Repository contracts

Interfaces de repository consumidas por application/domain não devem expor:

- `pgx`, `pgtype` ou `pgconn`;
- SQL;
- structs de `internal/infra/postgres/model`;
- criteria pertencente a `internal/infra/postgres/repository/criteria`;
- tipos HTTP/gRPC/NATS.

O adapter converte o contrato interno para a tecnologia utilizada.

## Checklist

- o contrato é conhecido em tempo de compilação?
- `any` ficou restrito à borda inevitável?
- DTO e entidade continuam separados?
- tipos financeiros não perderam semântica?
- detalhes de protocolo não vazaram para o domínio?
- evento possui payload mínimo e estável?
- repository não expõe tipos de infraestrutura?
- erros de decoding/validação são explícitos?
