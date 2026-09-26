package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/andreis3/isura-ledger-ms/internal/infra/configs"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository"
)

func main() {
	apply := flag.Bool("apply", false, "persist recalculated running balances; without this flag only report changes")
	flag.Parse()

	if err := run(*apply); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(apply bool) error {
	configuration := configs.LoadConfig()
	connectionString, err := databaseURL(configuration)
	if err != nil {
		return err
	}
	poolConfig, err := pgxpool.ParseConfig(connectionString)
	if err != nil {
		return fmt.Errorf("parse PostgreSQL configuration: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping PostgreSQL: %w", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin backfill transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	report, err := repository.NewHistoricalBalanceBackfill(pool).Run(database.WithTx(ctx, tx))
	if err != nil {
		return fmt.Errorf("recalculate historical balances: %w", err)
	}
	if apply {
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit historical balance backfill: %w", err)
		}
	} else if err := tx.Rollback(ctx); err != nil {
		return fmt.Errorf("rollback backfill preview: %w", err)
	}

	result := struct {
		Mode string `json:"mode"`
		repository.BackfillReport
	}{Mode: "preview", BackfillReport: report}
	if apply {
		result.Mode = "applied"
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return fmt.Errorf("write backfill report: %w", err)
	}
	if !report.ReadyForActivation {
		return errors.New("backfill completed but activation checks failed; review policy violations and sequence mismatches")
	}
	return nil
}

func databaseURL(configuration *configs.Configs) (string, error) {
	if configuration == nil {
		return "", errors.New("load database configuration from config.json or environment")
	}
	postgres := configuration.DataBase.Postgres
	if strings.TrimSpace(postgres.Host) == "" || postgres.Port < 1 || postgres.User == "" || postgres.Database == "" {
		return "", errors.New("database host, port, user, and database must be configured")
	}
	connectionURL := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(postgres.User, postgres.Password),
		Host:   net.JoinHostPort(postgres.Host, strconv.Itoa(postgres.Port)),
		Path:   postgres.Database,
	}
	query := connectionURL.Query()
	query.Set("sslmode", postgres.SSLMode)
	connectionURL.RawQuery = query.Encode()
	return connectionURL.String(), nil
}
