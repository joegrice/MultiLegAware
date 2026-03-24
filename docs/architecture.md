# Architecture & Design Decisions

---

## 1. System Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         MultiLegAware Service                           │
│                                                                         │
│  ┌─────────────────────────────────────────────┐                        │
│  │                  main.go                    │                        │
│  │  • Loads & validates env vars               │                        │
│  │  • Constructs clients                       │                        │
│  │  • Registers cron jobs                      │                        │
│  │  • Runs HTTP health server                  │                        │
│  └──────────────┬──────────────────────────────┘                        │
│                 │                                                        │
│        ┌────────▼────────┐                                              │
│        │  robfig/cron    │  ← MORNING_CRON / AFTERNOON_CRON             │
│        │  scheduler      │    (6-field, seconds-first)                  │
│        └────────┬────────┘                                              │
│                 │  fires on schedule                                    │
│                 ▼                                                        │
│        ┌─────────────────┐                                              │
│        │ runner.Runner   │  Orchestrates a single journey run           │
│        │ .Run(from, to)  │  Creates context (10s timeout)               │
│        └──────┬──────────┘                                              │
│               │                                                         │
│       ┌───────┴────────┐                                                │
│       │                │                                                │
│       ▼                ▼                                                │
│  ┌─────────┐    ┌──────────────┐                                        │
│  │   tfl   │    │   telegram   │                                        │
│  │ client  │    │   client     │                                        │
│  └────┬────┘    └──────┬───────┘                                        │
│       │                │                                                │
└───────┼────────────────┼────────────────────────────────────────────────┘
        │                │
        ▼                ▼
┌───────────────┐  ┌────────────────────────────────┐
│  TfL Unified  │  │  Telegram Bot API              │
│  API          │  │  POST /bot{TOKEN}/sendMessage  │
│               │  │  (1 message per run,           │
│  GET          │  │   all journeys combined)       │
│  /Journey/    │  └────────────────────────────────┘
│  JourneyResult│
│  s/{from}/to/ │
│  {to}         │
└───────────────┘

Trigger paths:

  (a) Scheduled  ─── cron fires ──► runner.Run ──► tfl ──► telegram
                                                              ▲
  (b) On-demand  ─── POST /journey ──► auth middleware ───────┘
      (planned)       (API key via         │
                       X-API-Key header)   └──► same runner.Run pipeline

HTTP server (always running):
  GET /health  →  {"status":"ok"}        (Docker HEALTHCHECK / Coolify probe)
  POST /journey  →  planned on-demand trigger
```

---

## 2. Design Decisions

### Go stdlib only (one exception: robfig/cron)

**Chosen:** `net/http`, `encoding/json`, `context`, `log`, `os`, `net/url` from
stdlib. One external dependency: `github.com/robfig/cron/v3` for cron parsing.

**Alternatives considered:** Gin or Echo (HTTP framework), gocron or a custom
ticker loop (scheduling), viper (config), zap or logrus (structured logging).

**Why:** The service has two simple external calls and no routing complexity.
Adding a framework would multiply the dependency surface area, increase image
size, and introduce upgrade maintenance burden. `robfig/cron` is the only
dependency that would be genuinely painful to reimplement correctly (cron
expression parsing with DST awareness is non-trivial). Everything else is
straightforward enough that stdlib is the right tool.

---

### scratch Docker base image

**Chosen:** `golang:1.22-alpine` as builder, `scratch` as the final runtime
image. `CGO_ENABLED=0` produces a fully static binary.

**Alternatives considered:** `alpine`, `distroless/static`, `debian-slim`.

**Why:** `scratch` results in the smallest possible image (the binary plus CA
certificates — typically under 10 MB). There is no shell, no package manager,
and no attack surface. The trade-off is that debugging inside the container is
impossible, but this service has no interactive debugging requirement and all
useful information is surfaced via structured logs.

---

### No database / stateless design

**Chosen:** The service holds no state. Each cron run fetches fresh data from
TfL and dispatches it. No results are stored between runs.

**Alternatives considered:** SQLite to cache recent results, Redis for
deduplication, a file to track "last sent journey hash".

**Why:** The purpose of the service is to deliver real-time journey information.
Caching would serve stale data; deduplication would suppress legitimate
updates when the journey plan hasn't changed. Statelessness also means the
container can be restarted, redeployed, or horizontally scaled without
coordination. Given the low frequency of runs (twice daily), there is no
performance argument for caching.

---

### Direction reversal via config rather than a separate endpoint

**Chosen:** `MORNING_CRON` always fires `ORIGIN → DESTINATION`; `AFTERNOON_CRON`
always fires `DESTINATION → ORIGIN`. The reversal is hardcoded in `main.go`.

**Alternatives considered:** A `direction` field in a request body, two separate
pairs of `ORIGIN`/`DESTINATION` env vars, a trip configuration file.

**Why:** For a single commuter use case, the return trip is always the exact
reversal. Two separate origin/destination pairs would double the configuration
surface for no benefit. A runtime `direction` parameter would require either a
request body (adding HTTP complexity) or additional env vars. The current design
makes the invariant explicit and eliminates a class of misconfiguration.

---

### Static API key auth (not JWT, not OAuth)

**Chosen (planned for the on-demand endpoint):** A single shared secret passed
via an `X-API-Key` HTTP header, compared with `crypto/subtle.ConstantTimeCompare`.

**Alternatives considered:** JWT with RS256, OAuth 2.0 client credentials, mTLS,
IP allowlisting.

**Why:** This service has a single operator (the person running it). JWT and
OAuth are appropriate for multi-tenant or delegated access scenarios. mTLS adds
certificate management overhead. A static API key is simple, auditable, and
safe when used over TLS (which Coolify's reverse proxy provides). The
constant-time comparison prevents timing-based key enumeration attacks.

---

### Single Telegram message per run

**Chosen:** All journeys for a run are formatted into one message by
`telegram.FormatJourneys` and sent with a single `SendMessage` call.

**Alternatives considered:** One message per journey (previous approach);
concurrent sends with `sync.WaitGroup`.

**Why:** A single message is more readable — the origin, destination, and
search time appear once as a header, and all journey options sit below it as a
self-contained block. It also eliminates ordering concerns (concurrent sends
can arrive out of order) and reduces the number of Telegram API calls per run
from three to one.

---

### TfL API over alternatives

**Chosen:** TfL Unified API — `https://api.tfl.gov.uk/Journey/JourneyResults`

**Alternatives considered:**

| API | Why not chosen |
|---|---|
| **Google Maps Routes API** | Per-request billing; requires geocoding step for postcodes; complex pricing model |
| **TransportAPI** | Paid plans for production use; thinner TfL coverage |
| **OpenTripPlanner** | Requires self-hosting and GTFS data pipeline maintenance |
| **Navitia** | Good coverage but requires account approval; less granular TfL mode data |

**Why TfL:** Native postcode support (no geocoding step), free for read-only
use, returns leg-level mode and instruction data exactly matching what this
service needs, and has authoritative data for London multimodal journeys.

---

## 3. TfL API Notes

### Coverage
The TfL Unified API is London-centric. It covers tube, bus, DLR, Overground,
Elizabeth line, tram, river services, and walking within Greater London.
Setting `nationalSearch=true` extends planning to some cross-boundary national
rail connections (e.g. journeys to Heathrow, Gatwick approach routes, and
southern home counties rail). It is not suitable for journeys entirely outside
London.

### Endpoint used

```
GET https://api.tfl.gov.uk/Journey/JourneyResults/{from}/to/{to}
    ?nationalSearch=true
    &app_key={TFL_APP_KEY}
```

- `{from}` and `{to}` are URL-path-encoded (spaces → `%20`) using
  `url.PathEscape`.
- The service uses "leave now" timing (TfL default when no `time` or `date`
  parameter is supplied).

### Auth

`app_key` is supplied as a query parameter. Without it, requests are accepted
but subject to stricter rate limiting. With a registered free key, limits are
generous for a twice-daily use pattern (well within the free tier of ~500
requests/day per key as documented on the TfL API portal).

### User-Agent

TfL blocks requests sent with the default Go HTTP client user-agent
(`Go-http-client/1.1`) with a `403 Forbidden`. All requests from `tfl/client.go`
set `User-Agent: MultiLegAware/1.0` to avoid this.

### Postcode support

TfL accepts full UK postcodes natively in the URL path — no geocoding or
coordinate lookup is required. Full postcodes are preferred over partial ones
(e.g. `SW1A 1AA` rather than `SW1A`) as partial postcodes may match multiple
points and produce ambiguous results.

### Rate limits

The free developer tier allows approximately 500 requests per day per key (per
TfL API portal documentation; subject to change). This service makes at most 2
requests per day under normal operation, well within limits.

---

## 4. Telegram Notes

### Bot setup

See `docs/telegram-setup.md` for the full step-by-step guide. In summary: create
a bot via @BotFather, retrieve your chat ID via `getUpdates`, and set both
as environment variables.

### Why MarkdownV2

Telegram supports three message parse modes: plain text, `Markdown` (legacy),
and `MarkdownV2`. `MarkdownV2` is used because:

- It supports bold (`*text*`) for journey headers without ambiguity.
- The legacy `Markdown` mode has undocumented edge-case behaviour around
  certain characters and is no longer recommended by Telegram.
- MarkdownV2 is strict — any unescaped reserved character causes a 400 error,
  which surfaces formatting bugs immediately rather than silently mangling output.

### MarkdownV2 escaping rules

The following characters have special meaning in MarkdownV2 and **must be
escaped with a backslash** when they appear in literal text (i.e. outside of
formatting markers):

```
_   *   [   ]   (   )   ~   `   >   #   +   -   =   |   {   }   .   !
```

The `escapeMarkdownV2` function in `telegram/client.go` handles this
replacement for all API-derived strings (TfL instruction summaries, postcode
values). Do not pass unescaped user-controlled or API-controlled strings
directly as MarkdownV2 text.

Bold markers themselves (`*Journey 1*`) and italic markers (`_07:30 Mon 24 Mar_`)
must **not** be escaped — they are intentional formatting.

### Message structure

Each run produces **one Telegram message** containing:

```
*SW1A 1AA → EC1N 2TD*
_07:30 Mon 24 Mar_

*Journey 1* — 34 mins
• 🚶 Walk to Victoria Station — 13 min
• 🚇 Victoria line to Oxford Circus — 4 min
• 🚇 Central line to Chancery Lane — 5 min
• 🚶 Walk to EC1N 2TD — 7 min

*Journey 2* — 38 mins
...
```

The header line is bold; the search time is italic. Each leg is a bullet.
All user-derived and API-derived strings (postcodes, instruction summaries) are
passed through `escapeMarkdownV2` before insertion.
