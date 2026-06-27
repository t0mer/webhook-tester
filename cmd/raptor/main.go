// Command raptor is a self-hosted webhook/email/DNS capture and inspection
// server — a Go rewrite of webhook.site. See CLAUDE.md for the design contract.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	flag "github.com/spf13/pflag"

	"github.com/t0mer/raptor/internal/capture"
	"github.com/t0mer/raptor/internal/config"
	"github.com/t0mer/raptor/internal/server"
	"github.com/t0mer/raptor/internal/sse"
	"github.com/t0mer/raptor/internal/store"
	"github.com/t0mer/raptor/internal/version"
)

func main() {
	cfg, err := config.Load(os.Args[1:], os.Getenv)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return // pflag already printed usage
		}
		fmt.Fprintln(os.Stderr, "raptor:", err)
		os.Exit(2)
	}

	if cfg.ShowVersion {
		fmt.Println(version.Version)
		return
	}

	logger := newLogger(cfg)
	slog.SetDefault(logger)

	if err := run(cfg, logger); err != nil {
		logger.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(cfg config.Config, logger *slog.Logger) error {
	if err := os.MkdirAll(cfg.Data, 0o750); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	if cfg.DBDriver != "sqlite" {
		return fmt.Errorf("db driver %q not yet supported", cfg.DBDriver)
	}
	st, err := store.Open(filepath.Join(cfg.Data, "raptor.db"))
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	hub := sse.NewHub()
	capturer := capture.New(st, cfg.BaseURL,
		capture.WithGlobalRequestLimit(cfg.MaxRequests),
		capture.WithPublisher(hub),
	)
	srv := server.New(cfg, st, capturer, hub)

	httpSrv := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errc := make(chan error, 1)
	go func() {
		logger.Info("raptor listening",
			"version", version.Version, "addr", httpSrv.Addr, "base_url", cfg.BaseURL)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
	}()

	select {
	case err := <-errc:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpSrv.Shutdown(shutdownCtx)
	}
}

func newLogger(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.SlogLevel()}
	return slog.New(slog.NewTextHandler(os.Stderr, opts))
}
