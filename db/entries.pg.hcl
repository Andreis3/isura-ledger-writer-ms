table "entries" {
  schema = schema.public

  column "id" {
    type = text
    null = false
  }

  column "account_id" {
        type = text
        null = false
  }

  column "transaction_id" {
      type = text
      null = false
  }

  column "account_sequence" {
    type = bigint
    null = false
  }

  column "direction" {
    type = varchar(6)
    null = false
  }

  column "amount" {
    type = bigint
    null = false
  }

  column "currency" {
    type = varchar(5)
    null = false
  }

  column "metadata" {
    type = jsonb
    null = true
  }

  column "created_at" {
        type = timestamptz
        null = false
  }

  primary_key {
    columns = [column.id]
  }

  foreign_key "fk_account_id" {
    columns = [column.account_id]
    ref_columns = [table.accounts.column.id]
  }

  foreign_key "fk_transactios_id" {
    columns = [column.transaction_id]
    ref_columns = [table.transactions.column.id]
  }

  index "idx_entries_transaction_id" {
    columns = [column.transaction_id]
  }

  index "unique_account_sequence" {
    columns = [column.account_id, column.account_sequence]
    unique = true
  }

  check "entries_amount_positive" {
    expr = "amount > 0"
  }

  check "entries_direction_valid" {
    expr = "((direction)::text = ANY ((ARRAY['DEBIT'::character varying, 'CREDIT'::character varying])::text[]))"
  }
}
