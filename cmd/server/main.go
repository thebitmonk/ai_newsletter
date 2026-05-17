package main

import (
	"context"
	"log"
	"os"

	"github.com/thebitmonk/ai_newsletter/internal/db"
	"github.com/thebitmonk/ai_newsletter/internal/server"
)

func main() {
	ctx := context.Background()

	pool, err := db.Open(ctx)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	if err := server.New(pool).Run(addr); err != nil {
		log.Fatalf("server: %v", err)
	}
}
