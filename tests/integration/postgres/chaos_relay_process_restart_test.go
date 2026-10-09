//go:build integration

package postgres_test

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	"github.com/andreis3/isura-ledger-ms/internal/infra/configs"
	infranats "github.com/andreis3/isura-ledger-ms/internal/infra/nats"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository"
	adaptermocks "github.com/andreis3/isura-ledger-ms/tests/mocks/infra/adapter"
	"github.com/jackc/pgx/v5/pgxpool"
	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Helper is run as a separate OS process, sharing only Testcontainers DB and
// broker addresses with the parent, not its PostgreSQL transactions or memory.
func TestChaosRelayChild(t *testing.T) {
	if os.Getenv("ISURA_RELAY_CHILD") != "1" {
		t.Skip("helper only")
	}
	childCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := pgxpool.New(childCtx, os.Getenv("ISURA_RELAY_PG_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := repository.NewOutBoxRepository(db)
	switch os.Getenv("ISURA_RELAY_MODE") {
	case "claim":
		items, err := r.ClaimPending(childCtx, 1000, outbox.MaxAttempts, 0)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, item := range items {
			if item.ID.String() == os.Getenv("ISURA_RELAY_EVENT") {
				found = true
			}
		}
		if !found {
			t.Fatal("target event was not claimed")
		}
		fmt.Println("CLAIMED")
		// Simulate a process that dies after the committed claim but before publish.
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	case "publish":
		nc, err := natsgo.Connect(os.Getenv("ISURA_RELAY_NATS"), natsgo.Timeout(3*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		defer nc.Close()
		js, err := jetstream.New(nc)
		if err != nil {
			t.Fatal(err)
		}
		relay := infranats.NewOutboxRelay(r, js,
			adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{},
			adaptermocks.SilentMetricsMock{},
			configs.OutboxRelay{BatchSize: 1000, MaxAttempts: outbox.MaxAttempts, RetryAfter: time.Millisecond, MaxWorkers: 2},
		)
		if err := relay.PublishBatch(childCtx); err != nil {
			t.Fatal(err)
		}
		var status string
		if err := db.QueryRow(childCtx, "SELECT status FROM outbox_events WHERE id=$1", os.Getenv("ISURA_RELAY_EVENT")).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != string(outbox.Success) {
			t.Fatalf("outbox not published after relay restart: %s", status)
		}
		fmt.Println("PUBLISHED")
	default:
		t.Fatal("unsupported helper mode")
	}
}

var _ = Describe("CHAOS :: REAL OUTBOX RELAY PROCESS RESTART", func() {
	It("recovers a claimed event after SIGKILL and publishes once through real JetStream", func() {
		testCtx, cancel := context.WithTimeout(ctx, 65*time.Second)
		defer cancel()
		broker, err := testcontainers.GenericContainer(testCtx, testcontainers.GenericContainerRequest{
			Started: true,
			ContainerRequest: testcontainers.ContainerRequest{
				Image: "nats:2.11-alpine", Cmd: []string{"-js"}, ExposedPorts: []string{"4222/tcp"},
				WaitingFor: wait.ForListeningPort("4222/tcp"),
			},
		})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			cleanupCtx, done := context.WithTimeout(context.Background(), 15*time.Second)
			defer done()
			Expect(broker.Terminate(cleanupCtx)).To(Succeed())
		})
		host, err := broker.Host(testCtx)
		Expect(err).NotTo(HaveOccurred())
		port, err := broker.MappedPort(testCtx, "4222/tcp")
		Expect(err).NotTo(HaveOccurred())
		url := fmt.Sprintf("nats://%s:%s", host, port.Port())
		nc, err := natsgo.Connect(url)
		Expect(err).NotTo(HaveOccurred())
		defer nc.Close()
		js, err := jetstream.New(nc)
		Expect(err).NotTo(HaveOccurred())
		_, err = js.CreateStream(testCtx, jetstream.StreamConfig{Name: "CHAOS_RELAY_PROCESS", Subjects: []string{"ledger.transaction.created"}, Duplicates: 2 * time.Minute})
		Expect(err).NotTo(HaveOccurred())

		// Save COMMITTED data; an uncommitted suite transaction is not visible
		// to child processes. The surrounding suite tx remains untouched.
		item := newOutbox("chaos-relay-process")
		Expect(repository.NewOutBoxRepository(pool).Save(testCtx, item)).To(Succeed())
		DeferCleanup(func() {
			cleanupCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
			defer done()
			_, cleanupErr := pool.Exec(cleanupCtx, "DELETE FROM outbox_events WHERE id=$1", item.ID.String())
			Expect(cleanupErr).NotTo(HaveOccurred())
		})

		start := func(mode string) (*exec.Cmd, io.WriteCloser, *bufio.Reader, *bytes.Buffer) {
			cmd := exec.CommandContext(testCtx, os.Args[0], "-test.run=^TestChaosRelayChild$")
			cmd.Env = append(os.Environ(), "ISURA_RELAY_CHILD=1", "ISURA_RELAY_MODE="+mode,
				"ISURA_RELAY_PG_DSN="+pool.Config().ConnString(), "ISURA_RELAY_NATS="+url, "ISURA_RELAY_EVENT="+item.ID.String())
			stdin, pipeErr := cmd.StdinPipe()
			Expect(pipeErr).NotTo(HaveOccurred())
			stdout, pipeErr := cmd.StdoutPipe()
			Expect(pipeErr).NotTo(HaveOccurred())
			stderr := new(bytes.Buffer)
			cmd.Stderr = stderr
			Expect(cmd.Start()).To(Succeed())
			DeferCleanup(func() {
				if cmd.ProcessState == nil {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
				}
			})
			return cmd, stdin, bufio.NewReader(stdout), stderr
		}
		crashed, in, output, logs := start("claim")
		line, err := output.ReadString('\n')
		Expect(err).NotTo(HaveOccurred(), logs.String())
		Expect(strings.TrimSpace(line)).To(Equal("CLAIMED"))
		Expect(crashed.Process.Kill()).To(Succeed())
		Expect(crashed.Wait()).To(HaveOccurred())
		_ = in.Close()

		restarted, restartedIn, restartedOut, restartedLogs := start("publish")
		line, err = restartedOut.ReadString('\n')
		Expect(err).NotTo(HaveOccurred(), restartedLogs.String())
		Expect(strings.TrimSpace(line)).To(Equal("PUBLISHED"))
		Expect(restarted.Wait()).To(Succeed(), restartedLogs.String())
		_ = restartedIn.Close()

		var status string
		Expect(pool.QueryRow(testCtx, "SELECT status FROM outbox_events WHERE id=$1", item.ID.String()).Scan(&status)).To(Succeed())
		Expect(status).To(Equal(string(outbox.Success)))
		stream, err := js.Stream(testCtx, "CHAOS_RELAY_PROCESS")
		Expect(err).NotTo(HaveOccurred())
		info, err := stream.Info(testCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.State.Msgs).To(BeNumerically(">=", 1))
	})
})
