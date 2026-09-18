table "outbox_events" {
  schema = schema.public

  column "id" {
    type = text
    null = false
  }

  column "aggregate_id" {
    type = text
    null = false
  }

  column "aggregate_type" {
    type = varchar(50)
    null = false
  }

  column "event_type" {
    type = varchar(50)
    null = false
  }

  column "payload" {
    type = jsonb
    null = false
  }

  column "status" {
    type = varchar(50)
    null = false
  }

  column "attempts" {
    type = int
    null = false
  }

  column "last_attempt_at" {
    type = timestamptz
    null = true
  }

  column "published_at" {
    type = timestamptz
    null = true
  }

  column "created_at" {
    type = timestamptz
    null = false
  }

  primary_key {
    columns = [column.id]
  }

  index "idx_aggregate_id_aggregate_type" {
    columns = [column.aggregate_id, column.aggregate_type]
  }

  index "idx_outbox_events_status" {
    columns = [column.status]
  }

  check "outbox_attempts_non_negative" {
    expr = "attempts >= 0"
  }

  check "outbox_status_valid" {
    expr = "((status)::text = ANY ((ARRAY['PENDING'::character varying, 'FAILED'::character varying, 'SUCCESS'::character varying])::text[]))"
  }
}
