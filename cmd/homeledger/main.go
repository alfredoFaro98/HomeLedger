package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"homeledger.local/app/internal/server"
)

func main() {
	addr := envOrDefault("HTTP_ADDR", ":8080")

	srv := &http.Server{
		Addr:              addr,
		Handler:           server.New(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("starting HomeLedger on %s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server stopped: %v", err)
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
