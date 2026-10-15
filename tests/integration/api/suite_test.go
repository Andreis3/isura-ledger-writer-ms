//go:build integration

package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/infra/configs"
	"github.com/andreis3/isura-ledger-ms/internal/infra/dependency"
	"github.com/andreis3/isura-ledger-ms/internal/infra/logger"
	"github.com/andreis3/isura-ledger-ms/internal/infra/nats"
	"github.com/andreis3/isura-ledger-ms/internal/infra/observability"
	postgresadapter "github.com/andreis3/isura-ledger-ms/internal/infra/postgres"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository"
	"github.com/andreis3/isura-ledger-ms/internal/transport/rest"
	adaptermocks "github.com/andreis3/isura-ledger-ms/tests/mocks/infra/adapter"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	ctx          = context.Background()
	postgresBox  *postgrescontainer.PostgresContainer
	natsBox      testcontainers.Container
	postgresPool *postgresadapter.Postgres
	natsClient   *nats.ClientNats
	prometheus   *observability.Prometheus
	api          http.Handler
	relayCancel  context.CancelFunc
	relayDone    chan error
)

func TestAPIIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "REST API integration suite")
}

var _ = BeforeSuite(func() {
	var err error
	postgresBox, err = postgrescontainer.Run(ctx,
		"postgres:18-alpine",
		postgrescontainer.WithDatabase("isura_ledger_api_test"),
		postgrescontainer.WithUsername("admin"),
		postgrescontainer.WithPassword("admin"),
		postgrescontainer.BasicWaitStrategies(),
	)
	Expect(err).NotTo(HaveOccurred())

	dbURL, err := postgresBox.ConnectionString(ctx, "sslmode=disable")
	Expect(err).NotTo(HaveOccurred())
	_, sourceFile, _, ok := runtime.Caller(0)
	Expect(ok).To(BeTrue())
	dbDir := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "../../../db"))
	migration := execMigration(ctx, dbURL, dbDir)
	Expect(migration.err).NotTo(HaveOccurred(), migration.output)

	natsBox, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "nats:2-alpine",
			Cmd:          []string{"-js"},
			ExposedPorts: []string{"4222/tcp"},
			WaitingFor:   wait.ForLog("Server is ready").WithStartupTimeout(30 * time.Second),
		},
		Started: true,
	})
	Expect(err).NotTo(HaveOccurred())
	natsHost, err := natsBox.Host(ctx)
	Expect(err).NotTo(HaveOccurred())
	natsPort, err := natsBox.MappedPort(ctx, "4222/tcp")
	Expect(err).NotTo(HaveOccurred())

	config := &configs.Configs{
		ApplicationName: "isura-ledger-api-integration",
		Nats: configs.Nats{
			URL:     fmt.Sprintf("nats://%s:%s", natsHost, natsPort.Port()),
			Name:    "LEDGER_API_TEST_EVENTS",
			Subject: "ledger.writer.event",
			Relay: configs.OutboxRelay{
				Stream: "LEDGER_API_TEST_TRANSACTIONS", Subject: "ledger.transaction.created",
				DLQSubject: "ledger.api.integration.transaction.created.dlq",
			},
		},
		DataBase: configs.DataBase{Postgres: configs.Postgres{
			Host: "", User: "admin", Password: "admin", Database: "isura_ledger_api_test", SSLMode: "disable",
			MaxConnections: 10, MinConnections: 1, MaxConnLifetime: time.Minute, MaxConnIdleTime: time.Minute,
		}},
	}
	dbHost, err := postgresBox.Host(ctx)
	Expect(err).NotTo(HaveOccurred())
	dbPort, err := postgresBox.MappedPort(ctx, "5432/tcp")
	Expect(err).NotTo(HaveOccurred())
	config.DataBase.Postgres.Host = dbHost
	config.DataBase.Postgres.Port, err = strconv.Atoi(dbPort.Port())
	Expect(err).NotTo(HaveOccurred())

	postgresPool, err = postgresadapter.NewPostgresWithContext(ctx, config)
	Expect(err).NotTo(HaveOccurred())
	natsClient, err = nats.NewJetStreamConnection(config)
	Expect(err).NotTo(HaveOccurred())
	prometheus, err = observability.NewPrometheus()
	Expect(err).NotTo(HaveOccurred())

	deps := &dependency.BaseDeps{
		Cfg: config, Log: logger.NewLogger(), Prom: prometheus, Pg: postgresPool,
		Tracer: adaptermocks.SilentTracerMock{}, Nats: natsClient,
	}
	relay := nats.NewOutboxRelay(repository.NewOutBoxRepository(postgresPool.Pool()), natsClient.JS,
		adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{}, config.Nats.Relay)
	relayCtx, cancelRelay := context.WithCancel(ctx)
	relayCancel = cancelRelay
	relayDone = make(chan error, 1)
	go func() { relayDone <- relay.Run(relayCtx) }()
	mux := chi.NewRouter()
	rest.Setup(&rest.SetupDeps{Mux: mux, Deps: deps})
	api = mux
})

var _ = AfterSuite(func() {
	if relayCancel != nil {
		relayCancel()
		Expect(<-relayDone).NotTo(HaveOccurred())
	}
	if natsClient != nil {
		natsClient.Close()
	}
	if prometheus != nil {
		prometheus.Close()
	}
	if postgresPool != nil {
		postgresPool.Close()
	}
	if natsBox != nil {
		Expect(testcontainers.TerminateContainer(natsBox)).To(Succeed())
	}
	if postgresBox != nil {
		Expect(testcontainers.TerminateContainer(postgresBox)).To(Succeed())
	}
})

var _ = Describe("REST API", func() {
	Context("success cases", func() {
		It("should accept concurrent requests at the configured 100-entry limit", func() {
			const requestCount = 8
			const entriesPerRequest = 100
			type requestEntry struct {
				AccountID string `json:"account_id"`
				Direction string `json:"direction"`
				Amount    int    `json:"amount"`
				Currency  string `json:"currency"`
			}
			type transactionRequest struct {
				IdempotencyKey string         `json:"idempotency_key"`
				Operation      string         `json:"operation"`
				Amount         int            `json:"amount"`
				Entries        []requestEntry `json:"entries"`
			}
			type result struct {
				response *httptest.ResponseRecorder
				elapsed  time.Duration
			}

			// Arrange: each request uses isolated accounts and a balanced composition at the limit.
			requests := make([]string, requestCount)
			for requestIndex := range requestCount {
				debitAccount := createAccountThroughAPI("LIABILITY")
				creditAccount := createAccountThroughAPI("ASSET")
				entries := make([]requestEntry, 0, entriesPerRequest)
				for position := range entriesPerRequest {
					accountID, direction := debitAccount, "DEBIT"
					if position%2 != 0 {
						accountID, direction = creditAccount, "CREDIT"
					}
					entries = append(entries, requestEntry{
						AccountID: accountID, Direction: direction, Amount: 1, Currency: "BRL",
					})
				}
				body, err := json.Marshal(transactionRequest{
					IdempotencyKey: uuid.NewString(), Operation: "TRANSFER", Amount: entriesPerRequest / 2,
					Entries: entries,
				})
				Expect(err).NotTo(HaveOccurred())
				requests[requestIndex] = string(body)
			}

			// Act: exercise the HTTP handler and transaction path concurrently.
			results := make(chan result, requestCount)
			var waitGroup sync.WaitGroup
			for _, body := range requests {
				waitGroup.Add(1)
				go func(body string) {
					defer waitGroup.Done()
					started := time.Now()
					results <- result{response: callAPI(http.MethodPost, "/transactions", body, ""), elapsed: time.Since(started)}
				}(body)
			}
			waitGroup.Wait()
			close(results)

			// Assert: all requests commit all entries and retain the full event payload.
			for completed := range results {
				Expect(completed.response.Code).To(Equal(http.StatusCreated), completed.response.Body.String())
				var response struct {
					TransactionID string `json:"transaction_id"`
				}
				Expect(json.Unmarshal(completed.response.Body.Bytes(), &response)).To(Succeed())
				var entryCount int
				var eventPayload []byte
				Expect(postgresPool.Pool().QueryRow(ctx, `
					SELECT (SELECT count(*) FROM entries WHERE transaction_id = $1), payload
					FROM outbox_events WHERE aggregate_id = $1`, response.TransactionID).Scan(&entryCount, &eventPayload)).To(Succeed())
				Expect(entryCount).To(Equal(entriesPerRequest))
				Expect(len(eventPayload)).To(BeNumerically("<", 1<<20), "event must fit within the default 1 MiB NATS payload limit")
				var event struct {
					Entries []json.RawMessage `json:"entries"`
				}
				Expect(json.Unmarshal(eventPayload, &event)).To(Succeed())
				Expect(event.Entries).To(HaveLen(entriesPerRequest))
				Expect(completed.elapsed).To(BeNumerically("<", 10*time.Second))
			}
		})

		It("should create accounts and persist a transaction through the HTTP endpoints", func() {
			// Arrange
			debitAccount := createAccountThroughAPI("LIABILITY")
			creditAccount := createAccountThroughAPI("ASSET")
			idempotencyKey := uuid.NewString()
			requestBody := fmt.Sprintf(`{"debit_account_id":%q,"credit_account_id":%q,"amount":1250,"currency":"BRL","operation":"TRANSFER"}`, debitAccount, creditAccount)

			// Act
			first := callAPI(http.MethodPost, "/transactions", requestBody, idempotencyKey)
			replay := callAPI(http.MethodPost, "/transactions", requestBody, idempotencyKey)

			// Assert
			Expect(first.Code).To(Equal(http.StatusCreated), first.Body.String())
			Expect(replay.Code).To(Equal(http.StatusOK), replay.Body.String())
			var firstResult map[string]any
			var replayResult map[string]any
			Expect(json.Unmarshal(first.Body.Bytes(), &firstResult)).To(Succeed())
			Expect(json.Unmarshal(replay.Body.Bytes(), &replayResult)).To(Succeed())
			Expect(firstResult["transaction_id"]).NotTo(BeEmpty())
			Expect(replayResult["transaction_id"]).To(Equal(firstResult["transaction_id"]))
			Expect(replayResult["idempotent_replay"]).To(BeTrue())
		})

		It("should confirm a composed transaction and publish correlated movements and entries", func() {
			// Arrange: observe only events published after this request.
			consumer, err := natsClient.JS.CreateOrUpdateConsumer(ctx, "LEDGER_API_TEST_TRANSACTIONS", jetstream.ConsumerConfig{
				FilterSubject: "ledger.transaction.created", AckPolicy: jetstream.AckExplicitPolicy,
				DeliverPolicy: jetstream.DeliverNewPolicy,
			})
			Expect(err).NotTo(HaveOccurred())
			debitAccount := createAccountThroughAPI("LIABILITY")
			creditAccount := createAccountThroughAPI("ASSET")
			feeDebitAccount := createAccountThroughAPI("LIABILITY")
			feeCreditAccount := createAccountThroughAPI("ASSET")
			key := uuid.NewString()
			requestBody := fmt.Sprintf(`{"idempotency_key":%q,"operation":"TRANSFER","amount":110,"entries":[{"account_id":%q,"direction":"DEBIT","amount":100,"currency":"BRL"},{"account_id":%q,"direction":"CREDIT","amount":100,"currency":"BRL"},{"account_id":%q,"direction":"DEBIT","amount":10,"currency":"BRL"},{"account_id":%q,"direction":"CREDIT","amount":10,"currency":"BRL"}],"movements":[{"type":"TRANSFER","description":"principal transfer","entry_positions":[0,1]},{"type":"FEE","description":"transfer fee","entry_positions":[2,3]}]}`,
				key, debitAccount, creditAccount, feeDebitAccount, feeCreditAccount)

			// Act: create through REST and receive the event through the outbox relay.
			response := callAPI(http.MethodPost, "/transactions", requestBody, "")
			Expect(response.Code).To(Equal(http.StatusCreated), response.Body.String())
			var result struct {
				TransactionID string `json:"transaction_id"`
			}
			Expect(json.Unmarshal(response.Body.Bytes(), &result)).To(Succeed())
			message, err := awaitTransactionEvent(consumer, result.TransactionID)
			Expect(err).NotTo(HaveOccurred())
			defer func() { Expect(message.Ack()).To(Succeed()) }()

			// Assert: persisted rows and the published snapshot agree on group membership.
			var payload struct {
				EventVersion  int    `json:"event_version"`
				TransactionID string `json:"transaction_id"`
				Entries       []struct {
					EntryID    string `json:"entry_id"`
					Position   int    `json:"position"`
					MovementID string `json:"movement_id"`
				} `json:"entries"`
				Movements []struct {
					MovementID string   `json:"movement_id"`
					Type       string   `json:"type"`
					EntryIDs   []string `json:"entry_ids"`
				} `json:"movements"`
			}
			Expect(json.Unmarshal(message.Data(), &payload)).To(Succeed())
			Expect(payload.EventVersion).To(Equal(1))
			Expect(payload.TransactionID).To(Equal(result.TransactionID))
			Expect(payload.Entries).To(HaveLen(4))
			Expect(payload.Movements).To(HaveLen(2))
			var persistedEntries, persistedMovements int
			Expect(postgresPool.Pool().QueryRow(ctx, "SELECT count(*) FROM entries WHERE transaction_id = $1", result.TransactionID).Scan(&persistedEntries)).To(Succeed())
			Expect(postgresPool.Pool().QueryRow(ctx, "SELECT count(*) FROM movements WHERE transaction_id = $1", result.TransactionID).Scan(&persistedMovements)).To(Succeed())
			Expect(persistedEntries).To(Equal(4))
			Expect(persistedMovements).To(Equal(2))
			for movementIndex, positions := range [][]int{{0, 1}, {2, 3}} {
				movement := payload.Movements[movementIndex]
				Expect(movement.Type).To(Equal([]string{"TRANSFER", "FEE"}[movementIndex]))
				Expect(movement.EntryIDs).To(HaveLen(2))
				for _, position := range positions {
					Expect(payload.Entries[position].Position).To(Equal(position))
					Expect(payload.Entries[position].MovementID).To(Equal(movement.MovementID))
					Expect(movement.EntryIDs).To(ContainElement(payload.Entries[position].EntryID))
				}
			}
		})
	})

	Context("error cases", func() {
		It("should return a client error for an invalid transaction request", func() {
			// Arrange
			requestBody := `{"debit_account_id":"bad","credit_account_id":"bad","amount":-1,"currency":"INVALID","operation":"UNKNOWN"}`

			// Act
			response := callAPI(http.MethodPost, "/transactions", requestBody, uuid.NewString())

			// Assert
			Expect(response.Code).To(BeNumerically(">=", http.StatusBadRequest))
			Expect(response.Code).To(BeNumerically("<", http.StatusInternalServerError))
			Expect(response.Header().Get("Content-Type")).To(ContainSubstring("application/json"))
		})

		It("should reject an invalid movement without persisting transaction, entries, or outbox event", func() {
			// Arrange: the movement is missing its required description.
			debitAccount := createAccountThroughAPI("LIABILITY")
			creditAccount := createAccountThroughAPI("ASSET")
			key := uuid.NewString()
			requestBody := fmt.Sprintf(`{"idempotency_key":%q,"operation":"TRANSFER","amount":100,"entries":[{"account_id":%q,"direction":"DEBIT","amount":100,"currency":"BRL"},{"account_id":%q,"direction":"CREDIT","amount":100,"currency":"BRL"}],"movements":[{"type":"TRANSFER","entry_positions":[0,1]}]}`,
				key, debitAccount, creditAccount)

			// Act: submit the invalid request.
			response := callAPI(http.MethodPost, "/transactions", requestBody, "")

			// Assert: API translation rejects it and no operation row can be found.
			Expect(response.Code).To(BeNumerically(">=", http.StatusBadRequest))
			Expect(response.Code).To(BeNumerically("<", http.StatusInternalServerError))
			Expect(response.Header().Get("Content-Type")).To(ContainSubstring("application/json"))
			var transactionCount int
			Expect(postgresPool.Pool().QueryRow(ctx,
				"SELECT count(*) FROM transactions WHERE idempotency_key = $1", key).Scan(&transactionCount)).To(Succeed())
			Expect(transactionCount).To(BeZero())
			var partialRecords int
			Expect(postgresPool.Pool().QueryRow(ctx, `
				SELECT (SELECT count(*) FROM entries e JOIN transactions t ON t.id = e.transaction_id WHERE t.idempotency_key = $1) +
				       (SELECT count(*) FROM movements m JOIN transactions t ON t.id = m.transaction_id WHERE t.idempotency_key = $1) +
				       (SELECT count(*) FROM outbox_events WHERE payload->>'idempotency_key' = $1)`, key).Scan(&partialRecords)).To(Succeed())
			Expect(partialRecords).To(BeZero())
		})
	})
})

func createAccountThroughAPI(accountType string) string {
	requestBody := fmt.Sprintf(`{"account_external_id":%q,"account_number":%q,"tax_id":"52998224725","account_type":%q,"balance_policy":"BALANCE_UNRESTRICTED","currency":"BRL"}`,
		uuid.NewString(), strconv.FormatInt(time.Now().UnixNano(), 10), accountType)
	response := callAPI(http.MethodPost, "/accounts", requestBody, "")
	Expect(response.Code).To(Equal(http.StatusCreated), response.Body.String())
	var result struct {
		AccountID string `json:"account_id"`
	}
	Expect(json.Unmarshal(response.Body.Bytes(), &result)).To(Succeed())
	Expect(result.AccountID).NotTo(BeEmpty())
	return accountExternalID(requestBody)
}

func accountExternalID(body string) string {
	var payload struct {
		ExternalID string `json:"account_external_id"`
	}
	Expect(json.Unmarshal([]byte(body), &payload)).To(Succeed())
	return payload.ExternalID
}

func callAPI(method, path, body, idempotencyKey string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	return response
}

func awaitTransactionEvent(consumer jetstream.Consumer, transactionID string) (jetstream.Msg, error) {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		message, err := consumer.Next(jetstream.FetchMaxWait(time.Second))
		if err != nil {
			return nil, fmt.Errorf("receive transaction event: %w", err)
		}
		var event struct {
			TransactionID string `json:"transaction_id"`
		}
		if err := json.Unmarshal(message.Data(), &event); err != nil {
			if ackErr := message.Ack(); ackErr != nil {
				return nil, fmt.Errorf("acknowledge malformed transaction event: %w", ackErr)
			}
			return nil, fmt.Errorf("decode transaction event: %w", err)
		}
		if event.TransactionID == transactionID {
			return message, nil
		}
		if err := message.Ack(); err != nil {
			return nil, fmt.Errorf("acknowledge unrelated transaction event: %w", err)
		}
	}
	return nil, fmt.Errorf("timed out waiting for transaction event %s", transactionID)
}

type migrationResult struct {
	output string
	err    error
}

func execMigration(ctx context.Context, databaseURL, schemaDir string) migrationResult {
	command := exec.CommandContext(ctx, "atlas", "schema", "apply", "--auto-approve", "--url", databaseURL, "--to", "file://"+schemaDir)
	output, err := command.CombinedOutput()
	return migrationResult{output: string(output), err: err}
}
