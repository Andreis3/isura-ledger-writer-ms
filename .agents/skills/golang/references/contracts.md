# Contratos e payloads

## Contrato tipado

```go
type AccountCreated struct {
	AccountID string `json:"account_id"`
}

func Publish(ctx context.Context, event AccountCreated) error {
	// O evento possui um contrato conhecido em tempo de compilação.
	return nil
}
```

## `any` restrito à borda

```go
func DecodePayload(raw []byte) (AccountCreated, error) {
	var payload map[string]any // Exceção: payload externo sem esquema confiável.
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
