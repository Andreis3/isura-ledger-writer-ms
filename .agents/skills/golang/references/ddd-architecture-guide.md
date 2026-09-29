# Guia de DDD e arquitetura para serviços Go

Este guia combina princípios de **Learning Domain-Driven Design**, **Implementing Domain-Driven Design**, **Software Architecture: The Hard Parts** e **Patterns of Enterprise Application Architecture** para orientar decisões no código Go deste projeto.

## Linguagem do domínio

A linguagem ubíqua é válida dentro de um bounded context. Não force o mesmo significado para um termo quando contextos de negócio distintos o modelam de maneiras diferentes.

Nomes de aggregates, entities, value objects, commands e eventos devem refletir a linguagem do contexto do ledger, e não detalhes do banco ou protocolo.

## Dependências

A direção arquitetural deste projeto permanece:

```text
transport -> application -> domain <- infrastructure
```

O domínio não deve importar PostgreSQL, pgx, HTTP, gRPC, DTOs de transporte ou critérios específicos de infraestrutura.

Repositories e outros ports definem contratos necessários pela parte interna; adapters de infraestrutura traduzem esses contratos para SQL, mensageria ou serviços externos.

## Aggregate e invariantes

Use o aggregate como fronteira de consistência das invariantes que precisam ser verdadeiras na operação.

Não mova regra de negócio para repository apenas porque os dados estão no banco. O repository persiste e reconstrói o modelo; não deve se tornar o lugar onde a semântica financeira é decidida.

Value objects são adequados quando identidade não é relevante e o valor precisa proteger invariantes próprias.

## Repositories

Repository deve apresentar ao código interno uma abstração alinhada ao modelo, escondendo detalhes de persistência.

Evite vazar para o domínio:

- SQL;
- pgx/pgtype;
- nomes de índices/constraints;
- paginação ou criteria específicos do adapter;
- models de banco.

Quando um caso de uso precisa de uma consulta que não representa carregamento de aggregate completo, avalie uma porta/query model específica em vez de deformar o aggregate para atender uma leitura.

## Trade-offs arquiteturais

Não trate decisões arquiteturais como melhores práticas universais. Arquitetura distribuída envolve trade-offs entre acoplamento, consistência, disponibilidade, performance, testabilidade e complexidade operacional.

Ao propor mudança estrutural, registre:

- força que motivou a decisão;
- alternativas relevantes;
- benefício esperado;
- custo/risco;
- impacto em acoplamento e consistência;
- como a decisão será validada.

Use ADR quando a decisão tiver impacto duradouro ou alterar uma premissa estrutural.

## Dados e bounded contexts

Ownership de dados é parte da fronteira arquitetural. Compartilhar tabela ou acessar diretamente o banco de outro contexto aumenta acoplamento e deve ser decisão explícita, não atalho acidental.

Mudanças que quebram contratos de dados exigem análise de consumidores e estratégia de evolução.

## Ledger

Para este projeto, preserve a separação conceitual:

```text
accounts      -> identidade/configuração
transactions  -> envelope da operação e idempotência
entries       -> fatos contábeis append-only
outbox        -> entrega transacional de eventos de integração
```

A semântica de débito/crédito, política de saldo e invariantes de transação pertencem ao domínio.

Não redesenhe o modelo apenas para encaixá-lo em um rótulo arquitetural. Se immutable accounting ledger + transaction envelope atende às necessidades, não force "Event Sourcing puro" sem requisito que justifique a mudança.

## Testabilidade como sinal

Acoplamento excessivo costuma aparecer como dificuldade de testar componentes isoladamente. Não contorne esse sintoma com mocks de detalhes internos; revise a fronteira e o contrato.

## Checklist

Em review arquitetural, verifique:

- domínio fala a linguagem do bounded context;
- invariantes estão protegidas no modelo adequado;
- dependências apontam para dentro;
- infraestrutura não vazou para o domínio;
- repository não contém regra de negócio;
- ownership de dados está claro;
- nova abstração resolve um problema real;
- trade-offs foram explicitados;
- decisões duradouras merecem ADR;
- testabilidade não depende de conhecer detalhes internos do adapter.

## Fontes-base

- Vladik Khononov — *Learning Domain-Driven Design*.
- Vaughn Vernon — *Implementing Domain-Driven Design*.
- Neal Ford, Mark Richards, Pramod Sadalage et al. — *Software Architecture: The Hard Parts*.
- Martin Fowler — *Patterns of Enterprise Application Architecture*.
