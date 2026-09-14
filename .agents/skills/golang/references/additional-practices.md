# Dez boas práticas adicionais para Go

Estas práticas complementam a regra principal e devem ser aplicadas conforme o contexto do código.

1. **Passe `context.Context` como primeiro parâmetro.** Não armazene contextos em structs; use-o para cancelamento, deadline e propagação de requisições. Respeite `ctx.Done()` em operações longas.
2. **Prefira composição e interfaces pequenas.** Defina interfaces no pacote consumidor, com o menor conjunto de métodos necessário, e use composição antes de hierarquias artificiais.
3. **Use `defer` imediatamente após adquirir recursos.** Feche arquivos, responses, locks e transações no mesmo escopo em que foram obtidos; valide o erro de `Close` quando ele puder afetar a operação.
4. **Separe inicialização de execução.** Construa dependências em factories, valide configuração no startup e faça funções de negócio receberem dependências explícitas, evitando estado global mutável.
5. **Faça APIs públicas documentadas e estáveis.** Toda exportação nova deve ter comentário iniciado pelo nome do símbolo; evite mudanças incompatíveis e prefira adicionar métodos ou tipos quando necessário.
6. **Use testes de tabela e subtestes para variações.** Nomeie casos pelo comportamento, cubra sucesso e falhas e mantenha testes determinísticos; use `t.Parallel()` somente quando isolamento e concorrência forem seguros.
7. **Use fuzzing em parsers e entradas não confiáveis.** Adicione testes `FuzzXxx` para decodificação, validação e normalização quando casos extremos puderem causar falhas ou vulnerabilidades.
8. **Execute verificações de segurança no ciclo de entrega.** Rode `govulncheck ./...` periodicamente e em CI, atualize dependências com revisão e testes, e trate achados alcançáveis como bloqueadores de segurança.
9. **Otimize com evidência.** Antes de alterar estruturas, alocações ou concorrência por desempenho, use benchmarks, `pprof` ou profiling; preserve clareza quando o ganho não for medido.
10. **Evite vazamentos de recursos e goroutines.** Defina timeouts para I/O, limite filas e buffers, assegure que toda goroutine tenha uma condição de saída e verifique o comportamento com testes sob carga quando aplicável.

Fontes oficiais consultadas:

- [Effective Go](https://go.dev/doc/effective_go)
- [Como escrever código Go](https://go.dev/doc/code)
- [Boas práticas de segurança para Go](https://go.dev/doc/security/best-practices)
- [Documentação do pacote `context`](https://pkg.go.dev/context)
- [Documentação do pacote `testing`](https://pkg.go.dev/testing)
- [Structured Logging with slog](https://go.dev/blog/slog)

