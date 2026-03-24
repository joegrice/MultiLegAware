# CLAUDE.md — MultiLegAware

> This file is read by Claude Code before any work begins. It provides full
> context so that a Claude agent can work safely and correctly on this codebase
> without needing to ask clarifying questions.

---

## 3.1 Project Overview

MultiLegAware is a lightweight Go microservice that fires on a configurable cron
schedule, queries the TfL (Transport for London) Unified API for multimodal
journey options between two fixed postcodes, and delivers the top three results
as formatted Telegram messages. It is designed to send a morning commute
summary (A → B) and an afternoon return summary (B → A) automatically each
weekday, without any manual interaction.

The two external APIs used are:

- **TfL Unified API** — provides journey planning results including legs,
  transport modes, durations, and human-readable instructions. Used read-only;
  no write operations are performed.
- **Telegram Bot API** — receives the formatted journey messages. The service
  acts as a bot and calls `sendMessage` for each journey result.

Primary deployment target: **Docker container managed by Coolify** running on a
VPS. The service is intentionally stateless; no database or persistent storage
is used.

---

## 3.2 Architecture

### Package Responsibilities

**`main` (`main.go`)**
Bootstraps the service: loads and validates all required environment variables
(failing fast with a clear message if any are missing), constructs the `tfl`
and `telegram` clients, wires the `runner.Runner`, registers both cron jobs,
starts the cron scheduler, and then blocks on a minimal HTTP server that exposes
only a `/health` endpoint. Graceful shutdown on SIGINT/SIGTERM is the
responsibility of this package.

> **Planned refactor:** Environment loading will move to `config/config.go` and
> cron registration will move to `scheduler/scheduler.go` once the codebase
> grows. Until then, both live in `main.go`. Follow the conventions in
> Section 3.7 when adding new variables or schedule logic.

**`tfl` (`tfl/client.go`)**
Owns all communication with the TfL Unified API. `tfl.Client` has a single
exported method, `GetJourneys`, which URL-encodes the from/to postcodes, fires
an HTTP GET with a 10-second context timeout, decodes the JSON response, and
returns a slice of typed `Journey` structs. Only the fields the service actually
uses are mapped; the rest of the TfL response is intentionally ignored.

**`telegram` (`telegram/client.go`)**
Owns all communication with the Telegram Bot API. `telegram.Client` exposes
`SendMessage` (sends a single MarkdownV2 message to a chat ID) and
`SendNoJourneysMessage` (sends a canned "no journeys found" error). The package
also contains the mode → emoji mapping, the MarkdownV2 escape function, and
`FormatJourneys`, which formats all journeys for a run into a single
ready-to-send string with a header showing origin, destination, and search time.

**`runner` (`runner/runner.go`)**
Orchestrates a single journey lookup-and-dispatch cycle. `runner.Runner.Run`
creates a fresh context with a 10-second timeout, calls `tfl.Client.GetJourneys`,
handles the zero-results case, then formats all results into a single message
via `telegram.FormatJourneys` and sends it with one `telegram.Client.SendMessage`
call. All errors are logged; `Run` has no return value because it is invoked
from the cron scheduler where there is no caller to surface errors to.

> **Planned package:** When the on-demand HTTP endpoint is added, a
> `handler/journey.go` will own the HTTP-layer orchestration, reusing `runner`
> for the core pipeline and an `apiKeyMiddleware` for authentication. Do not
> duplicate the pipeline logic — call `runner.Runner.Run` from the handler.

### Request Lifecycle

**(a) Scheduled trigger (currently implemented)**

```
cron scheduler fires
  └─► runner.Runner.Run(from, to)
        ├─► tfl.Client.GetJourneys(ctx, from, to, 3)
        │     └─► GET https://api.tfl.gov.uk/Journey/JourneyResults/{from}/to/{to}
        ├─► [0 journeys] telegram.Client.SendNoJourneysMessage
        └─► [n journeys] telegram.FormatJourneys(from, to, searchedAt, journeys)
              └─► telegram.Client.SendMessage(ctx, chatID, text)
                    └─► POST https://api.telegram.org/bot{TOKEN}/sendMessage
```

**(b) On-demand HTTP trigger (planned — not yet implemented)**

```
POST /journey
  └─► apiKeyMiddleware (crypto/subtle constant-time key comparison)
        └─► handler.JourneyHandler
              └─► runner.Runner.Run(from, to)   ← same pipeline as (a)
```

---

## 3.3 Development Commands

```bash
# Run locally (requires all env vars set — see Section 3.4)
go run .

# Run all tests
go test ./...

# Build binary
go build -o bin/journey-service .

# Docker build
docker build -t journey-service .

# Docker Compose — start detached
docker compose up -d

# Docker Compose — follow logs
docker compose logs -f journey-service

# Lint (requires golangci-lint installed)
golangci-lint run ./...
```

---

## 3.4 Environment Variables

| Variable              | Required | Default | Description                                                      |
|-----------------------|----------|---------|------------------------------------------------------------------|
| `TELEGRAM_BOT_TOKEN`  | yes      | —       | Bot token from @BotFather (e.g. `123456789:ABCdef...`)          |
| `TELEGRAM_CHAT_ID`    | yes      | —       | Telegram chat ID that receives journey messages                  |
| `TFL_APP_KEY`         | yes      | —       | TfL Unified API primary key from api-portal.tfl.gov.uk           |
| `ORIGIN`              | yes      | —       | Journey start for the morning run (e.g. `SW1A 1AA`)             |
| `DESTINATION`         | yes      | —       | Journey end for the morning run (e.g. `EC2V 8RT`)               |
| `MORNING_CRON`        | yes      | —       | 6-field cron expression for ORIGIN → DESTINATION                 |
| `AFTERNOON_CRON`      | yes      | —       | 6-field cron expression for DESTINATION → ORIGIN                 |
| `PORT`                | no       | `8080`  | HTTP port for the `/health` endpoint                             |

Cron field order (6 fields, seconds-first):
`second  minute  hour  day-of-month  month  day-of-week`

Example: `0 30 7 * * MON-FRI` = 07:30 every weekday.

Copy `.env.example` to `.env` and fill in real values before running locally.
Never commit `.env`.

---

## 3.5 Key Conventions & Constraints

**Single external dependency**
`github.com/robfig/cron/v3` is the only permitted external dependency. All
other functionality must use Go stdlib only. Do not add new modules without
explicit user approval.

**Error wrapping**
All errors must be wrapped with context: `fmt.Errorf("doing X: %w", err)`.
Never discard an error silently. In `runner.Run` (which has no return value),
log the error with `log.Printf("level=error ...")`.

**No secret logging**
`TELEGRAM_BOT_TOKEN`, `TFL_APP_KEY`, and any future API key values must never
appear in log output at any log level. Log their presence (`msg="token loaded"`)
but never their value.

**Constant-time key comparison**
When the on-demand HTTP endpoint is added, the API key check in
`apiKeyMiddleware` must use `subtle.ConstantTimeCompare` from `crypto/subtle`.
Never use `==` to compare secret values.

**Scheduler direction convention — do not change without updating docs**
`MORNING_CRON` always fires `ORIGIN → DESTINATION`.
`AFTERNOON_CRON` always fires `DESTINATION → ORIGIN`.
If this is ever reversed or parameterised, README.md and this file must both be
updated in the same commit.

**HTTP timeouts**
Every outbound HTTP call (TfL and Telegram) must use a `context.WithTimeout`
of exactly 10 seconds. Do not change this without considering cascading effects
on the cron job duration.

**Graceful shutdown**
`main.go` must handle `SIGINT` and `SIGTERM`: stop the cron scheduler (`c.Stop()`
returns a context you can wait on), then shut down the HTTP server with a 15-second
drain timeout. This is not yet implemented — when adding it, use
`http.Server.Shutdown(ctx)` and `signal.NotifyContext`.

**GoDoc comments**
All exported types and functions must have GoDoc comments. Unexported helpers do
not require comments unless the logic is non-obvious.

**Port**
The default port is `8080`. It is defined once, in `main.go`. Do not hardcode
`8080` anywhere else.

---

## 3.6 Testing Notes

There are currently no tests. This is known and acceptable for v1.0.0.

When tests are added, follow this layout:

| File                          | What to test                                           |
|-------------------------------|--------------------------------------------------------|
| `tfl/client_test.go`          | `GetJourneys` against a mock `httptest.Server`         |
| `telegram/client_test.go`     | `SendMessage`, `FormatJourneys`, `escapeMarkdownV2`    |
| `runner/runner_test.go`       | `Run` with stubbed TfL and Telegram clients            |
| `handler/journey_test.go`     | HTTP handler + `apiKeyMiddleware` via `httptest`       |

Use only the stdlib `testing` and `net/http/httptest` packages. Do not add
testify, gomock, or any other test framework.

---

## 3.7 Common Tasks

### Adding a new transport mode emoji
- Edit `telegram/client.go`, the `modeEmoji` function only.
- Add a new `case` to the `switch` statement.
- No other files need to change.

### Changing the Telegram message format
- Edit `telegram/client.go`, `FormatJourneys` function.
- The function signature is `FormatJourneys(from, to string, searchedAt time.Time, journeys []tfl.Journey) string`.
- It produces a single MarkdownV2 string: a bold `from → to` header, an italic
  search time, then each journey as a titled block with one bullet per leg.
- Verify output manually by sending a real message or writing a unit test.
- **MarkdownV2 caution:** every character in `_, *, [, ], (, ), ~, \`, >, #, +, -, =, |, {, }, ., !`
  must be escaped with a backslash in message text. Use the existing
  `escapeMarkdownV2` function for any user-derived or API-derived strings.
  Bold markers (`*...*`) and italic markers (`_..._`) must not be escaped.

### Adding a new environment variable
1. Add a `mustEnv` (required) or `os.Getenv` (optional) call in `main.go`.
   *(When `config/config.go` exists: add the field to the `Config` struct and
   its validation.)*
2. Add the variable to `.env.example` with a descriptive comment.
3. Add a row to the config table in `README.md`.
4. Add a row to the config table in this file (Section 3.4).

### Changing the cron schedule logic
- Edit the `c.AddFunc` calls in `main.go`.
  *(When `scheduler/scheduler.go` exists: edit that file only.)*
- Do not touch `runner/runner.go` unless the execution logic itself changes.
- Remember: morning = ORIGIN→DESTINATION, afternoon = DESTINATION→ORIGIN.

### Extending the TfL response parsing
- Edit `tfl/client.go` — add fields to the relevant struct(s) or add a helper.
- TfL API interactive docs: `https://api.tfl.gov.uk` (Swagger UI available).
- Run a real request with `curl` to inspect the raw JSON before adding fields.

### Adding a new HTTP endpoint
1. Create a new file in `handler/` (e.g. `handler/status.go`).
2. Register the route in `main.go` with `mux.Handle(...)`.
3. If the endpoint should be authenticated, import and apply
   `apiKeyMiddleware` from `handler/journey.go` — do not duplicate the middleware.

---

## 3.8 What NOT To Do

- **Do not add new external dependencies** without explicit user approval.
  go.mod must only contain `github.com/robfig/cron/v3` as a non-stdlib dep.

- **Do not log secret values.** Specifically: `TELEGRAM_BOT_TOKEN`,
  `TFL_APP_KEY`, and any future `API_KEY` variable. Log their presence or length
  if needed, never the value itself.

- **Do not change the Dockerfile base images** (`golang:1.22-alpine` builder,
  `scratch` final) without first confirming that `CGO_ENABLED=0` produces a
  fully static binary under the new base.

- **Do not add a database or persistent storage.** This service is intentionally
  stateless. Results are ephemeral; the scheduler re-fetches fresh data on every
  run.

- **Do not write real values into `.env.example`.** That file is committed to
  version control. It must contain only placeholder values (e.g. `your-bot-token`).

- **Do not add an HTTP framework** (Gin, Echo, Chi, Fiber, or similar).
  Use `net/http` from stdlib only.

- **Do not break the graceful shutdown path in `main.go`** when it is
  implemented. The cron scheduler's `c.Stop()` context and the HTTP server's
  `Shutdown` context must both be awaited before the process exits.

- **Do not remove the `/health` endpoint.** It is used by Docker's `HEALTHCHECK`
  directive and by Coolify's liveness probing.

---

## 3.9 Docker Compose Files

The repository contains two Compose files serving distinct purposes. Both can
run simultaneously on the same host without port conflicts.

### docker-compose.yml — production

- Reads secrets from `env_file: .env` — requires a fully populated `.env`
  before starting.
- Maps port `8080:8080`.
- `restart: unless-stopped` — the container recovers automatically after
  crashes or host reboots.
- Includes a `healthcheck` so Docker (and Coolify) can verify liveness.

```bash
docker compose up -d
```

### docker-compose.dev.yml — local development

- **Fully standalone** — no `extends`, no `env_file`. All variables are defined
  inline with placeholder values so the file is immediately runnable.
- Maps port `8081:8080` — avoids clashing with a production container on 8080.
- Service name is `journey-service-dev` — distinct from prod, so both can run
  at the same time.
- `restart: no` — fails fast during development rather than looping on bad config.
- No `healthcheck` — removes startup delay during iteration.

```bash
docker compose -f docker-compose.dev.yml up --build
```

### Running both simultaneously

```bash
# Terminal 1 — production
docker compose up -d

# Terminal 2 — dev (different port, different service name)
docker compose -f docker-compose.dev.yml up --build
```

Health checks:
- Production: `curl http://localhost:8080/health`
- Dev:        `curl http://localhost:8081/health`
