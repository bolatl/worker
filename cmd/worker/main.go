// Command worker: consumes jobs from RabbitMQ, processes them, and runs the reaper for stuck jobs.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"worker/internal/config"
	"worker/internal/db"
	"worker/internal/jobs"
	"worker/internal/queue"
	"worker/internal/worker"
)

// main loads config, connects to DB and RabbitMQ, runs migrations, starts consumer and reaper, and handles shutdown.
func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	// Optional: you may let only API run migrations, but running here is okay too.
	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatal(err)
	}

	rabbit, err := queue.Connect(cfg.RabbitURL, cfg.QueueName)
	if err != nil {
		log.Fatal(err)
	}
	defer rabbit.Close()

	deliveries, err := rabbit.Consume(cfg.Prefetch)
	if err != nil {
		log.Fatal(err)
	}

	repo := jobs.NewRepository(pool)
	proc := worker.NewProcessor(repo, cfg.WorkDuration)
	cons := worker.NewConsumer(deliveries, proc)

	reaper := worker.NewReaper(repo, cfg.ProcessingTimeout)
	go reaper.Run(ctx)

	go func() {
		log.Printf("worker started (prefetch=%d)", cfg.Prefetch)
		if err := cons.Run(ctx); err != nil && err != context.Canceled {
			log.Printf("consumer stopped: %v", err)
		}
	}()

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	cancel()
	time.Sleep(1 * time.Second)
}
