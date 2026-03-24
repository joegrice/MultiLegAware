# MultiLegAware

A lightweight Go microservice that fetches multimodal journey options from the TfL Unified API and delivers the top 3 results as Telegram messages, fired automatically on a configurable cron schedule.

## Prerequisites

### 1. Get a Telegram Bot Token

1. Open Telegram and search for **@BotFather**.
2. Send `/newbot` and follow the prompts to name your bot.
3. BotFather will reply with a token like `110201543:AAHdqTcvCH1vGWJxfSeofSs4tGeAAAAA`.
4. Set it as `TELEGRAM_BOT_TOKEN`.

### 2. Get Your Telegram Chat ID

1. Send any message to your new bot.
2. Open this URL in a browser (replace `TOKEN` with your bot token):
   ```
   https://api.telegram.org/botTOKEN/getUpdates
   ```
3. Find the `"chat"` object in the response — the `"id"` field is your chat ID.
4. Set it as `TELEGRAM_CHAT_ID`.

Alternatively, search for **@userinfobot** on Telegram and send it any message — it will reply with your chat ID.

### 3. TfL App Key

Register for a free key at [https://api-portal.tfl.gov.uk](https://api-portal.tfl.gov.uk). Set it as `TFL_APP_KEY`. This is **required** — the service will refuse to start without it.

---

## Environment Variables

| Variable           | Required | Description                                                     |
|--------------------|----------|-----------------------------------------------------------------|
| `TELEGRAM_BOT_TOKEN` | yes    | Bot token from @BotFather                                       |
| `TELEGRAM_CHAT_ID`   | yes    | Chat ID that receives journey messages                          |
| `TFL_APP_KEY`        | yes    | TfL Unified API key                                             |
| `ORIGIN`             | yes    | Journey start for the morning run (e.g. `SW1A 1AA`)            |
| `DESTINATION`        | yes    | Journey end for the morning run (e.g. `EC1N 2TD`)              |
| `MORNING_CRON`       | yes    | 6-field cron expression for the A→B run (see below)            |
| `AFTERNOON_CRON`     | yes    | 6-field cron expression for the B→A run (see below)            |
| `PORT`               | no     | HTTP port for the health endpoint (default: `8080`)             |

### Cron format

The service uses **6-field cron with seconds** (powered by `github.com/robfig/cron/v3`):

```
┌─────────── second  (0–59)
│ ┌───────── minute  (0–59)
│ │ ┌─────── hour    (0–23)
│ │ │ ┌───── day of month (1–31)
│ │ │ │ ┌─── month  (1–12 or JAN–DEC)
│ │ │ │ │ ┌─ day of week (0–6 or SUN–SAT, or MON-FRI)
│ │ │ │ │ │
* * * * * *
```

**Examples:**

| Expression              | Meaning                         |
|-------------------------|---------------------------------|
| `0 30 7 * * MON-FRI`   | 07:30 every weekday             |
| `0 0 17 * * MON-FRI`   | 17:00 every weekday             |
| `0 */15 * * * *`        | every 15 minutes (testing)      |

The afternoon run automatically reverses direction: `DESTINATION → ORIGIN`.

---

## Running Locally

```bash
export TELEGRAM_BOT_TOKEN="your-bot-token"
export TELEGRAM_CHAT_ID="123456789"
export TFL_APP_KEY="your-tfl-key"
export ORIGIN="SW1A 1AA"
export DESTINATION="EC2V 8RT"
export MORNING_CRON="0 30 7 * * MON-FRI"
export AFTERNOON_CRON="0 0 17 * * MON-FRI"
export PORT=8080   # optional

go run .
```

The service logs each scheduled run to stdout and sends one Telegram message per run containing all journey options. To test immediately without waiting for the schedule, temporarily set a cron expression that fires every minute:

```bash
export MORNING_CRON="0 * * * * *"
```

A health endpoint is available while the service runs:

```bash
curl http://localhost:8080/health
# {"status":"ok"}
```

---

## Deploying with Docker

### Build

```bash
docker build -t multilegaware .
```

### Run

```bash
docker run -d \
  -p 8080:8080 \
  -e TELEGRAM_BOT_TOKEN="your-bot-token" \
  -e TELEGRAM_CHAT_ID="123456789" \
  -e TFL_APP_KEY="your-tfl-key" \
  -e ORIGIN="SW1A 1AA" \
  -e DESTINATION="EC2V 8RT" \
  -e MORNING_CRON="0 30 7 * * MON-FRI" \
  -e AFTERNOON_CRON="0 0 17 * * MON-FRI" \
  --name multilegaware \
  multilegaware
```

### Deploy to fly.io

```bash
fly launch --name multilegaware --region lhr
fly secrets set \
  TELEGRAM_BOT_TOKEN="your-bot-token" \
  TELEGRAM_CHAT_ID="123456789" \
  TFL_APP_KEY="your-tfl-key" \
  ORIGIN="SW1A 1AA" \
  DESTINATION="EC2V 8RT" \
  MORNING_CRON="0 30 7 * * MON-FRI" \
  AFTERNOON_CRON="0 0 17 * * MON-FRI"
fly deploy
```

---

## Example Telegram Output

One message is sent per scheduled run containing all journey options:

> **SW1A 1AA → EC1N 2TD** &nbsp; _07:30 Mon 24 Mar_
>
> **Journey 1** — 32 mins
> - 🚶 Walk to St James's Park Station — 5 min
> - 🚇 District line to Westminster — 2 min
> - 🚇 Jubilee line to Chancery Lane — 11 min
> - 🚶 Walk to EC1N 2TD — 9 min
>
> **Journey 2** — 35 mins
> - 🚶 Walk to Victoria — 3 min
> - 🚌 Bus 88 to Oxford Circus — 22 min
> - 🚶 Walk to EC1N 2TD — 10 min
>
> **Journey 3** — 41 mins
> - 🚶 Walk to St James's Park Station — 5 min
> - 🚇 Circle line to Bank — 20 min
> - 🚶 Walk to EC1N 2TD — 14 min

The header shows origin, destination, and the time the search was made. The afternoon run reverses direction automatically.

---

## Notes

- TfL coverage: Greater London and some cross-boundary national rail routes (`nationalSearch=true`).
- Departure time: always "leave now" (TfL default).
- All outbound HTTP calls use a 10-second context timeout.
- Structured logs are written to stdout in `key=value` format.
- The service fails fast on startup if any required environment variable is missing.
