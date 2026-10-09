-- Read-only operational diagnostics for Isura Ledger PostgreSQL.
-- Run on a read replica or with a bounded read-only role in production.
-- Interpret threshold values with actual workload and SLA.

-- 1) Outbox backlog, failures, exhaustion and oldest pending event.
SELECT
  count(*) FILTER (WHERE status = 'PENDING') AS pending_events,
  count(*) FILTER (WHERE status = 'FAILED') AS failed_events,
  count(*) FILTER (WHERE status = 'FAILED' AND attempts >= 3) AS exhausted_events,
  coalesce(extract(epoch FROM (now() - min(created_at) FILTER
    (WHERE status IN ('PENDING','FAILED')))),0)::bigint AS oldest_unpublished_seconds
FROM outbox_events;

-- 2) Completed transactions lacking the event that was supposed to be inserted
-- atomically. Each transaction is expected to have a transaction-created event.
SELECT count(*) AS transactions_without_outbox
FROM transactions t
WHERE status = 'COMPLETED'
AND NOT EXISTS (
  SELECT 1 FROM outbox_events e
  WHERE e.aggregate_id = t.id
  AND e.event_type = 'ledger.transaction.created'
);

-- 3) Transactions with asymmetric postings, grouped by currency.
-- This is a diagnostic query; rows indicate a serious integrity violation.
SELECT transaction_id, currency,
  sum(amount) FILTER (WHERE direction = 'DEBIT') AS total_debits,
  sum(amount) FILTER (WHERE direction = 'CREDIT') AS total_credits
FROM entries
GROUP BY transaction_id, currency
HAVING coalesce(sum(amount) FILTER (WHERE direction = 'DEBIT'),0)
    <> coalesce(sum(amount) FILTER (WHERE direction = 'CREDIT'),0)
LIMIT 100;

-- 4) Duplicate or missing per-account sequence numbers.
-- Unique indexes should prevent duplicates, but do not guarantee continuity.
SELECT account_id, count(*) AS entries,
       min(sequence_number) AS first_sequence,
       max(sequence_number) AS last_sequence
FROM entries
GROUP BY account_id
HAVING min(sequence_number) <> 1
    OR max(sequence_number) <> count(*)
LIMIT 100;
