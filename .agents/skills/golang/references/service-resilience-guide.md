# Guia de serviços, resiliência e operação em Go

Este guia deriva de **gRPC Microservices in Go** e **12 Factor Applications with Docker and Go**.

## Context, deadlines e propagação

Chamadas entre serviços devem possuir timeout/deadline coerente com o orçamento da operação. Propague o contexto recebido quando a chamada downstream pertence à mesma operação; não substitua silenciosamente por `context.Background()`.

Cancele contextos criados localmente quando não forem mais necessários.

## Retry

Retry é adequado para falhas transitórias conhecidas, não para qualquer erro.

Antes de adicionar retry, determine:

- quais códigos/erros são transitórios;
- número máximo de tentativas;
- backoff;
- deadline total;
- idempotência da operação;
- risco de multiplicar carga durante indisponibilidade.

Não aplique retry cego a erro de validação ou regra de negócio.

## Circuit breaker

Circuit breaker pode evitar insistência contra dependência degradada. Centralize preocupações transversais em interceptors/middleware quando isso reduz duplicação sem esconder semântica importante do caso de uso.

## Erros estruturados

Em gRPC, consumidores precisam de códigos semânticos para decidir comportamento. Não dependa de parsing de mensagens para decidir retry ou tratamento.

A tradução entre erro de domínio e status gRPC pertence à borda/adaptador; o domínio não deve conhecer `codes.*`.

## Observabilidade

Interceptors são pontos adequados para tracing e métricas de chamadas gRPC. Propague contexto de tracing entre serviços.

Instrumente também operações relevantes de banco e dependências, preservando separação entre observabilidade e regra de negócio.

## Twelve-Factor

Configuração que varia entre ambientes deve permanecer fora do código e ser fornecida pelo ambiente/configuração de deploy.

Banco, Redis, brokers e serviços externos devem ser tratados como backing services substituíveis por configuração.

Processos da aplicação devem evitar depender de estado local persistente. Dados duráveis pertencem aos backing services apropriados.

Disposability exige startup previsível e shutdown controlado. Um processo deve poder ser substituído sem depender de memória ou disco local de outra instância.

## Checklist

- chamadas externas têm timeout/deadline;
- contexto é propagado;
- retries são limitados e seletivos;
- operação retryable é idempotente;
- circuit breaker tem justificativa operacional;
- códigos de erro são estruturados;
- tracing atravessa fronteiras;
- config não está hardcoded por ambiente;
- estado durável não depende do filesystem local;
- shutdown libera recursos de forma controlada.

## Fontes-base

- Hüseyin Babal — *gRPC Microservices in Go*.
- Tit Petric — *12 Factor Applications with Docker and Go*.
