package main

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Structures simulating the emission of your event
type EventEnvelope struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

type BalancePayload struct {
	AccountID string `json:"account_id"`
	Asset     string `json:"asset"`
}

func main() {
	// Connects to local NATS
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Fatalf("Falha ao conectar no NATS: %v", err)
	}
	defer nc.Close()

	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatalf("Falha ao criar JetStream: %v", err)
	}

	// IMPORTANT: Adjust to the Subject in your config.json (e.g., ledger.events)
	subject := "ledger.writer.event"

	totalMsgs := 50000
	concurrency := 100

	log.Printf("🚀 Iniciando ataque NATS: %d eventos válidos para o subject '%s'...", totalMsgs, subject)

	start := time.Now()
	var wg sync.WaitGroup
	msgsPerWorker := totalMsgs / concurrency

	// 1. POISON PILL INJECTION (Intentional junk data)
	log.Println("☠️ Injetando 5 Poison Pills (JSON quebrado e Tipo Inválido)...")
	js.PublishAsync(subject, []byte(`{ "type": "CreatedBalance", JSON_QUEBRADO `))
	js.PublishAsync(subject, []byte(`{ "type": "EventoInexistenteXYZ" }`))

	// 2. MASSIVE INJECTION OF VALID EVENTS
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < msgsPerWorker; j++ {
				env := EventEnvelope{
					Type: "created_balance",
					Data: BalancePayload{
						AccountID: uuid.NewString(),
						Asset:     "BRL",
					},
				}
				b, _ := json.Marshal(env)

				// PublishAsync is vital for high-performance testing in NATS.
				_, err := js.PublishAsync(subject, b)
				if err != nil {
					log.Println("Erro ao publicar:", err)
				}
			}
		}()
	}

	wg.Wait()

	// Aguarda os acks de confirmação do servidor NATS para os disparos Async
	select {
	case <-js.PublishAsyncComplete():
	case <-time.After(5 * time.Second):
		log.Println("Timeout esperando acks do servidor NATS")
	}

	duration := time.Since(start)
	rate := float64(totalMsgs) / duration.Seconds()
	log.Printf("✅ %d mensagens (+ lixo) enviadas em %v (Taxa: %.2f msg/s)", totalMsgs, duration, rate)
}
