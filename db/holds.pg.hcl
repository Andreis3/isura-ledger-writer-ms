table "holds" {
  schema = schema.public

  column "id" {
    type = varchar(36)
    null = false
  }
  column "account_id" {
    type = varchar(36)
    null = false
  }
  column "currency" {
    type = varchar(5)
    null = false
  }
  column "origin" {
    type = varchar(100)
    null = false
  }
  column "reference" {
    type = varchar(200)
    null = false
  }
  column "type" {
    type = varchar(32)
    null = false
  }
  column "requested_amount" {
    type = bigint
    null = false
  }
  column "allocated_amount" {
    type = bigint
    null = false
  }
  column "pending_amount" {
    type = bigint
    null = false
  }
  column "released_amount" {
    type = bigint
    null = false
  }
  column "status" {
    type = varchar(32)
    null = false
  }
  column "version" {
    type    = bigint
    null    = false
    default = 1
  }
  column "allow_partial" {
    type    = boolean
    null    = false
    default = false
  }
  column "policy_version" {
    type = varchar(100)
    null = true
  }
  column "request_fingerprint" {
    type = char(64)
    null = false
  }
  column "created_at" {
    type = timestamptz
    null = false
  }
  column "updated_at" {
    type = timestamptz
    null = false
  }

  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_holds_account_id" {
    columns     = [column.account_id]
    ref_columns = [table.accounts.column.id]
  }
  index "idx_holds_origin_reference" {
    columns = [column.origin, column.reference]
    unique  = true
  }
  index "idx_holds_account_currency_status" {
    columns = [column.account_id, column.currency, column.status]
  }
  check "holds_currency_valid" {
    expr = "currency IN ('BRL', 'USD', 'EUR')"
  }
  check "holds_type_valid" {
    expr = "type IN ('JUDICIAL', 'REGULATORY', 'GUARANTEE', 'CONTRACTUAL', 'OPERATIONAL')"
  }
  check "holds_status_valid" {
    expr = "status IN ('ALLOCATED', 'PARTIALLY_ALLOCATED', 'PENDING_REVIEW', 'RELEASED', 'REJECTED')"
  }
  check "holds_amounts_valid" {
    expr = "requested_amount > 0 AND allocated_amount >= 0 AND pending_amount >= 0 AND released_amount >= 0 AND allocated_amount <= requested_amount AND pending_amount = requested_amount - allocated_amount AND released_amount <= allocated_amount"
  }
  check "holds_fingerprint_valid" {
    expr = "length(request_fingerprint) = 64"
  }
  check "holds_identity_valid" {
    expr = "length(trim(origin)) > 0 AND length(trim(reference)) > 0"
  }
  check "holds_version_positive" {
    expr = "version > 0"
  }
}

table "hold_decisions" {
  schema = schema.public

  column "id" {
    type = varchar(36)
    null = false
  }
  column "hold_id" {
    type = varchar(36)
    null = false
  }
  column "idempotency_key" {
    type = varchar(200)
    null = false
  }
  column "request_fingerprint" {
    type = char(64)
    null = false
  }
  column "authorization_reference" {
    type = varchar(200)
    null = false
  }
  column "authorized_by" {
    type = varchar(200)
    null = false
  }
  column "action" {
    type = varchar(32)
    null = false
  }
  column "expected_version" {
    type = bigint
    null = false
  }
  column "policy_version" {
    type = varchar(100)
    null = true
  }
  column "policy_snapshot" {
    type = jsonb
    null = false
  }
  column "requested_amount" {
    type = bigint
    null = false
  }
  column "reason" {
    type = text
    null = false
  }
  column "result_status" {
    type = varchar(32)
    null = false
  }
  column "result_allocated" {
    type = bigint
    null = false
  }
  column "result_pending" {
    type = bigint
    null = false
  }
  column "created_at" {
    type = timestamptz
    null = false
  }

  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_hold_decisions_hold_id" {
    columns = [column.hold_id]
    ref_columns = [table.holds.column.id]
  }
  index "idx_hold_decisions_idempotency_key" {
    columns = [column.idempotency_key]
    unique = true
  }
  check "hold_decisions_fingerprint_valid" {
    expr = "length(request_fingerprint) = 64"
  }
  check "hold_decisions_action_valid" {
    expr = "action IN ('ALLOCATE', 'PARTIALLY_ALLOCATE', 'REJECT', 'KEEP_PENDING')"
  }
  check "hold_decisions_status_valid" {
    expr = "result_status IN ('ALLOCATED', 'PARTIALLY_ALLOCATED', 'REJECTED', 'PENDING_REVIEW')"
  }
  check "hold_decisions_amounts_valid" {
    expr = "requested_amount > 0 AND result_allocated >= 0 AND result_pending >= 0 AND result_allocated + result_pending = requested_amount"
  }
  check "hold_decisions_identity_valid" {
    expr = "length(trim(idempotency_key)) > 0 AND length(trim(authorization_reference)) > 0 AND length(trim(authorized_by)) > 0 AND length(trim(reason)) > 0"
  }
  check "hold_decisions_version_valid" {
    expr = "expected_version > 0"
  }
}

table "hold_history" {
  schema = schema.public

  column "id" {
    type = varchar(36)
    null = false
  }
  column "hold_id" {
    type = varchar(36)
    null = false
  }
  column "event_type" {
    type = varchar(32)
    null = false
  }
  column "requested_amount" {
    type = bigint
    null = false
  }
  column "allocated_delta" {
    type = bigint
    null = false
  }
  column "pending_amount" {
    type = bigint
    null = false
  }
  column "released_delta" {
    type = bigint
    null = false
  }
  column "policy_version" {
    type = varchar(100)
    null = true
  }
  column "idempotency_key" {
    type = varchar(200)
    null = true
  }
  column "reason" {
    type = text
    null = true
  }
  column "conflict_ids" {
    type    = jsonb
    null    = false
    default = sql("'[]'::jsonb")
  }
  column "created_at" {
    type = timestamptz
    null = false
  }

  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_hold_history_hold_id" {
    columns     = [column.hold_id]
    ref_columns = [table.holds.column.id]
  }
  index "idx_hold_history_hold_created" {
    columns = [column.hold_id, column.created_at, column.id]
  }
  check "hold_history_event_valid" {
    expr = "event_type IN ('INSTRUCTION_RECORDED', 'DECISION_RECORDED', 'DECISION_RESOLVED', 'RELEASE_RECORDED')"
  }
  check "hold_history_amounts_non_negative" {
    expr = "requested_amount >= 0 AND allocated_delta >= 0 AND pending_amount >= 0 AND released_delta >= 0"
  }
}

table "hold_releases" {
  schema = schema.public

  column "id" {
    type = varchar(36)
    null = false
  }
  column "hold_id" {
    type = varchar(36)
    null = false
  }
  column "idempotency_key" {
    type = varchar(200)
    null = false
  }
  column "amount" {
    type = bigint
    null = false
  }
  column "authorization_reference" {
    type = varchar(200)
    null = false
  }
  column "request_fingerprint" {
    type = char(64)
    null = false
  }
  column "created_at" {
    type = timestamptz
    null = false
  }

  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_hold_releases_hold_id" {
    columns     = [column.hold_id]
    ref_columns = [table.holds.column.id]
  }
  index "idx_hold_releases_idempotency_key" {
    columns = [column.idempotency_key]
    unique  = true
  }
  index "idx_hold_releases_hold_id" {
    columns = [column.hold_id]
  }
  check "hold_releases_amount_positive" {
    expr = "amount > 0"
  }
  check "hold_releases_fingerprint_valid" {
    expr = "length(request_fingerprint) = 64"
  }
  check "hold_releases_references_valid" {
    expr = "length(trim(idempotency_key)) > 0 AND length(trim(authorization_reference)) > 0"
  }
}
