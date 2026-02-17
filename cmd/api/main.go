package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"worker/internal/api"
	"worker/internal/config"
	"worker/internal/db"
	"worker/internal/jobs"
	"worker/internal/queue"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatal(err)
	}

	rabbit, err := queue.Connect(cfg.RabbitURL, cfg.QueueName)
	if err != nil {
		log.Fatal(err)
	}
	defer rabbit.Close()

	repo := jobs.NewRepository(pool)
	svc := jobs.NewService(repo, rabbit, cfg.MaxAttempts)
	handlers := api.NewHandlers(svc, repo)

	srv := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           api.Router(handlers),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("api listening on :%s", cfg.HTTPPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
