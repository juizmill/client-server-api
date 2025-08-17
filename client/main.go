package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	serverURL     = "http://localhost:8080/cotacao"
	clientTimeout = 300 * time.Millisecond
	outputFile    = "cotacao.txt"
)

type quoteResponse struct {
	Bid string `json:"bid"`
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), clientTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL, nil)
	if err != nil {
		log.Fatalf("erro ao criar request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			log.Fatalf("timeout ao chamar servidor (>%v): %v", clientTimeout, err)
		}
		log.Fatalf("erro ao chamar servidor: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("servidor retornou status %d", resp.StatusCode)
	}

	var q quoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&q); err != nil {
		log.Fatalf("erro ao decodificar resposta do servidor: %v", err)
	}
	if q.Bid == "" {
		log.Fatalf("resposta do servidor sem campo 'bid'")
	}

	content := fmt.Sprintf("Dólar: %s", q.Bid)
	if err := os.WriteFile(outputFile, []byte(content), 0644); err != nil {
		log.Fatalf("erro ao escrever arquivo %s: %v", outputFile, err)
	}

	log.Printf("Cotação salva em %s: %s", outputFile, content)
}
