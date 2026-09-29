package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/andreis3/isura-ledger-ms/internal/infra/configs"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository"
)

const reconciliationTimeout = 5 * time.Minute

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() (runErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), reconciliationTimeout)
	defer cancel()

	configuration := configs.LoadConfig()
	if configuration == nil {
		return errors.New("load PostgreSQL configuration from config.json or environment")
	}
	pg, err := postgres.NewPostgresWithContext(ctx, configuration)
	if err != nil {
		return fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	defer pg.Close()

	tx, err := pg.Pool().BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return fmt.Errorf("begin read-only reconciliation transaction: %w", err)
	}
	txEnded := false
	defer func() {
		if txEnded {
			return
		}
		rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer rollbackCancel()
		if err := tx.Rollback(rollbackCtx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			runErr = errors.Join(runErr, fmt.Errorf("rollback reconciliation transaction: %w", err))
		}
	}()

	report, err := repository.NewLedgerReconciliation(pg).Run(database.WithTx(ctx, tx))
	if err != nil {
		return fmt.Errorf("run ledger reconciliation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("close reconciliation transaction: %w", err)
	}
	txEnded = true

	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		return fmt.Errorf("write reconciliation report: %w", err)
	}
	if !report.Reconciled {
		return fmt.Errorf("ledger reconciliation found %d account mismatches", report.MismatchCount)
	}
	return nil
}
