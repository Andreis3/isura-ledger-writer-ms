# Guia de tipos, interfaces e generics em Go

Este guia deriva de **Know Go — Generics** e **100 Go Mistakes and How to Avoid Them**.

## Tipos concretos primeiro

Comece pelo problema concreto. Não introduza abstração apenas porque generics ou interfaces permitem.

Prefira tipos concretos quando existe apenas uma implementação relevante e não há necessidade real de desacoplamento.

## Interfaces

Interfaces representam comportamento. Mantenha-as pequenas e coesas e, quando fizer sentido, defina-as no pacote consumidor.

Não use uma interface grande como mecanismo para expor toda a API de uma implementação. Não introduza `interface{}`/`any` para escapar do sistema de tipos.

## Generics

Generics são úteis quando existe um algoritmo ou estrutura genuinamente independente do tipo e a parametrização elimina duplicação mantendo o código claro.

Casos típicos incluem coleções/algoritmos que executam a mesma operação sobre diferentes tipos e utilitários onde o relacionamento entre os tipos faz parte do contrato.

Não use generics quando uma interface simples já expressa o comportamento necessário. Se uma função apenas chama métodos de um contrato como `io.Writer`, receber a interface diretamente tende a ser mais simples que parametrizar o tipo.

Não introduza type parameters prematuramente. Espere existir duplicação ou uma abstração concreta a ser extraída.

## Constraints

Uma constraint deve expressar somente as operações exigidas pelo algoritmo. Evite constraints amplas sem necessidade.

Type sets devem tornar o contrato mais preciso, não criar uma hierarquia artificial.

## Código financeiro

No domínio financeiro, prefira tipos que carreguem semântica explícita em vez de valores genéricos. Não use `any` para transportar amount, currency, direction, account identifiers ou estados de transação entre camadas.

Generics não devem apagar invariantes de domínio. Uma abstração reutilizável é inadequada se torna mais fácil combinar tipos que o negócio considera incompatíveis.

## Checklist

Antes de introduzir interface ou generic:

- qual duplicação ou acoplamento concreto está sendo resolvido?
- um tipo concreto seria mais simples?
- uma interface pequena expressa melhor o comportamento?
- o type parameter participa de fato da lógica?
- a constraint é mínima?
- a abstração melhora leitura e segurança?
- ela preserva a linguagem do domínio?

## Fontes-base

- John Arundel — *Know Go — Generics*.
- Teiva Harsanyi — *100 Go Mistakes and How to Avoid Them*.
