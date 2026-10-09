# Ledger operational integrity checks

Run `db/diagnostics/ledger-health.sql` against a read-only database connection.
These checks complement (and do not replace) the full account replay performed by `make reconcile-balances`.

```bash
psql "$DB_URL" -X -v ON_ERROR_STOP=1 -f db/diagnostics/ledger-health.sql
```

## Recommended alert thresholds (starting points, not an SLA)

| Signal | Initial threshold | Response |
| --- | --- | --- |
| Oldest unpublished event | >300 seconds for 5 minutes | Investigate relay, JetStream and stuck claims |
| Failed events | sustained increase for 5 minutes | Investigate NATS connectivity and publish acknowledgements |
| Exhausted events | >0 | Page on-call and inspect DLQ; do not silently discard |
| Completed transactions without outbox | >0 | Critical ledger integrity incident |
| Unbalanced entries by transaction and currency | any row | Critical; block automatic repair |
| Sequence gaps by account | any row | Investigate persisted history and backfill provenance |

The SQL is a diagnostic snapshot, **not** a Prometheus exporter or an alert manager.
Integrate these queries with your monitoring system using a bounded read-only
database role and suitable collection intervals. Expensive full-table checks
should run on a replica or on a controlled schedule. Never include customer IDs
or raw payloads in alert labels.

## Financial reconciliation

Schedule `make reconcile-balances` separately, preferably against a read-only
snapshot or replica when supported by deployment. The CLI exits non-zero on
mismatches, suitable for a scheduled job. Capture its aggregate counts but avoid
logging customer identifiers in metrics labels.

## Incident handling

1. Preserve the database snapshot and alert timestamp.
2. Check the oldest outbox age, failed/attempt-exhausted counts, and NATS state.
3. Run read-only reconciliation and inspect any sequence gaps.
4. Do not update ledger balances or reinsert entries manually.
5. Resolve root cause, replay via documented idempotent recovery, and reconcile again.
