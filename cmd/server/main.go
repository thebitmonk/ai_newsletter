package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thebitmonk/ai_newsletter/internal/cadence"
	"github.com/thebitmonk/ai_newsletter/internal/db"
	"github.com/thebitmonk/ai_newsletter/internal/issues"
	"github.com/thebitmonk/ai_newsletter/internal/nsqx"
	"github.com/thebitmonk/ai_newsletter/internal/server"
)

func main() {
	ctx := context.Background()

	pool, err := db.Open(ctx)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	stopScheduler := maybeStartScheduler(pool)
	defer stopScheduler()

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}

	srv := server.New(pool)
	go func() {
		if err := srv.Run(addr); err != nil {
			log.Fatalf("server: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
}

// maybeStartScheduler wires the cadence scheduler if NSQ env vars are set,
// returning a shutdown function. If NSQ is not configured, returns a no-op so
// the HTTP API still runs (useful for local dev without NSQ).
func maybeStartScheduler(pool *pgxpool.Pool) func() {
	nsqdAddr := os.Getenv("NSQD_TCP_ADDR")
	lookupdAddr := os.Getenv("NSQLOOKUPD_HTTP_ADDR")
	if nsqdAddr == "" || lookupdAddr == "" {
		log.Printf("cadence scheduler: NSQD_TCP_ADDR/NSQLOOKUPD_HTTP_ADDR unset, scheduler disabled")
		return func() {}
	}

	prod, err := nsqx.NewProducer(nsqdAddr)
	if err != nil {
		log.Fatalf("scheduler: producer: %v", err)
	}

	issueStore := issues.NewStore(pool)
	sched := cadence.NewScheduler(pool, prod, issueStore)

	consumer, err := nsqx.Subscribe(lookupdAddr, cadence.TickTopic, "scheduler",
		sched.HandleTick, nsqx.ConsumerOpts{MaxInFlight: 1})
	if err != nil {
		log.Fatalf("scheduler: subscribe: %v", err)
	}

	if err := sched.Bootstrap(); err != nil {
		log.Fatalf("scheduler: bootstrap: %v", err)
	}
	log.Printf("cadence scheduler: running, tick=%s", cadence.TickTopic)

	return func() {
		consumer.Stop()
		prod.Stop()
	}
}
