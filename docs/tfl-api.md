# TfL API Reference Notes

This document covers everything needed to understand how MultiLegAware
interacts with the TfL Unified API and how to extend that interaction.

---

## 1. Getting an API Key

1. Go to **https://api-portal.tfl.gov.uk** and register for a free account.
   No credit card is required.
2. After verifying your email, log in and navigate to **Products**.
3. Subscribe to the **Unified API** product (free tier).
4. Under your **Profile**, find the **Primary Key** for your subscription.
   It is a 32-character hex string.
5. Set it in your `.env` file:
   ```
   TFL_APP_KEY=your32characterhexkeyhere
   ```

> `TFL_APP_KEY` is **required** in MultiLegAware. The service will refuse to
> start if it is not set. Without a key, TfL accepts requests but applies
> stricter anonymous rate limits.

---

## 2. Journey Endpoint Reference

### Full URL template

```
GET https://api.tfl.gov.uk/Journey/JourneyResults/{from}/to/{to}
```

`{from}` and `{to}` are URL-path-encoded using `url.PathEscape` in Go
(spaces become `%20`).

### Query parameters used by this service

| Parameter       | Value used        | Effect                                                                 |
|----------------|-------------------|------------------------------------------------------------------------|
| `nationalSearch` | `true`           | Extends journey planning to include some national rail connections beyond Greater London |
| `app_key`        | `TFL_APP_KEY`    | Authenticates the request and grants higher rate limits                |

### Parameters not currently used (worth knowing)

| Parameter         | Type    | Description                                                              |
|-------------------|---------|--------------------------------------------------------------------------|
| `date`            | string  | Travel date in `YYYYMMDD` format. Defaults to today.                     |
| `time`            | string  | Departure or arrival time in `HHMM` format. Defaults to now.            |
| `timeIs`          | string  | Whether `time` is `Departing` or `Arriving`. Defaults to `Departing`.   |
| `journeyPreference` | string | `LeastTime`, `LeastInterchange`, or `LeastWalking`.                   |
| `mode`            | string  | Comma-separated list of modes to filter by (e.g. `tube,bus`).           |
| `maxTransferMinutes` | int  | Maximum walking time between stops in minutes.                          |
| `walkingSpeed`    | string  | `Slow`, `Average`, or `Fast`.                                            |
| `cyclePreference` | string  | For cycle journeys: `AllTheWay`, `LeaveAtStation`, etc.                 |
| `viaId`           | string  | Force journey to pass through a specific stop point.                    |

---

## 3. Response Structure

The TfL response is a large JSON object. MultiLegAware reads only the fields it
needs and ignores the rest. Below is a map of what is used.

### Fields read by this service

```
journeys[]                       — array of journey options
  .duration                      — int: total journey duration in minutes
  .legs[]                        — array of legs within this journey
    .duration                    — int: leg duration in minutes
    .departureTime               — string: ISO 8601 departure datetime
    .arrivalTime                 — string: ISO 8601 arrival datetime
    .mode
      .name                      — string: transport mode identifier (see Section 4)
    .instruction
      .summary                   — string: human-readable leg description
                                   e.g. "Jubilee line towards Stanmore"
```

### Fields intentionally ignored

TfL returns significantly more data. The following are present in the response
but not mapped in `tfl/client.go`:

- **`disruptions`** — service disruption alerts for this journey
- **`fare`** — fare breakdown, Oyster/contactless estimates
- **`path.lineString`** — polyline geometry for map rendering
- **`path.stopPoints`** — intermediate stop points on a leg
- **`arrivalPoint` / `departurePoint`** — coordinates and names of start/end
- **`isDisrupted`** — boolean flag for leg-level disruption
- **`plannedWorks`** — scheduled engineering works affecting the leg
- **`routeOptions`** — alternative route options within a leg

If a future feature needs any of these fields, add them to the relevant struct
in `tfl/client.go`. See Section 3.7 of `CLAUDE.md` for the correct procedure.

---

## 4. Supported Mode Names

These are the known values of `legs[].mode.name` returned by the TfL API, with
their corresponding display emoji as defined in `telegram/client.go`.

| `mode.name`       | Description                          | Emoji |
|-------------------|--------------------------------------|-------|
| `walking`         | On-foot segment                      | 🚶    |
| `tube`            | London Underground                   | 🚇    |
| `bus`             | TfL bus (any number route)           | 🚌    |
| `national-rail`   | National Rail services               | 🚆    |
| `dlr`             | Docklands Light Railway              | 🚈    |
| `overground`      | London Overground                    | 🚆    |
| `elizabeth-line`  | Elizabeth line (Crossrail)           | 🚇    |
| `tram`            | London Trams (Croydon)               | 🚊    |
| `cable-car`       | IFS Cloud Cable Car (Royal Docks)    | 🚡    |
| `coach`           | Long-distance coach                  | 🚌    |
| `river-bus`       | Thames Clipper / river services      | 🚍*   |
| `cycle`           | Cycling (own bike)                   | 🚍*   |
| `cycle-hire`      | Santander Cycles hire                | 🚍*   |
| *(any other)*     | Unknown / future mode                | 🚍    |

\* These modes fall through to the default emoji. Add specific cases in
`telegram/client.go → modeEmoji()` if finer control is needed.

---

## 5. Postcode Support

TfL accepts full UK postcodes natively as path segments in the journey endpoint
— no geocoding or coordinate lookup is required.

### Encoding

Postcodes often contain a space (e.g. `SW1A 1AA`). This space must be
percent-encoded as `%20` in the URL path. In Go, use `url.PathEscape`:

```go
url.PathEscape("SW1A 1AA")  // → "SW1A%201AA"
```

Do not use `url.QueryEscape` for path segments — it encodes spaces as `+`,
which is only valid in query strings and may confuse the TfL router.

### Full vs partial postcodes

Always use the **full postcode** (inward + outward, e.g. `SW1A 1AA`) rather
than just the outward code (`SW1A`). Partial postcodes may resolve to multiple
points and TfL may return unexpected results or an error.

### Place names

TfL also accepts place names and station names (e.g. `"King's Cross"`,
`"Canary Wharf"`) in the from/to path segments. These are resolved by TfL's
disambiguation engine. For reliability, prefer postcodes — they are
unambiguous.

---

## 6. Coverage & Limitations

### Geographic coverage

The TfL Unified API is authoritative for **Greater London** and the transport
modes TfL operates. With `nationalSearch=true`, it can plan journeys that
involve some **national rail services** connecting London to its surroundings
(e.g. journeys to Gatwick Airport via Thameslink, or Reading via the
Elizabeth line).

It is **not** suitable for:
- Journeys entirely outside London (e.g. Manchester to Edinburgh)
- Intercity coach services not routed through London
- International journeys

### Real-time disruptions

The TfL API returns disruption data in the response, but MultiLegAware does not
currently use it. Journey times and routing reflect the planned timetable
combined with TfL's real-time journey planning engine at the time of the
request. If a line is severely disrupted, TfL may route around it automatically
or increase journey duration estimates — but the service does not explicitly
flag disruptions in the Telegram message.

Displaying disruption data is noted as a potential future enhancement.
See `CLAUDE.md` Section 3.7 ("Extending the TfL response parsing") for how to
add fields from the response.
