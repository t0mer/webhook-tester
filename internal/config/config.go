// Package config resolves Raptor's runtime configuration from command-line
// flags and environment variables.
//
// Precedence follows the house convention (AGHSync model):
//
//	environment variable  >  --flag  >  built-in default
//
// i.e. an environment variable, when set, overrides the corresponding flag.
// Every flag has a RAPTOR_-prefixed environment counterpart.
package config

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	flag "github.com/spf13/pflag"
)

// Config holds the fully resolved runtime configuration.
type Config struct {
	Port        int    // --port / RAPTOR_PORT
	SMTPPort    int    // --smtp-port / RAPTOR_SMTP_PORT
	DNSPort     int    // --dns-port / RAPTOR_DNS_PORT
	Data        string // --data / RAPTOR_DATA
	DBDriver    string // --db-driver / RAPTOR_DB_DRIVER
	DBDSN       string // --db-dsn / RAPTOR_DB_DSN
	BaseURL     string // --base-url / RAPTOR_BASE_URL
	EmailDomain string // --email-domain / RAPTOR_EMAIL_DOMAIN
	DNSDomain   string // --dns-domain / RAPTOR_DNS_DOMAIN
	MaxRequests int    // --max-requests / RAPTOR_MAX_REQUESTS
	GeoIPDB     string // --geoip-db / RAPTOR_GEOIP_DB
	LogLevel    string // --log-level / RAPTOR_LOG_LEVEL
	RequireAuth bool   // --require-auth / RAPTOR_REQUIRE_AUTH

	// Action SSRF lists (comma-separated host suffixes / CIDRs) for
	// http_request/script actions. ActionAllow, when set, is a strict allow-list.
	ActionAllow         []string // --action-allow / RAPTOR_ACTION_ALLOW
	ActionDeny          []string // --action-deny / RAPTOR_ACTION_DENY
	ActionAllowInternal bool     // --action-allow-internal / RAPTOR_ACTION_ALLOW_INTERNAL

	// ShowVersion is true when --version was passed; the caller prints the
	// version and exits.
	ShowVersion bool

	// ResetPassword is true when --reset-password was passed; the caller runs
	// the interactive admin password reset and exits.
	ResetPassword bool
}

// Defaults returns a Config populated with the built-in default values.
func Defaults() Config {
	return Config{
		Port:        8084,
		SMTPPort:    2525,
		DNSPort:     5354,
		Data:        "/data",
		DBDriver:    "sqlite",
		BaseURL:     "http://localhost:8084",
		EmailDomain: "emailhook.site",
		DNSDomain:   "dnshook.site",
		MaxRequests: 0,
		LogLevel:    "info",
		RequireAuth: false,
	}
}

// Load parses the given argument slice and resolves the configuration, applying
// environment overrides via getenv (which behaves like os.Getenv). Passing the
// real os.Args[1:] and os.Getenv wires it to the process environment.
func Load(args []string, getenv func(string) string) (Config, error) {
	cfg := Defaults()

	fs := flag.NewFlagSet("raptor", flag.ContinueOnError)
	fs.IntVar(&cfg.Port, "port", cfg.Port, "HTTP listen port (app + capture + API)")
	fs.IntVar(&cfg.SMTPPort, "smtp-port", cfg.SMTPPort, "inbound email listener port")
	fs.IntVar(&cfg.DNSPort, "dns-port", cfg.DNSPort, "inbound DNS listener port (UDP+TCP)")
	fs.StringVar(&cfg.Data, "data", cfg.Data, "data directory (SQLite + uploaded files)")
	fs.StringVar(&cfg.DBDriver, "db-driver", cfg.DBDriver, "database driver: sqlite | postgres")
	fs.StringVar(&cfg.DBDSN, "db-dsn", cfg.DBDSN, "Postgres DSN when --db-driver=postgres")
	fs.StringVar(&cfg.BaseURL, "base-url", cfg.BaseURL, "external base URL for copyable links")
	fs.StringVar(&cfg.EmailDomain, "email-domain", cfg.EmailDomain, "inbound email suffix")
	fs.StringVar(&cfg.DNSDomain, "dns-domain", cfg.DNSDomain, "inbound DNS suffix")
	fs.IntVar(&cfg.MaxRequests, "max-requests", cfg.MaxRequests, "max stored requests per token (0 = unlimited)")
	fs.StringVar(&cfg.GeoIPDB, "geoip-db", cfg.GeoIPDB, "optional MaxMind GeoLite2 DB path for request geo")
	fs.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "log level: debug | info | warning | error")
	fs.BoolVar(&cfg.RequireAuth, "require-auth", cfg.RequireAuth, "gate the management API behind an API key")
	allow := fs.String("action-allow", "", "comma-separated allow-list of hosts for outbound actions")
	deny := fs.String("action-deny", "", "comma-separated deny-list of hosts for outbound actions")
	fs.BoolVar(&cfg.ActionAllowInternal, "action-allow-internal", false, "permit outbound actions to reach internal/loopback hosts")
	fs.BoolVar(&cfg.ResetPassword, "reset-password", false, "interactively set an admin password and exit")
	fs.BoolVarP(&cfg.ShowVersion, "version", "v", false, "print version and exit")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	if v := getenv("RAPTOR_ACTION_ALLOW"); v != "" {
		*allow = v
	}
	if v := getenv("RAPTOR_ACTION_DENY"); v != "" {
		*deny = v
	}
	if err := envBool(getenv, "RAPTOR_ACTION_ALLOW_INTERNAL", &cfg.ActionAllowInternal); err != nil {
		return Config{}, err
	}
	cfg.ActionAllow = splitList(*allow)
	cfg.ActionDeny = splitList(*deny)

	// Apply environment overrides (env wins over flags per house convention).
	if err := applyEnv(&cfg, getenv); err != nil {
		return Config{}, err
	}

	if !validLogLevel(cfg.LogLevel) {
		return Config{}, fmt.Errorf("invalid --log-level %q: want debug|info|warning|error", cfg.LogLevel)
	}
	if cfg.DBDriver != "sqlite" && cfg.DBDriver != "postgres" {
		return Config{}, fmt.Errorf("invalid --db-driver %q: want sqlite|postgres", cfg.DBDriver)
	}

	return cfg, nil
}

func applyEnv(cfg *Config, getenv func(string) string) error {
	if err := envInt(getenv, "RAPTOR_PORT", &cfg.Port); err != nil {
		return err
	}
	if err := envInt(getenv, "RAPTOR_SMTP_PORT", &cfg.SMTPPort); err != nil {
		return err
	}
	if err := envInt(getenv, "RAPTOR_DNS_PORT", &cfg.DNSPort); err != nil {
		return err
	}
	if err := envInt(getenv, "RAPTOR_MAX_REQUESTS", &cfg.MaxRequests); err != nil {
		return err
	}
	if err := envBool(getenv, "RAPTOR_REQUIRE_AUTH", &cfg.RequireAuth); err != nil {
		return err
	}
	envStr(getenv, "RAPTOR_DATA", &cfg.Data)
	envStr(getenv, "RAPTOR_DB_DRIVER", &cfg.DBDriver)
	envStr(getenv, "RAPTOR_DB_DSN", &cfg.DBDSN)
	envStr(getenv, "RAPTOR_BASE_URL", &cfg.BaseURL)
	envStr(getenv, "RAPTOR_EMAIL_DOMAIN", &cfg.EmailDomain)
	envStr(getenv, "RAPTOR_DNS_DOMAIN", &cfg.DNSDomain)
	envStr(getenv, "RAPTOR_GEOIP_DB", &cfg.GeoIPDB)
	envStr(getenv, "RAPTOR_LOG_LEVEL", &cfg.LogLevel)
	return nil
}

func envStr(getenv func(string) string, key string, dst *string) {
	if v := getenv(key); v != "" {
		*dst = v
	}
}

func envInt(getenv func(string) string, key string, dst *int) error {
	v := getenv(key)
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fmt.Errorf("invalid %s=%q: %w", key, v, err)
	}
	*dst = n
	return nil
}

func envBool(getenv func(string) string, key string, dst *bool) error {
	v := getenv(key)
	if v == "" {
		return nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fmt.Errorf("invalid %s=%q: %w", key, v, err)
	}
	*dst = b
	return nil
}

// splitList parses a comma-separated list, trimming spaces and empties.
func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func validLogLevel(level string) bool {
	switch strings.ToLower(level) {
	case "debug", "info", "warning", "warn", "error":
		return true
	default:
		return false
	}
}

// SlogLevel maps the configured log level string onto a slog.Level.
func (c Config) SlogLevel() slog.Level {
	switch strings.ToLower(c.LogLevel) {
	case "debug":
		return slog.LevelDebug
	case "warning", "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
