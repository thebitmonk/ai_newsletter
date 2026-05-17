package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thebitmonk/ai_newsletter/internal/cadence"
	"github.com/thebitmonk/ai_newsletter/internal/candidates"
	"github.com/thebitmonk/ai_newsletter/internal/db"
	"github.com/thebitmonk/ai_newsletter/internal/issues"
	"github.com/thebitmonk/ai_newsletter/internal/nsqx"
	"github.com/thebitmonk/ai_newsletter/internal/server"
	"github.com/thebitmonk/ai_newsletter/internal/sourceadapter"
	"github.com/thebitmonk/ai_newsletter/internal/sourceadapter/rss"
	"github.com/thebitmonk/ai_newsletter/internal/sourceadapter/substack"
	"github.com/thebitmonk/ai_newsletter/internal/sourceadapter/web"
	"github.com/thebitmonk/ai_newsletter/internal/sourceadapter/xhandle"
	"github.com/thebitmonk/ai_newsletter/internal/sourceadapter/youtube"
	"github.com/thebitmonk/ai_newsletter/internal/sources"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := db.Open(ctx)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	serverOpts, stopWorkers := maybeStartWorkers(ctx, pool)
	defer stopWorkers()

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	srv := server.New(pool, serverOpts...)
	go func() {
		if err := srv.Run(addr); err != nil {
			log.Fatalf("server: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
}

// maybeStartWorkers wires the cadence scheduler, source poller, supervisor,
// and TTL sweep when NSQ env vars are set. Returns the server options to wire
// (e.g. the bootstrap-on-source-create hook) and a shutdown fn. Without NSQ
// the HTTP API still serves and the returned options slice is empty.
func maybeStartWorkers(ctx context.Context, pool *pgxpool.Pool) ([]server.Option, func()) {
	nsqdAddr := os.Getenv("NSQD_TCP_ADDR")
	lookupdAddr := os.Getenv("NSQLOOKUPD_HTTP_ADDR")
	if nsqdAddr == "" || lookupdAddr == "" {
		log.Printf("workers: NSQD_TCP_ADDR/NSQLOOKUPD_HTTP_ADDR unset, background workers disabled")
		return nil, func() {}
	}

	prod, err := nsqx.NewProducer(nsqdAddr)
	if err != nil {
		log.Fatalf("workers: producer: %v", err)
	}

	// Cadence scheduler.
	issueStore := issues.NewStore(pool)
	sched := cadence.NewScheduler(pool, prod, issueStore)
	schedConsumer, err := nsqx.Subscribe(lookupdAddr, cadence.TickTopic, "scheduler",
		sched.HandleTick, nsqx.ConsumerOpts{MaxInFlight: 1})
	if err != nil {
		log.Fatalf("workers: subscribe cadence: %v", err)
	}
	if err := sched.Bootstrap(); err != nil {
		log.Fatalf("workers: bootstrap cadence: %v", err)
	}

	// Source poller.
	candStore := candidates.NewStore(pool)
	feed := rss.New()
	reg := sourceadapter.NewRegistry()
	reg.Register(sources.TypeRSS, feed)
	reg.Register(sources.TypeSubstack, substack.New(feed))
	reg.Register(sources.TypeYouTubeChannel, youtube.New(feed))
	reg.Register(sources.TypeWeb, web.New(feed))
	reg.Register(sources.TypeXHandle, xhandle.New())
	poller := sourceadapter.NewPoller(pool, reg, candStore, prod)
	pollerConsumer, err := nsqx.Subscribe(lookupdAddr, sourceadapter.PollTopic, "poller",
		poller.HandleMessage, nsqx.ConsumerOpts{MaxInFlight: 4})
	if err != nil {
		log.Fatalf("workers: subscribe poller: %v", err)
	}

	// Supervisor — bootstrap any overdue sources at startup.
	sup := sourceadapter.NewSupervisor(pool, poller)
	if err := sup.RunOnce(ctx); err != nil {
		log.Printf("workers: supervisor: %v", err)
	}

	// TTL sweep.
	sweepStop := startTTLSweep(ctx, candStore)

	log.Printf("workers: running (cadence=%s, poll=%s)", cadence.TickTopic, sourceadapter.PollTopic)

	// Hook the HTTP handler to bootstrap polling immediately on Source create.
	opts := []server.Option{
		server.WithSourcePostCreateHook(func(id uuid.UUID) {
			if err := poller.Bootstrap(id); err != nil {
				log.Printf("source post-create bootstrap %s: %v", id, err)
			}
		}),
	}

	return opts, func() {
		schedConsumer.Stop()
		pollerConsumer.Stop()
		sweepStop()
		prod.Stop()
	}
}

func startTTLSweep(ctx context.Context, store *candidates.Store) func() {
	tick := time.NewTicker(1 * time.Hour)
	done := make(chan struct{})
	go func() {
		if n, err := store.ExpireOlderThan(ctx, time.Now()); err != nil {
			log.Printf("ttl sweep: %v", err)
		} else if n > 0 {
			log.Printf("ttl sweep: expired %d candidates", n)
		}
		for {
			select {
			case <-tick.C:
				if n, err := store.ExpireOlderThan(ctx, time.Now()); err != nil {
					log.Printf("ttl sweep: %v", err)
				} else if n > 0 {
					log.Printf("ttl sweep: expired %d candidates", n)
				}
			case <-done:
				tick.Stop()
				return
			}
		}
	}()
	return func() { close(done) }
}
