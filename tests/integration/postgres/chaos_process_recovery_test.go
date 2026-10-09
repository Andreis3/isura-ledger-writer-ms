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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestChaosTransactionChild is a real OS process launched from the test
// executable. It holds its own PostgreSQL connection and can be killed
// without stopping the parent Ginkgo process or its Testcontainers.
func TestChaosTransactionChild(t *testing.T) {
	if os.Getenv("ISURA_CHAOS_CHILD") != "1" {
		t.Skip("helper subprocess only")
	}
	childCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := pgxpool.New(childCtx, os.Getenv("ISURA_CHAOS_PG_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	key := os.Getenv("ISURA_CHAOS_KEY")
	id := os.Getenv("ISURA_CHAOS_TX_ID")
	var foundID string
	err = db.QueryRow(childCtx, "SELECT id FROM transactions WHERE idempotency_key=$1", key).Scan(&foundID)
	if err == nil {
		if foundID != id {
			t.Fatalf("idempotency key unexpectedly maps to %s", foundID)
		}
		fmt.Println("REPLAY")
		return
	}
	// A new connection is opened each time the executable is restarted.
	tx, err := db.Begin(childCtx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	_, err = tx.Exec(childCtx, `
  INSERT INTO transactions
   (id,idempotency_key,request_fingerprint,status,operation,amount,currency,created_at,updated_at)
  VALUES ($1,$2,$3,'COMPLETED','TRANSFER',10,'BRL',now(),now())`,
		id, key, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("READY")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(line) != "COMMIT" {
		t.Fatalf("unexpected parent command %q", line)
	}
	if err := tx.Commit(childCtx); err != nil {
		t.Fatal(err)
	}
	fmt.Println("COMMITTED")
}

var _ = Describe("CHAOS :: REAL PROCESS :: POSTGRES TRANSACTION RECOVERY", func() {
	It("rolls back on SIGKILL, recovers after process restart, and replays once", func() {
		testCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		key := "chaos-" + uuid.NewString()
		transactionID := uuid.NewString()
		var count int
		// This is real subprocess isolation: a SIGKILL closes its PostgreSQL
		// session without running Go defer statements or application cleanup.
		start := func() (*exec.Cmd, io.WriteCloser, *bufio.Reader, *bytes.Buffer) {
			cmd := exec.CommandContext(testCtx, os.Args[0], "-test.run=^TestChaosTransactionChild$")
			cmd.Env = append(os.Environ(), "ISURA_CHAOS_CHILD=1",
				"ISURA_CHAOS_PG_DSN="+pool.Config().ConnString(),
				"ISURA_CHAOS_KEY="+key, "ISURA_CHAOS_TX_ID="+transactionID)
			stdin, err := cmd.StdinPipe()
			Expect(err).NotTo(HaveOccurred())
			stdout, err := cmd.StdoutPipe()
			Expect(err).NotTo(HaveOccurred())
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
		readState := func(stdout *bufio.Reader) string {
			line, readErr := stdout.ReadString('\n')
			Expect(readErr).NotTo(HaveOccurred())
			return strings.TrimSpace(line)
		}

		crashed, in, output, logs := start()
		Expect(readState(output)).To(Equal("READY"))
		Expect(crashed.Process.Kill()).To(Succeed())
		Expect(crashed.Wait()).To(HaveOccurred())
		_ = in.Close()
		Eventually(func(g Gomega) {
			g.Expect(pool.QueryRow(testCtx, "SELECT count(*) FROM transactions WHERE idempotency_key=$1", key).Scan(&count)).To(Succeed())
			g.Expect(count).To(BeZero())
		}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

		restarted, stdin, stdout, logs := start()
		Expect(readState(stdout)).To(Equal("READY"))
		_, err := io.WriteString(stdin, "COMMIT\n")
		Expect(err).NotTo(HaveOccurred())
		Expect(readState(stdout)).To(Equal("COMMITTED"))
		Expect(restarted.Wait()).To(Succeed(), logs.String())
		_ = stdin.Close()

		replayed, replayIn, replayOut, replayLogs := start()
		Expect(readState(replayOut)).To(Equal("REPLAY"))
		Expect(replayed.Wait()).To(Succeed(), replayLogs.String())
		_ = replayIn.Close()
		Expect(pool.QueryRow(testCtx, "SELECT count(*) FROM transactions WHERE idempotency_key=$1", key).Scan(&count)).To(Succeed())
		Expect(count).To(Equal(1))
		DeferCleanup(func() {
			cleanupCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
			defer done()
			_, err := pool.Exec(cleanupCtx, "DELETE FROM transactions WHERE idempotency_key=$1", key)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
