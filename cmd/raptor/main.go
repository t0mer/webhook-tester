// Command raptor is a self-hosted webhook/email/DNS capture and inspection
// server — a Go rewrite of webhook.site. See CLAUDE.md for the design contract.
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	flag "github.com/spf13/pflag"

	"github.com/t0mer/raptor/internal/config"
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

	logger.Info("raptor starting",
		"version", version.Version,
		"port", cfg.Port,
		"data", cfg.Data,
		"db_driver", cfg.DBDriver,
		"base_url", cfg.BaseURL,
	)

	// Server wiring (store, capture, API, SSE) is added in subsequent changes.
	logger.Warn("server not yet wired up; exiting")
}

func newLogger(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.SlogLevel()}
	return slog.New(slog.NewTextHandler(os.Stderr, opts))
}
