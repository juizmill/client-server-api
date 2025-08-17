package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	_ "modernc.org/sqlite"
)

const (
	serverAddr       = ":8080"
	externalAPIURL   = "https://economia.awesomeapi.com.br/json/last/USD-BRL"
	apiTimeout       = 200 * time.Millisecond
	dbTimeout        = 10 * time.Millisecond
	sqliteDSN        = "file:quotes.db?cache=shared&_pragma=busy_timeout(5000)"
	createTableQuery = `
CREATE TABLE IF NOT EXISTS quotes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	code TEXT NOT NULL,
	codein TEXT NOT NULL,
	bid TEXT NOT NULL,
	ts DATETIME NOT NULL
);`
	insertQuoteQuery = `INSERT INTO quotes(code, codein, bid, ts) VALUES(?, ?, ?, ?);`
)

type awesomeAPIResponse struct {
	USDBRL struct {
		Code   string `json:"code"`
		Codein string `json:"codein"`
		Bid    string `json:"bid"`
		// demais campos ignorados
	} `json:"USDBRL"`
}

type quoteResponse struct {
	Bid string `json:"bid"`
}

type server struct {
	db     *sql.DB
	client *http.Client
}

func main() {
	db, err := sql.Open("sqlite", sqliteDSN)
	if err != nil {
		log.Fatalf("erro ao abrir banco sqlite: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if _, err := db.Exec(createTableQuery); err != nil {
		log.Fatalf("erro ao criar tabela: %v", err)
	}

	s := &server{
		db:     db,
		client: &http.Client{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/cotacao", s.handleCotacao)

	log.Printf("Servidor rodando em %s:%s ", "http://localhost", serverAddr)
	if err := http.ListenAndServe(serverAddr, mux); err != nil {
		log.Fatalf("erro no servidor HTTP: %v", err)
	}
}

func (s *server) handleCotacao(w http.ResponseWriter, r *http.Request) {
	ctxAPI, cancelAPI := context.WithTimeout(r.Context(), apiTimeout)
	defer cancelAPI()

	req, err := http.NewRequestWithContext(ctxAPI, http.MethodGet, externalAPIURL, nil)
	if err != nil {
		log.Printf("erro ao criar request p/ API externa: %v", err)
		http.Error(w, "erro interno", http.StatusInternalServerError)
		return
	}

	resp, err := s.client.Do(req)
	if err != nil {
		if errors.Is(ctxAPI.Err(), context.DeadlineExceeded) {
			log.Printf("timeout ao chamar API externa (>%v): %v", apiTimeout, err)
			http.Error(w, "timeout na API externa", http.StatusGatewayTimeout)
			return
		}
		log.Printf("erro ao chamar API externa: %v", err)
		http.Error(w, "erro ao obter cotação", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("API externa retornou status %d", resp.StatusCode)
		http.Error(w, "falha na API externa", http.StatusBadGateway)
		return
	}

	var apiResp awesomeAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		log.Printf("erro ao decodificar resposta da API externa: %v", err)
		http.Error(w, "erro ao processar cotação", http.StatusBadGateway)
		return
	}

	bid := apiResp.USDBRL.Bid
	code := apiResp.USDBRL.Code
	codein := apiResp.USDBRL.Codein
	if bid == "" {
		log.Printf("resposta da API sem campo 'bid'")
		http.Error(w, "cotação indisponível", http.StatusBadGateway)
		return
	}

	ctxDB, cancelDB := context.WithTimeout(r.Context(), dbTimeout)
	defer cancelDB()

	_, err = s.db.ExecContext(ctxDB, insertQuoteQuery, code, codein, bid, time.Now().UTC())
	if err != nil {
		if errors.Is(ctxDB.Err(), context.DeadlineExceeded) {
			log.Printf("timeout ao persistir no banco (>%v): %v", dbTimeout, err)
		} else {
			log.Printf("erro ao persistir no banco: %v", err)
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(quoteResponse{Bid: bid})
}
