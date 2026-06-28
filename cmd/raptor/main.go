// Command raptor is a self-hosted webhook/email/DNS capture and inspection
// server. See CLAUDE.md for the design contract.
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

	"github.com/t0mer/raptor/internal/actions"
	"github.com/t0mer/raptor/internal/auth"
	"github.com/t0mer/raptor/internal/capture"
	"github.com/t0mer/raptor/internal/config"
	"github.com/t0mer/raptor/internal/crypto"
	dnssrv "github.com/t0mer/raptor/internal/dns"
	"github.com/t0mer/raptor/internal/email"
	"github.com/t0mer/raptor/internal/netguard"
	"github.com/t0mer/raptor/internal/schedules"
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

	// Secrets-at-rest: load (or create) the AES-256-GCM key and enable
	// transparent encryption of secret columns (notify URLs, etc.).
	key, err := crypto.LoadOrCreateKey(filepath.Join(cfg.Data, "secret.key"))
	if err != nil {
		return fmt.Errorf("load encryption key: %w", err)
	}
	cipher, err := crypto.New(key)
	if err != nil {
		return fmt.Errorf("init cipher: %w", err)
	}
	st.SetCipher(cipher)

	hub := sse.NewHub()
	// One SSRF guard shared by actions, replay and schedule monitoring.
	guard := netguard.New(cfg.ActionAllow, cfg.ActionDeny, cfg.ActionAllowInternal)
	engine := actions.New(actions.WithGuard(guard))
	actionsSvc := actions.NewService(engine, st)

	capturer := capture.New(st, cfg.BaseURL,
		capture.WithGlobalRequestLimit(cfg.MaxRequests),
		capture.WithPublisher(hub),
		capture.WithFilesDir(filepath.Join(cfg.Data, "files")),
		capture.WithActions(actionsSvc),
	)
	authSvc := auth.NewService(st)

	// --reset-password: set an admin password interactively and exit.
	if cfg.ResetPassword {
		return resetPassword(st, authSvc)
	}

	// Seed an initial admin from the environment on first run (optional).
	if seeded, err := authSvc.SeedAdminIfEmpty(context.Background(),
		os.Getenv("RAPTOR_ADMIN_EMAIL"), os.Getenv("RAPTOR_ADMIN_PASSWORD")); err != nil {
		return fmt.Errorf("seed admin: %w", err)
	} else if seeded {
		logger.Info("seeded admin user from environment")
	}

	scheduleRunner := schedules.New(st, actionsSvc, schedules.WithLogger(logger), schedules.WithGuard(guard))
	srv := server.New(cfg, st, capturer, hub, actionsSvc, scheduleRunner, guard, authSvc)

	httpSrv := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	emailSrv := email.New(capturer, cfg.EmailDomain, email.WithLogger(logger))
	dnsSrv := dnssrv.New(capturer, cfg.DNSDomain, dnssrv.WithLogger(logger))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// HTTP listen failure is fatal; email/DNS failures are logged but do not
	// take down the primary HTTP capture service.
	errc := make(chan error, 1)
	go func() {
		logger.Info("raptor listening",
			"version", version.Version, "addr", httpSrv.Addr, "base_url", cfg.BaseURL)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
	}()
	go func() {
		if err := emailSrv.ListenAndServe(":" + strconv.Itoa(cfg.SMTPPort)); err != nil {
			logger.Error("smtp server stopped", "error", err)
		}
	}()
	go func() {
		if err := dnsSrv.ListenAndServe(":" + strconv.Itoa(cfg.DNSPort)); err != nil {
			logger.Error("dns server stopped", "error", err)
		}
	}()
	go scheduleRunner.Start(ctx)

	select {
	case err := <-errc:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = emailSrv.Shutdown(shutdownCtx)
		_ = dnsSrv.Shutdown(shutdownCtx)
		return httpSrv.Shutdown(shutdownCtx)
	}
}

func newLogger(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.SlogLevel()}
	return slog.New(slog.NewTextHandler(os.Stderr, opts))
}
