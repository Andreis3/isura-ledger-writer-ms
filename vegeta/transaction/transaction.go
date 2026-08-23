package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	vegeta "github.com/tsenart/vegeta/v12/lib"
)

var bufferPool = sync.Pool{
	New: func() interface{} {
		return bytes.NewBuffer(make([]byte, 0, 256))
	},
}

var rngPool = sync.Pool{
	New: func() interface{} {
		return rand.New(rand.NewSource(time.Now().UnixNano()))
	},
}

func main() {
	rateFlag := flag.Int("rate", 1000, "Request rate per second")
	durationFlag := flag.Duration("duration", 60*time.Second, "Test duration")
	urlFlag := flag.String("url", "http://localhost:8080/transactions", "Target URL")
	connectionsFlag := flag.Int("connections", 5000, "Maximum simultaneous connections")
	workersFlag := flag.Int("workers", 500, "Number of parallel workers")
	inputFile := flag.String("input", "accounts_pool.json", "Ficheiro com o pool de contas")
	flag.Parse()

	// Lê o pool de contas gerado pelo seed
	fileData, err := os.ReadFile(*inputFile)
	if err != nil {
		fmt.Printf("❌ Erro ao ler o ficheiro de contas '%s': %v\nCertifique-se de executar o seed primeiro.\n", *inputFile, err)
		os.Exit(1)
	}

	var accountPool []string
	if err := json.Unmarshal(fileData, &accountPool); err != nil || len(accountPool) < 2 {
		fmt.Printf("❌ O ficheiro de contas está vazio ou inválido.\n")
		os.Exit(1)
	}

	fmt.Printf("📂 Carregadas %d contas do ficheiro '%s' para o teste.\n", len(accountPool), *inputFile)

	rate := vegeta.Rate{Freq: *rateFlag, Per: time.Second}
	duration := *durationFlag
	targetURL := *urlFlag

	targeter := func(tgt *vegeta.Target) error {
		if tgt == nil {
			return vegeta.ErrNoTargets
		}

		rng := rngPool.Get().(*rand.Rand)
		idempotencyKey := uuid.New().String()

		debitIdx := rng.Intn(len(accountPool))
		creditIdx := rng.Intn(len(accountPool))
		for creditIdx == debitIdx {
			creditIdx = rng.Intn(len(accountPool))
		}

		debitAccountID := accountPool[debitIdx]
		creditAccountID := accountPool[creditIdx]
		rngPool.Put(rng)

		tgt.Method = http.MethodPost
		tgt.URL = targetURL
		tgt.Header = http.Header{
			"Content-Type": []string{"application/json"},
		}

		buf := bufferPool.Get().(*bytes.Buffer)
		buf.Reset()

		buf.WriteString(`{"idempotency_key": "`)
		buf.WriteString(idempotencyKey)
		buf.WriteString(`", "debit_account_id": "`)
		buf.WriteString(debitAccountID)
		buf.WriteString(`", "credit_account_id": "`)
		buf.WriteString(creditAccountID)
		buf.WriteString(`", "operation": "PIX_IN", "amount": 10000, "currency": "BRL"}`)

		tgt.Body = append([]byte(nil), buf.Bytes()...)
		bufferPool.Put(buf)

		return nil
	}

	tr := &http.Transport{
		MaxIdleConns:        50000,
		MaxIdleConnsPerHost: 50000,
		MaxConnsPerHost:     0,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
		DisableKeepAlives:   false,
		ForceAttemptHTTP2:   true,
	}
	client := &http.Client{Transport: tr}

	attacker := vegeta.NewAttacker(
		vegeta.Client(client),
		vegeta.Connections(*connectionsFlag),
		vegeta.Workers(uint64(*workersFlag)),
	)
	var metrics vegeta.Metrics

	fmt.Printf("Starting Distributed Transaction Load Test \n"+
		"| URL: %s \n"+
		"| Rate: %d req/s \n"+
		"| Duration: %v...\n", targetURL, *rateFlag, duration)

	for res := range attacker.Attack(targeter, rate, duration, "Distributed Transaction Load Test") {
		metrics.Add(res)
	}
	metrics.Close()

	reporter := vegeta.NewTextReporter(&metrics)
	reporter.Report(os.Stdout)
}
