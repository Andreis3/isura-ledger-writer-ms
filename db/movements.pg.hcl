table "movements" {
  schema = schema.public

  column "id" {
    type = text
    null = false
  }

  column "transaction_id" {
    type = text
    null = false
  }

  column "type" {
    type = varchar(100)
    null = false
  }

  column "description" {
    type = text
    null = false
  }

  primary_key {
    columns = [column.id]
  }

  foreign_key "fk_movements_transaction_id" {
    columns     = [column.transaction_id]
    ref_columns = [table.transactions.column.id]
  }

  index "unique_movement_id_transaction_id" {
    columns = [column.id, column.transaction_id]
    unique  = true
  }

  index "idx_movements_transaction_id" {
    columns = [column.transaction_id]
  }
}
