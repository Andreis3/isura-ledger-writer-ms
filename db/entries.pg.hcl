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

  column "sequence_number" {
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

  column "running_balance" {
    type = bigint
    null = false
  }

  column "currency" {
    type = varchar(3)
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

  foreign_key "fk_entries_account_id" {
    columns     = [column.account_id]
    ref_columns = [table.accounts.column.id]
  }

  foreign_key "fk_entries_transaction_id" {
    columns     = [column.transaction_id]
    ref_columns = [table.transactions.column.id]
  }

  index "idx_entries_transaction_id" {
    columns = [column.transaction_id]
  }

  index "idx_entries_sequence_number_desc" {
    columns = [
      column.account_id,
      column.sequence_number,
    ]
  }

  index "unique_entry_sequence_number" {
    columns = [
      column.account_id,
      column.sequence_number,
    ]
    unique = true
  }

  check "entries_amount_positive" {
    expr = "amount > 0"
  }

  check "entries_sequence_number_positive" {
    expr = "sequence_number > 0"
  }

  check "entries_direction_valid" {
    expr = "direction IN ('DEBIT', 'CREDIT')"
  }

  check "entries_currency_valid" {
    expr = "char_length(currency) = 3"
  }
}
