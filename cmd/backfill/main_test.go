package main

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/andreis3/isura-ledger-ms/internal/infra/configs"
)

func TestDatabaseURLUsesEscapedCredentialsAndConfiguredEndpoint(t *testing.T) {
	configuration := &configs.Configs{}
	configuration.DataBase.Postgres = configs.Postgres{
		Host:     "::1",
		Port:     5432,
		User:     "ledger user",
		Password: "p@ss:/word",
		Database: "ledger/db",
		SSLMode:  "disable",
	}

	connectionString, err := databaseURL(configuration)
	if err != nil {
		t.Fatalf("databaseURL returned an error: %v", err)
	}
	parsed, err := pgxpool.ParseConfig(connectionString)
	if err != nil {
		t.Fatalf("ParseConfig returned an error: %v", err)
	}
	if parsed.ConnConfig.Host != "::1" || parsed.ConnConfig.Port != 5432 || parsed.ConnConfig.Database != "ledger/db" {
		t.Fatalf("unexpected PostgreSQL endpoint: host=%q port=%d database=%q", parsed.ConnConfig.Host, parsed.ConnConfig.Port, parsed.ConnConfig.Database)
	}
	if parsed.ConnConfig.User != "ledger user" || parsed.ConnConfig.Password != "p@ss:/word" {
		t.Fatal("database credentials were not preserved through URL encoding")
	}
}

func TestDatabaseURLRejectsMissingConfiguration(t *testing.T) {
	if _, err := databaseURL(nil); err == nil {
		t.Fatal("expected nil configuration to be rejected")
	}
	if _, err := databaseURL(&configs.Configs{}); err == nil {
		t.Fatal("expected incomplete PostgreSQL configuration to be rejected")
	}
}
