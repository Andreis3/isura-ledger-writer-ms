# Recalcular saldos históricos

O banco não está em produção e não há contas legadas para migrar no rollout atual. Contas novas precisam informar `balance_policy`; não atribua política por inferência.

O comando de backfill permite validar ou recalcular `running_balance` quando houver dados históricos de teste ou antes de uma futura migração de dados:

```sh
# Prévia: abre uma transação, bloqueia escritas concorrentes e desfaz todas as alterações.
make backfill-balances

# Aplicar os saldos calculados; requer decisão operacional explícita.
make backfill-balances BACKFILL_ARGS=--apply
```

O comando lê `config.json` ou as variáveis de ambiente de PostgreSQL documentadas no `AGENTS.md`. A prévia é o padrão. O relatório JSON informa discrepâncias, violações da política restritiva e `ReadyForActivation`. Uma execução aplicada persiste os saldos recalculados mesmo quando o relatório exige ação sobre violações; nesse caso, o processo termina com erro e não declara prontidão.

O backfill bloqueia escrita em `accounts` e `entries` durante sua transação. Execute-o em janela de manutenção, sem tráfego de escrita do serviço. Revise as violações e inconsistências antes de considerar qualquer ativação. A execução é repetível: após uma aplicação sem alterações concorrentes, outra prévia deve reportar zero saldos divergentes.

Em uma instalação nova sem entries, o backfill não é necessário para criar as contas; a regra de saldo entra em vigor com a criação de cada conta e sua política explícita.
