# Operational Runbook

---

## 1. Starting & Stopping

### Start (detached)

```bash
docker compose up -d
```

### Stop

```bash
docker compose down
```

### Restart a running service (e.g. after a config change)

```bash
docker compose restart journey-service
```

### Check whether the container is running

```bash
docker compose ps
```

A healthy container shows `running (healthy)` in the `STATUS` column once the
Docker `HEALTHCHECK` passes (typically within 30 seconds of startup).

You can also hit the health endpoint directly:

```bash
curl -s http://localhost:8080/health
# Expected: {"status":"ok"}
```

---

## 2. Checking Logs

### Follow live logs

```bash
docker compose logs -f journey-service
```

### Last 100 lines (no follow)

```bash
docker compose logs --tail=100 journey-service
```

### Healthy startup output

A correctly configured service prints lines similar to the following on startup:

```
2024/11/25 07:00:00 level=info msg="cron scheduler started" morning="0 30 7 * * MON-FRI" afternoon="0 0 17 * * MON-FRI"
2024/11/25 07:00:00 level=info msg="starting server" addr=":8080"
```

If any required environment variable is missing the process exits immediately:

```
2024/11/25 07:00:00 level=fatal msg="required environment variable not set" var="TFL_APP_KEY"
```

If a cron expression is malformed the process exits immediately:

```
2024/11/25 07:00:00 level=fatal msg="invalid MORNING_CRON expression" expr="bad expr" error="..."
```

### Successful scheduled run output

```
2024/11/25 07:30:00 level=info msg="morning run triggered" from="SW1A 1AA" to="EC2V 8RT"
2024/11/25 07:30:01 level=info from="SW1A 1AA" to="EC2V 8RT" journeys_found=3 tfl_duration_ms=843
```

A single Telegram message is sent containing all three journeys. If TfL returns
no results, a "no journeys found" message is sent instead:

```
2024/11/25 07:30:01 level=info from="SW1A 1AA" to="EC2V 8RT" journeys_found=0 tfl_duration_ms=712
```

---

## 3. Verifying the Schedule

### Confirm cron jobs are registered

Look for the scheduler startup line in the logs immediately after the container
starts:

```
level=info msg="cron scheduler started" morning="0 30 7 * * MON-FRI" afternoon="0 0 17 * * MON-FRI"
```

Both expressions should match the values you set in your environment. If either
is wrong, update the environment variable and restart the service.

### Test a journey immediately without waiting for the schedule

While the on-demand HTTP endpoint is not yet implemented, you can test the full
pipeline by temporarily setting a cron expression that fires every minute:

```bash
# In your .env or Coolify environment variables:
MORNING_CRON=0 * * * * *

# Restart the service, wait up to 60 seconds, then check Telegram and logs.
# Restore the real expression afterwards.
```

---

## 4. Common Failure Modes

| Symptom | Likely Cause | Diagnostic Step | Fix |
|---|---|---|---|
| No Telegram message received | Bot token or chat ID wrong; bot never started | Check logs for `telegram_error`; try `curl` test in `docs/telegram-setup.md` | Verify `TELEGRAM_BOT_TOKEN` and `TELEGRAM_CHAT_ID`; send `/start` to the bot |
| No Telegram message received | Cron schedule not firing at expected time | Check timezone — cron runs in the container's UTC clock | Adjust cron expression to UTC, or set `TZ` env var if your deployment platform supports it |
| "No journeys found" message received | Postcodes unrecognised by TfL; typo in `ORIGIN`/`DESTINATION` | Call TfL API manually: `curl "https://api.tfl.gov.uk/Journey/JourneyResults/SW1A%201AA/to/EC2V%208RT?nationalSearch=true"` | Correct the postcode values and restart |
| Service crashes on startup | Missing required env var | Check exit log for `level=fatal msg="required environment variable not set"` | Set the missing variable and restart |
| Service crashes on startup | Invalid cron expression | Check exit log for `level=fatal msg="invalid MORNING_CRON expression"` | Fix the cron expression (see cron format in `README.md`) |
| HTTP 401 on POST /journey | Wrong or missing API key header | Check `X-API-Key` header value | Use the correct key set in `API_KEY` env var (when auth middleware is implemented) |
| Journey times look wrong / outdated | TfL "leave now" is based on server clock | Verify the container clock: `docker compose exec journey-service date` | Ensure container time is correct; TfL results reflect real-time conditions at request time |
| Telegram 400 Bad Request error | MarkdownV2 escaping issue in a TfL instruction summary or postcode | Note the failing value in logs; test escaping manually | Fix `escapeMarkdownV2` in `telegram/client.go` or handle the specific character |

---

## 5. Updating the Service

Pull new code and redeploy without exposing secrets:

```bash
# 1. Pull latest code
git pull

# 2. Rebuild the image
docker compose build journey-service

# 3. Replace the running container
docker compose up -d journey-service
```

Secrets are never in the image or in version control — they are injected at
runtime via environment variables. No secret handling is required during this
process.

To verify the new version is running:

```bash
docker compose ps
docker compose logs --tail=20 journey-service
curl -s http://localhost:8080/health
```

---

## 6. Coolify-Specific Notes

### Setting environment variables

In the Coolify UI, navigate to your service → **Environment Variables** tab.
Add each variable from the table in `README.md` as a key/value pair. Coolify
injects them into the container at runtime — you do not need a `.env` file
on the server.

Mark sensitive variables (`TELEGRAM_BOT_TOKEN`, `TFL_APP_KEY`) as
**Secret** in Coolify so they are not shown in the UI after saving.

### No .env file needed

When deploying via Coolify, do not create a `.env` file on the server. Coolify
manages environment injection directly. Using a `.env` file alongside Coolify
can cause variable conflicts.

### Triggering a redeploy

From the Coolify UI: navigate to your service → click **Redeploy**. Coolify
will pull the latest image (or rebuild from source if configured), then
replace the running container with zero-downtime if a healthcheck is configured.

Alternatively, configure a webhook in Coolify and trigger it from your CI
pipeline on push to your deployment branch.

### Port mapping

The service listens on port `8080` inside the container. In Coolify:

- If using Coolify's built-in reverse proxy (Traefik/Caddy): set the
  **Container Port** to `8080` and let Coolify handle external routing and
  TLS termination. You do not need to expose 8080 publicly.
- If binding directly: map `8080:8080` in the port settings. Ensure the
  host firewall allows traffic on that port.

The `/health` endpoint at `http://<host>:8080/health` can be used as the
Coolify health check URL.

---

## 7. Local Development with docker-compose.dev.yml

### When to use this vs the production compose file

Use `docker-compose.dev.yml` when you are iterating locally and do not yet have
a fully populated `.env` file, or when you want to override individual
environment variables inline without touching `.env`. Use the production
`docker-compose.yml` when you need to validate the exact production configuration
(healthcheck, restart policy, env_file loading) before deploying.

Both files can run at the same time — they use different service names and
different host ports (dev binds `8081`, prod binds `8080`).

### Start

```bash
docker compose -f docker-compose.dev.yml up --build
```

`--build` ensures the image is rebuilt from the current source on every start.
Omit it if you haven't changed any Go files and want a faster startup.

### Tail logs

```bash
docker compose -f docker-compose.dev.yml logs -f
```

### Test the on-demand endpoint (port 8081)

```bash
curl -s -X POST http://localhost:8081/journey \
  -H "X-API-Key: dev-api-key" \
  -H "Content-Type: application/json" \
  -d '{"from":"SW1A 1AA","to":"EC2V 8RT"}' | jq
```

> Note: the on-demand `/journey` endpoint and its `X-API-Key` auth middleware
> are planned but not yet implemented. The command above will return a 404
> until that work is complete. Use the `MORNING_CRON` / `AFTERNOON_CRON`
> environment variables to trigger runs in the interim.

### Stop

```bash
docker compose -f docker-compose.dev.yml down
```

### Placeholder values cause API failures

The inline environment variables in `docker-compose.dev.yml` ship with
placeholder strings (`your-token-here`, `your-tfl-key-here`, etc.). The service
**will start** with these values but every cron-triggered run will fail:

- TfL calls will return a 4xx (invalid or missing app key).
- Telegram sends will return 401 Unauthorized (invalid token).

Replace the placeholders with real values before expecting end-to-end results.
Do not commit real values back to the file — use a local git stash or a
`.env.local` sourced manually if you need persistence between sessions.
