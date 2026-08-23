package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

type AccountRequest struct {
	OwnerID           string `json:"owner_id"`
	AccountExternalID string `json:"account_external_id"`
	TaxID             string `json:"tax_id"`
	AccountNumber     string `json:"account_number"`
	AccountType       string `json:"account_type"`
	Currency          string `json:"currency"`
}

func main() {
	totalAccounts := flag.Int("count", 1000, "Número de contas a gerar")
	urlFlag := flag.String("url", "http://localhost:8080/accounts", "URL do endpoint de contas")
	concurrency := flag.Int("concurrency", 50, "Número de goroutines simultâneas")
	outputFile := flag.String("output", "accounts_pool.json", "Ficheiro de saída com os IDs")
	flag.Parse()

	fmt.Printf("🚀 A gerar %d contas em %s...\n", *totalAccounts, *urlFlag)

	accountingTypes := []string{"ASSET", "LIABILITY", "EQUITY", "REVENUE", "EXPENSE"}
	currencies := []string{"BRL"}

	jobs := make(chan int, *totalAccounts)
	results := make(chan string, *totalAccounts)
	var wg sync.WaitGroup

	for w := 1; w <= *concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := &http.Client{Timeout: 10 * time.Second}
			rng := rand.New(rand.NewSource(time.Now().UnixNano()))

			for range jobs {
				accountExtID := uuid.New().String()
				payload := AccountRequest{
					OwnerID:           uuid.New().String(),
					AccountExternalID: accountExtID,
					TaxID:             GerarCNPJ(rng),
					AccountNumber:     strconv.FormatInt(time.Now().UnixNano()+int64(rng.Intn(100000)), 10),
					AccountType:       accountingTypes[rng.Intn(len(accountingTypes))],
					Currency:          currencies[rng.Intn(len(currencies))],
				}

				bodyBytes, _ := json.Marshal(payload)
				resp, err := client.Post(*urlFlag, "application/json", bytes.NewBuffer(bodyBytes))
				if err != nil {
					continue
				}
				resp.Body.Close()

				if resp.StatusCode == http.StatusCreated {
					results <- accountExtID
				}
			}
		}()
	}

	for i := 0; i < *totalAccounts; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	close(results)

	var createdAccounts []string
	for id := range results {
		createdAccounts = append(createdAccounts, id)
	}

	fileData, _ := json.MarshalIndent(createdAccounts, "", "  ")
	_ = os.WriteFile(*outputFile, fileData, 0644)

	fmt.Printf("\n✅ Sucesso! %d contas guardadas no ficheiro '%s'.\n", len(createdAccounts), *outputFile)
}

func GerarCNPJ(r *rand.Rand) string {
	nums := make([]int, 12)
	for i := 0; i < 8; i++ {
		nums[i] = r.Intn(10)
	}
	nums[8], nums[9], nums[10], nums[11] = 0, 0, 0, 1

	pesos1 := []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	dv1 := calculaDV(nums, pesos1)
	numsComDv1 := append(nums, dv1)

	pesos2 := []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	dv2 := calculaDV(numsComDv1, pesos2)
	cnpjNums := append(numsComDv1, dv2)

	buf := make([]byte, 14)
	for i, n := range cnpjNums {
		buf[i] = byte('0' + n)
	}
	return string(buf)
}

func calculaDV(numeros []int, pesos []int) int {
	soma := 0
	for i, v := range numeros {
		soma += v * pesos[i]
	}
	resto := soma % 11
	if resto < 2 {
		return 0
	}
	return 11 - resto
}
