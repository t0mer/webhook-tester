# Raptor

**Self-hosted webhook, email & DNS capture and inspection.** Spin up instant,
unique URLs that capture, inspect and (in later phases) transform, automate and
forward inbound HTTP requests, emails and DNS queries — all from a single static
binary with an embedded UI.

> Raptor is built in reviewable phases. **Phases 1–3 (current)** deliver the core
> HTTP capture engine, inspection API, a real-time web inbox, request search,
> groups, a control panel, and **inbound email + DNS capture**. Custom actions,
> schedules and accounts land in later phases (see [Roadmap](#roadmap)).

## Screenshots

### Inbox — light & dark

| Light | Dark |
| --- | --- |
| ![Inbox, light theme](assets/screenshots/inbox-light.png) | ![Inbox, dark theme](assets/screenshots/inbox-dark.png) |

### Search & Control Panel

| Search DSL | Control Panel |
| --- | --- |
| ![Filtering requests with the search DSL](assets/screenshots/search-dark.png) | ![Control Panel managing URLs and groups](assets/screenshots/control-panel-dark.png) |

### Email & DNS capture

| Email (rendered + auth checks) | DNS query |
| --- | --- |
| ![Captured email with sandboxed HTML body and DKIM/SPF/DMARC results](assets/screenshots/email-detail-dark.png) | ![Captured DNS query detail](assets/screenshots/dns-detail-light.png) |

### Mobile (responsive)

| Light | Dark |
| --- | --- |
| ![Mobile, light theme](assets/screenshots/mobile-light.png) | ![Mobile, dark theme](assets/screenshots/mobile-dark.png) |

## Features (Phase 1)

- **Instant capture URLs** — every method on any sub-path is recorded:
  `POST /{token}/any/path`, alias URLs, and a `/{token}/{statusCode}` form to
  force a response status for retry testing.
- **Full request inspection** — method, headers, query, body, client IP, host
  and user-agent, all captured and rendered.
- **Real-time inbox** — new requests stream into the UI instantly over
  Server-Sent Events, with a 60-second polled fallback.
- **Configurable default response** — status, body, content-type, permissive
  CORS, and redirect, all editable per URL.
- **Guardrails** — per-URL `100 ÷ timeout` rpm rate limit, `request_limit`
  ring-buffer, body-size cap and TTL `expiry`.
- **Modern UI** — React + TypeScript SPA embedded in the binary, system-aware
  light/dark theme with a persisted toggle, responsive from phone to desktop.
- **REST API first** — every UI action maps to a documented `/api/v1` endpoint;
  interactive Swagger UI at `/api/docs`.
- **Operations** — CSV export of captured requests, Prometheus metrics at
  `/metrics`, JSON health at `/health`.

### Phase 2 — Request management

- **Search DSL** — filter the inbox with a Lucene-style query: free text matches
  the body, plus `method:POST`, `type:web`, `content:charge`, `ip:`, `host:`,
  `url:`, `query:`, `headers.x-event:push`, `_exists_:custom_action_errors`, and
  date ranges like `created_at:[* TO now-14d]`. Terms are ANDed.
- **Subset delete** — delete only the requests matching a search query or
  `date_from`/`date_to` window, or clear everything.
- **Groups** — organise URLs into colour-coded groups; the sidebar buckets them
  and a group can be deleted without losing its URLs.
- **Control Panel** — manage every URL and group from one table: reassign groups,
  open or delete URLs, and create/delete groups.

### Phase 3 — Email & DNS capture

- **Inbound email** — an SMTP server captures mail sent to
  `{token}@{email-domain}`. Messages are MIME-parsed (subject, sender, HTML and
  plain bodies, **attachments**), and **DKIM/SPF/DMARC** are evaluated and shown
  as badges. HTML bodies render in a fully sandboxed iframe (no script execution).
- **Inbound DNS** — a DNS server (UDP+TCP) captures queries for
  `{token}.{dns-domain}` and any subdomain, recording the query name, type and
  client IP, and returns a minimal answer.
- Email and DNS captures share the inbox, search, SSE stream and CSV export with
  HTTP requests — filter them with `type:email` / `type:dns`.

#### Exposing email & DNS

The SMTP (`2525`) and DNS (`5354`) listeners default to unprivileged ports. To
accept real mail/queries on the standard ports, put a reverse proxy or port
forward in front (`25 → 2525`, `53 → 5354`), and point DNS at your host:

- **Email:** add an MX record for `emailhook.site` → your host.
- **DNS:** delegate `dnshook.site` with an NS record pointing at your host, so
  all `*.dnshook.site` queries reach Raptor.

Override the suffixes with `--email-domain` / `--dns-domain` to use your own.

## Quick start

### Docker Compose

```bash
docker compose up --build
# UI + API + capture: http://localhost:8084
```

### Docker

```bash
docker run -p 8084:8084 -v "$PWD/data:/data" \
  -e RAPTOR_BASE_URL=http://localhost:8084 techblog/raptor:latest
```

### From source

```bash
cd web && npm ci && npm run build && cd ..   # build & embed the UI
go build -o raptor ./cmd/raptor
./raptor --base-url http://localhost:8084 --data ./data
```

Then open <http://localhost:8084>, click **New URL**, and send a request to the
generated address:

```bash
curl -X POST http://localhost:8084/<token>/demo \
  -H 'Content-Type: application/json' -d '{"hello":"world"}'
```

It appears in the inbox instantly.

## CLI flags

| Flag | Env | Default | Purpose |
| --- | --- | --- | --- |
| `--port` | `RAPTOR_PORT` | `8084` | HTTP port (app + capture + API) |
| `--smtp-port` | `RAPTOR_SMTP_PORT` | `2525` | Inbound email listener (Phase 3) |
| `--dns-port` | `RAPTOR_DNS_PORT` | `5354` | Inbound DNS listener (Phase 3) |
| `--data` | `RAPTOR_DATA` | `/data` | SQLite + uploaded files directory |
| `--db-driver` | `RAPTOR_DB_DRIVER` | `sqlite` | `sqlite` \| `postgres` |
| `--db-dsn` | `RAPTOR_DB_DSN` | — | Postgres DSN when `--db-driver=postgres` |
| `--base-url` | `RAPTOR_BASE_URL` | `http://localhost:8084` | External base URL for copyable links |
| `--email-domain` | `RAPTOR_EMAIL_DOMAIN` | `emailhook.site` | Inbound email suffix (Phase 3) |
| `--dns-domain` | `RAPTOR_DNS_DOMAIN` | `dnshook.site` | Inbound DNS suffix (Phase 3) |
| `--max-requests` | `RAPTOR_MAX_REQUESTS` | `0` | Per-URL stored-request cap (`0` = unlimited) |
| `--geoip-db` | `RAPTOR_GEOIP_DB` | — | Optional MaxMind GeoLite2 DB for request geo |
| `--log-level` | `RAPTOR_LOG_LEVEL` | `info` | `debug` \| `info` \| `warning` \| `error` |
| `--require-auth` | `RAPTOR_REQUIRE_AUTH` | `false` | Gate the management API behind an API key (Phase 6) |
| `--version` | — | — | Print version and exit |

**Precedence:** environment variable → `--flag` → built-in default (an env var,
when set, overrides the flag).

## API

The management API is versioned under `/api/v1` and is the contract the UI
consumes. The spec-first source of truth is [`openapi.yaml`](openapi.yaml),
served with an embedded **Swagger UI at `/api/docs`** (works fully offline).

Key endpoints:

| Method | Path | Description |
| --- | --- | --- |
| `POST` | `/api/v1/tokens` | Create a capture URL |
| `GET` | `/api/v1/tokens` | List URLs |
| `PUT` / `DELETE` | `/api/v1/tokens/{id}` | Update / delete a URL |
| `GET` | `/api/v1/tokens/{id}/requests` | List captured requests (paged; `q`/`date_from`/`date_to` search) |
| `DELETE` | `/api/v1/tokens/{id}/requests` | Delete all, or a `q`/date subset |
| `GET` | `/api/v1/tokens/{id}/requests/latest` | Most recent request |
| `GET` | `/api/v1/tokens/{id}/requests/{rid}/raw` | Raw request text |
| `GET` | `/api/v1/tokens/{id}/requests.csv` | CSV export |
| `GET` | `/api/v1/tokens/{id}/stream` | SSE stream of new requests |
| `GET` `POST` | `/api/v1/groups` | List / create groups |
| `PUT` `DELETE` | `/api/v1/groups/{id}` | Update / delete a group |

When `--require-auth` is set, send `Api-Key: <uuid>`. With no key configured the
API is open (documented first-run bootstrap mode).

## Development

```bash
./scripts/dev.sh    # backend (:8084) + Vite dev server (:5173) with hot reload
go test ./...       # backend tests
cd web && npm run build   # produce the embedded UI bundle
```

The frontend lives in [`web/`](web) (React + TypeScript + Vite); its build output
is embedded into the Go binary via `embed.FS`, so production ships a single file.

## Roadmap

| Phase | Scope |
| --- | --- |
| **1 — Core capture** *(done)* | URLs, HTTP capture, inspection API, real-time SPA inbox, default responses, CSV, metrics |
| **2 — Response control** *(done)* | Request search DSL, subset delete, groups, control panel |
| **3 — Email + DNS** *(done)* | Inbound SMTP (`@emailhook`) capture with DKIM/SPF/DMARC, inbound DNS (`.dnshook`) capture |
| 4 — Custom Actions | Action chain engine, variables, conditions, scripting |
| 5 — Schedules & replay | Cron schedules, alerting, request replay, CLI forwarding |
| 6 — Accounts & org | API keys, multi-user, SAML SSO, custom domains |

## License

[Apache-2.0](LICENSE).
