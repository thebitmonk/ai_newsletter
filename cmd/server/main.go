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

	"github.com/thebitmonk/ai_newsletter/internal/blobstore"
	"github.com/thebitmonk/ai_newsletter/internal/cadence"
	"github.com/thebitmonk/ai_newsletter/internal/candidates"
	"github.com/thebitmonk/ai_newsletter/internal/curation"
	"github.com/thebitmonk/ai_newsletter/internal/db"
	"github.com/thebitmonk/ai_newsletter/internal/firebaseauth"
	"github.com/thebitmonk/ai_newsletter/internal/imagegen"
	"github.com/thebitmonk/ai_newsletter/internal/issues"
	"github.com/thebitmonk/ai_newsletter/internal/llmclient"
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

	verifier, err := firebaseauth.NewFromEnv(ctx)
	if err != nil {
		log.Fatalf("firebaseauth: %v", err)
	}

	serverOpts, stopWorkers := maybeStartWorkers(ctx, pool)
	defer stopWorkers()
	serverOpts = append(serverOpts, server.WithTokenVerifier(verifier))

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
// TTL sweep, and curation worker when NSQ env vars are set. Without NSQ the
// HTTP API still serves and background processing is skipped. Returns the
// server options to wire and a shutdown fn.
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

	issueStore := issues.NewStore(pool)

	// Cadence scheduler.
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

	sup := sourceadapter.NewSupervisor(pool, poller)
	if err := sup.RunOnce(ctx); err != nil {
		log.Printf("workers: supervisor: %v", err)
	}

	sweepStop := startTTLSweep(ctx, candStore)

	// Curation worker (LLM + image + R2). curationWorker is non-nil when the
	// LLM + R2 env vars are present; it doubles as the issuesapi Regenerator
	// (synchronous Story/cover regeneration shares the same dependencies).
	curationConsumer, curationWorker, stopCuration := maybeStartCuration(ctx, pool, issueStore, candStore, lookupdAddr)

	log.Printf("workers: running (cadence=%s, poll=%s, curation=%s)",
		cadence.TickTopic, sourceadapter.PollTopic, curation.StartTopic)

	opts := []server.Option{
		server.WithSourcePostCreateHook(func(id uuid.UUID) {
			if err := poller.Bootstrap(id); err != nil {
				log.Printf("source post-create bootstrap %s: %v", id, err)
			}
		}),
		server.WithCurateTrigger(func(id uuid.UUID) error {
			return curation.Trigger(prod, id)
		}),
	}
	if curationWorker != nil {
		opts = append(opts, server.WithRegenerator(curationWorker))
	}

	return opts, func() {
		schedConsumer.Stop()
		pollerConsumer.Stop()
		if curationConsumer != nil {
			curationConsumer.Stop()
		}
		stopCuration()
		sweepStop()
		prod.Stop()
	}
}

// maybeStartCuration spins up the curation worker if all its required env
// vars are present. Missing OPENAI_API_KEY / R2_* are tolerated — the manual
// trigger endpoint still works but no consumer is processing the messages
// (they'll queue up). This is intentional for local dev so the rest of the
// system runs without paid creds.
func maybeStartCuration(ctx context.Context, pool *pgxpool.Pool, is *issues.Store, cs *candidates.Store, lookupdAddr string) (*nsqx.Consumer, *curation.Worker, func()) {
	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Printf("curation: OPENAI_API_KEY unset, worker disabled (manual trigger will still enqueue)")
		return nil, nil, func() {}
	}

	r2Cfg, err := blobstore.LoadR2ConfigFromEnv()
	if err != nil {
		log.Printf("curation: R2 env missing, worker disabled: %v", err)
		return nil, nil, func() {}
	}
	r2, err := blobstore.NewR2(ctx, r2Cfg)
	if err != nil {
		log.Fatalf("curation: r2 init: %v", err)
	}

	llm, err := llmclient.NewFromEnv()
	if err != nil {
		log.Fatalf("curation: llm: %v", err)
	}
	ig, err := imagegen.NewFromEnv(r2)
	if err != nil {
		log.Fatalf("curation: imagegen: %v", err)
	}

	worker := curation.NewWorker(pool, is, cs, llm, llm, ig)
	consumer, err := nsqx.Subscribe(lookupdAddr, curation.StartTopic, "curator",
		worker.HandleMessage, nsqx.ConsumerOpts{MaxInFlight: 2})
	if err != nil {
		log.Fatalf("curation: subscribe: %v", err)
	}
	return consumer, worker, func() {}
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
