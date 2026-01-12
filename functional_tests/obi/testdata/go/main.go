package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type ChainRequest struct {
	Targets []string `json:"targets"`
}

type ChainResponse struct {
	Service string      `json:"service"`
	Status  int         `json:"status"`
	Targets []string    `json:"targets,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleChain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(ChainResponse{Service: "go", Status: http.StatusMethodNotAllowed, Error: "Only POST requests are allowed"})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ChainResponse{Service: "go", Status: http.StatusBadRequest, Error: fmt.Sprintf("Failed to read request body: %v", err)})
		return
	}
	defer r.Body.Close()

	var req ChainRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ChainResponse{Service: "go", Status: http.StatusBadRequest, Error: fmt.Sprintf("Failed to parse JSON: %v", err)})
		return
	}

	if len(req.Targets) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(ChainResponse{Service: "go", Status: http.StatusOK, Result: "Chain completed"})
		return
	}

	nextTarget := req.Targets[0]
	remaining := []string{}
	if len(req.Targets) > 1 {
		remaining = req.Targets[1:]
	}

	nextReq := ChainRequest{Targets: remaining}
	nextBody, err := json.Marshal(nextReq)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(ChainResponse{Service: "go", Status: http.StatusInternalServerError, Error: fmt.Sprintf("Failed to marshal request: %v", err)})
		return
	}

	url := nextTarget
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = fmt.Sprintf("http://%s/chain", nextTarget)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	reqOut, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(nextBody))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(ChainResponse{Service: "go", Status: http.StatusBadGateway, Error: fmt.Sprintf("Failed to create request to %s: %v", url, err)})
		return
	}
	reqOut.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(reqOut)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(ChainResponse{Service: "go", Status: http.StatusBadGateway, Targets: req.Targets, Error: fmt.Sprintf("Failed to call %s: %v", url, err)})
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(ChainResponse{Service: "go", Status: http.StatusBadGateway, Error: fmt.Sprintf("Failed to read response from %s: %v", url, err)})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)
}

func main() {
	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = os.Getenv("PORT")
	}
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/chain", handleChain)

	addr := fmt.Sprintf(":%s", port)
	fmt.Printf("Starting server on %s\n", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
