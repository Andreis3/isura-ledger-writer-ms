table "accounts" {
  schema = schema.public

  column "id" {
    type = varchar(36)
    null = false
  }

  column "account_external_id" {
    type = varchar(36)
    null = false
  }

  column "account_number" {
    type = varchar(36)
    null = false
  }

  column "tax_id" {
    type = varchar(14)
    null = false
  }

  column "status" {
      type    = varchar(20)
      null    = false
  }

  column "type" {
      type   = varchar(14)
      null   = false
    }

  column "balance_policy" {
    type = varchar(32)
    null = false
  }

  column "currency" {
    type = varchar(5)
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

  index "idx_account_number_currency" {
      columns = [column.account_number, column.currency]
      unique  = true
    }

    index "idx_account_external_currency" {
      columns = [column.account_external_id, column.currency]
      unique  = true
    }

    index "idx_tax_id" {
      columns = [column.tax_id]
    }

    index "idx_account_type" {
      columns = [column.type]
    }

  check "accounts_balance_policy_valid" {
    expr = "balance_policy IN ('BALANCE_NON_NEGATIVE', 'BALANCE_UNRESTRICTED')"
  }
}
