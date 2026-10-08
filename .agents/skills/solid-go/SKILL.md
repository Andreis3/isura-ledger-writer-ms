---
name: solid-go
description: Aplicar e revisar SOLID de forma idiomática em Go no Isura Ledger. Use ao projetar ou refatorar commands, agregados, interfaces, repositories, adapters e factories, e ao revisar acoplamento, coesão e extensibilidade.
metadata:
  short-description: SOLID idiomático para Go e DDD
---

# SOLID aplicado a Go — Isura Ledger

## Escopo e precedência

SOLID é um conjunto de critérios de projeto, não uma exigência de criar interfaces, hierarquias ou camadas adicionais. Preserve primeiro as invariantes contábeis do ledger, as decisões explícitas do PRD/TechSpec e as regras do `AGENTS.md`, `.agents/rules/` e `.agents/skills/golang/SKILL.md`.

Quando a tarefa envolver entries, saldo, sequenciamento, idempotência, UoW, concorrência transacional ou outbox, consulte `.agents/skills/golang/references/ledger-engineering-guide.md`.

## S — Single Responsibility Principle (SRP)

- Organize tipos e packages em torno de uma responsabilidade coesa e um motivo claro para mudar.
- `transport` interpreta HTTP/gRPC e traduz erros; `application` orquestra casos de uso; `domain` protege invariantes; `infra` integra banco, NATS e outros serviços.
- Um command pode coordenar UoW, repositories e outbox sem violar SRP; não deve, porém, implementar SQL ou regras contábeis próprias do domínio.
- Evite fragmentar uma operação coesa em dezenas de tipos artificiais.

## O — Open/Closed Principle (OCP)

- Prefira composição e contratos estáveis para acrescentar comportamentos quando existirem variações reais.
- Introduza estratégias somente se houver pelo menos uma necessidade concreta de substituição ou extensão.
- `switch` explícito é aceitável para conjuntos pequenos e estáveis de casos.
- Não generalize o ledger para operações hipotéticas que não aparecem nos requisitos.

## L — Liskov Substitution Principle (LSP)

- Implementações de uma interface devem preservar suas garantias observáveis, não apenas suas assinaturas.
- Repositories devem manter a semântica esperada de erro, atomicidade e cancelamento.
- Um adapter de publicação não deve confirmar sucesso se a mensagem não foi efetivamente aceita conforme o contrato.
- Mocks e fakes devem respeitar a semântica do contrato; use asserções de compilação quando pertinentes e testes comportamentais para garantias críticas.

## I — Interface Segregation Principle (ISP)

- Prefira interfaces pequenas, próximas de quem as consome, com somente os métodos necessários.
- Evite interfaces `Manager`/`Repository` gigantes que agrupam responsabilidades desconexas.
- Não crie uma interface para cada struct. Em Go, dependências concretas são apropriadas quando não há variação nem fronteira a isolar.
- Considere separar interfaces de leitura e escrita quando os casos de uso tiverem necessidades diferentes.

Exemplo ilustrativo (não representa necessariamente os tipos reais do repositório):

```go
type AccountReader interface {
    FindByID(ctx context.Context, id string) (Account, error)
}
```

## D — Dependency Inversion Principle (DIP)

- `domain` não deve importar pgx, NATS, HTTP ou SDKs de observabilidade.
- `application` depende de contratos de domínio ou portas apropriadas, enquanto `infra` fornece implementações concretas.
- Faça a composição de dependências nas factories e no bootstrap existentes; evite service locators e variáveis globais mutáveis.
- DIP não exige interfaces entre todos os packages: use abstrações somente quando protegem uma fronteira ou necessidade concreta.

## Práticas idiomáticas de Go

- Prefira composição a herança simulada e embedding usado apenas para reaproveitamento acidental.
- Use interfaces implícitas pequenas, tipos concretos e construtores claros.
- Propague `context.Context` para operações com I/O e preserve cancelamento e deadlines.
- Faça wrapping de erros com `%w` quando necessário e use `errors.Is`/`errors.As` para decisões semânticas.
- Evite `any`, reflection e generics usados apenas para esconder contratos.
- Preserve atomicidade, idempotência, sequenciamento por conta e append-only independentemente da refatoração.

## Checklist de revisão SOLID

1. Qual responsabilidade mudou e qual componente deve ser seu proprietário?
2. Existe dependência de infraestrutura vazando para `domain` ou `application`?
3. Uma interface representa uma necessidade do consumidor ou apenas espelha uma struct?
4. Uma implementação alternativa preservaria as mesmas garantias observáveis?
5. A abstração proposta reduz acoplamento real ou aumenta complexidade sem benefício?
6. A mudança mantém as invariantes e os contratos já estabelecidos do ledger?
7. Os testes verificam comportamento e efeitos, e não apenas chamadas ou detalhes internos?

## Validação e entrega

- Consulte `.agents/rules/tests.md` para estrutura, cobertura e critérios de testes.
- Execute `gofmt` apenas nos arquivos Go alterados e `make unit` antes de entregar mudanças em código de produção.
- Para concorrência, lifecycle e consumers, execute também `make unit-verbose` e as verificações adicionais aplicáveis; use `make integration-tests` quando a mudança envolver contratos de infraestrutura e `make vet` quando apropriado.
- Explique as decisões de design, alternativas rejeitadas e trade-offs; não apresente SOLID como justificativa suficiente por si só.
- Não altere código apenas para demonstrar um princípio SOLID.
