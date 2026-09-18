# Padrões de codificação

Estas regras se aplicam a todo código novo ou alterado neste repositório. Os exemplos usam Go.

Regras específicas de concorrência, `context`, erros, graceful shutdown e observabilidade devem ser mantidas em `.agents/rules/golang.md` quando esse arquivo estiver disponível.

## Formatação e estilo Go

- Todo código Go deve ser formatado com `gofmt`.
- Organize imports com as ferramentas padrão do projeto.
- Comentários de símbolos exportados devem começar pelo nome do símbolo.
- Use nomes idiomáticos de Go e evite abreviações não convencionais.
- Prefira composição, funções pequenas e tipos com responsabilidades coesas.
- Não introduza abstrações, interfaces ou generics sem necessidade concreta.

## Tamanho dos arquivos

Arquivos `.go` de produção devem preferencialmente ter no máximo 500 linhas. Separe responsabilidades em pacotes ou arquivos coesos quando o limite estiver próximo.

O limite não se aplica a código gerado por ferramentas oficiais, como os arquivos em `internal/transport/grpc/pb/`. Código gerado não deve ser refatorado ou editado manualmente.

```go
// Regras de negócio em transaction.go; persistência em repository.go.
```

## Tamanho de métodos e funções

Funções novas devem preferencialmente ter até 50 linhas, excluindo código gerado. O limite considera o corpo completo da função, incluindo comentários e linhas em branco.

Se o comportamento ultrapassar esse limite, extraia etapas com nomes significativos. Uma exceção pode ser aceita quando a extração reduzir a coesão ou esconder uma sequência transacional importante; nesse caso, preserve a legibilidade e justifique a decisão no review.

```go
func (s *Service) Create(ctx context.Context, in Input) error {
	if err := validateInput(in); err != nil {
		return err
	}

	entity := buildEntity(in)
	if err := s.persist(ctx, entity); err != nil {
		return fmt.Errorf("persist entity: %w", err)
	}

	return s.publish(ctx, entity)
}
```

## Condicionais e cláusulas de guarda

Prefira early return e evite condicionais profundamente aninhadas. Funções com alta complexidade ciclomática devem ser divididas quando isso melhorar a compreensão do fluxo.

Não use um limite numérico como substituto de julgamento sobre legibilidade. Extraia regras independentes para funções com nomes significativos.

```go
func authorize(account Account, amount Money) error {
	if account.IsBlocked() {
		return ErrAccountBlocked
	}
	if amount.IsNegative() || amount.IsZero() {
		return ErrInvalidAmount
	}
	if !account.HasBalance(amount) {
		return ErrInsufficientBalance
	}
	return nil
}
```

## Quantidade de parâmetros

Evite muitos parâmetros quando representarem um único conceito ou tornarem a assinatura difícil de ler. Agrupe dados relacionados em uma struct, mas não crie structs artificiais apenas para cumprir um limite numérico.

`context.Context` deve ser o primeiro parâmetro e não conta para essa avaliação.

```go
type CreateTransactionParams struct {
	FromAccount AccountID
	ToAccount   AccountID
	Amount      Money
	Key         string
}

func (r *Repository) Create(ctx context.Context, p CreateTransactionParams) error {
	return nil
}
```

## Constantes e valores mágicos

Extraia números e strings mágicos para constantes que expressem o conceito. Não extraia valores isolados quando isso reduzir a clareza ou esconder um valor óbvio.

```go
const (
	defaultAckWait = 30 * time.Second
	ledgerSubject  = "ledger.writer.event"
)
```

## Declaração de variáveis

Declare variáveis próximas do ponto de uso e mantenha o escopo mínimo. Evite variáveis mutáveis compartilhadas sem sincronização explícita.

```go
for _, entry := range entries {
	amount := entry.Amount()
	total = total.Add(amount)
}
```

## Contexto e cancelamento

- O `context.Context` deve ser o primeiro parâmetro de operações que fazem I/O ou podem bloquear.
- Propague o contexto recebido para PostgreSQL, NATS, HTTP, gRPC e tracing.
- Não armazene `context.Context` em structs.
- Não substitua o contexto recebido por `context.Background()` em fluxos iniciados por uma requisição ou mensagem.
- Respeite cancelamento e deadlines, especialmente em consumers e graceful shutdown.

## Tratamento de erros

- Preserve a causa dos erros com `%w` quando adicionar contexto.
- Use `errors.Is` e `errors.As` para classificação de erros.
- Não descarte erros sem justificativa explícita.
- Não use `panic` para erros esperados de domínio, configuração ou infraestrutura.
- Erros retornados ao cliente devem ser traduzidos para o contrato do protocolo e não devem expor detalhes internos.
- Registre contexto suficiente para diagnóstico sem duplicar o mesmo erro em várias camadas.

```go
if err := repo.Save(ctx, entity); err != nil {
	return fmt.Errorf("save account: %w", err)
}
```

## Interfaces e dependências

- Interfaces devem ser pequenas e, preferencialmente, definidas pelo consumidor.
- Use interfaces nas fronteiras entre camadas e para dependências que precisam ser substituídas em testes ou em runtime.
- Evite criar interfaces antecipadamente sem uma necessidade concreta.
- Construa dependências em factories ou na composição da aplicação, não dentro dos casos de uso.
- Respeite `transport → application → domain ← infra`; o domínio não depende de PostgreSQL, NATS, frameworks ou protocolos externos.

## Concorrência e lifecycle

- Toda goroutine deve ter proprietário, condição de encerramento e estratégia de espera definidos.
- Propague cancelamento para goroutines criadas durante uma operação.
- Proteja estado compartilhado com sincronização explícita ou use estruturas imutáveis/confinadas.
- Não use `time.Sleep` para coordenar concorrência.
- Recursos criados devem ter fechamento garantido, ordenado e idempotente quando aplicável.
- Consumers devem tolerar redelivery e não assumir exactly-once sem garantia explícita da infraestrutura.

## Dados sensíveis e configuração

Nunca coloque chaves de API, senhas, tokens ou credenciais no código, testes versionados ou logs. Leia valores de `config.json` (fora do versionamento) ou variáveis de ambiente; use `config.example.json` como modelo.

```go
db, err := postgres.Connect(cfg.DataBase.Postgres)
if err != nil {
	return err
}
```

```go
// Incorreto: password := "senha-real-do-banco"
```

Masque dados sensíveis em logs e não os inclua em erros retornados ao cliente. Evite registrar payloads financeiros completos, tokens, credenciais e dados pessoais sem necessidade operacional.

## Persistência, transações e outbox

- Preserve invariantes de partidas dobradas, idempotência e atomicidade antes de persistir.
- Estado persistido e evento consequente devem compartilhar a mesma transação quando o fluxo exigir atomicidade.
- Prefira a transactional outbox existente para eventos derivados de operações persistidas.
- Não publique diretamente após o commit quando isso quebrar as garantias da outbox.
- Repositórios devem seguir o padrão de Unit of Work existente e não iniciar ou finalizar transações implicitamente fora desse padrão.
- Mantenha constraints relevantes no banco e não duplique somente na aplicação validações críticas que podem ser reforçadas pelo PostgreSQL.

## Dinheiro e domínio financeiro

- Valores monetários usam `Money` e a representação definida pelo domínio, normalmente em centavos (`int64`); nunca use `float64` para valores financeiros.
- Preserve moeda, precisão, sinais e regras de arredondamento definidos pelo domínio.
- Não mova regras de partidas dobradas, saldo, idempotência ou transação para transport ou infraestrutura.

## Observabilidade

- Reutilize `log/slog`, Prometheus e OpenTelemetry da infraestrutura existente.
- Preserve propagação de contexto e correlação entre requests, comandos, mensagens e operações de banco.
- Inclua operação e identificadores técnicos úteis para diagnóstico, sem incluir dados sensíveis desnecessários.
- Evite logs e métricas duplicados em várias camadas para o mesmo evento.
- Adicione métricas somente quando houver utilidade operacional clara.

## Código gerado e contratos

- Não edite stubs em `internal/transport/grpc/pb`.
- Altere somente os arquivos fonte em `proto/ledger/v1` e execute `make proto-lint` e `make proto-gen`.
- Confirme a origem de qualquer arquivo potencialmente gerado antes de modificá-lo.

## Testes e validação

Cubra o comportamento alterado no nível apropriado:

- domínio: `tests/unit/domain`;
- aplicação: `tests/unit/application`;
- adaptadores: testes de integração ou testes específicos do adaptador;
- transport: testes de handlers e contratos.

Todo código de produção novo ou alterado deve ter testes automatizados compatíveis com o risco da mudança. Execute `make unit` antes de entregar alterações. Para concorrência, lifecycle ou consumers, use também as validações exigidas por `.agents/rules/tests.md` e pelas regras de Go do projeto.
