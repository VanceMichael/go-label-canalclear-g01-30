package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/config"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/httpapi"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/store"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/worker"
)

func main() {
	if err := run(); err != nil {
		slog.Error("canalclear stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	db, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Bootstrap(ctx); err != nil {
		return err
	}
	go (worker.Outbox{DB: db, Publisher: worker.LogPublisher{}, Batch: 20, Policy: worker.DefaultRetryPolicy()}).Run(ctx, cfg.WorkerEvery)
	go func() {
		_ = (worker.SessionReaper{DB: db, Retention: 30 * 24 * time.Hour}).Run(ctx, time.Hour)
	}()
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: httpapi.New(db, cfg.SessionTTL).Handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_ = server.Shutdown(shutdown)
	}()
	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
