# overhead

The planes above home, live — on the TV across the room, with the history of everything that flew over and an alert
when something rare does. No radio hardware: it reads free community flight-tracking feeds.

> **Status: M0 (poller + terminal table) built.** This README is the spec. The milestones at the end track what is
> built.

## What it is

Every airliner broadcasts its position, altitude and identity (ADS-B). Thousands of hobbyists receive those
broadcasts and share them through community networks that publish the merged live picture. overhead asks those
networks "what is within 30 km of home?" every few seconds, works out what each plane is and where it is going, shows
it on a screen designed to be read from the sofa, and keeps everything it saw so it can answer questions later:
what was the rarest aircraft this month, what is the busiest hour over the street, when did an A380 last come by.

## Data sources

Personal, non-commercial use; every source is credited on screen. Terms and limits were checked on 2026-10-29: overhead
asks adsb.fi every 5 s and adsb.lol every 15 s (one request every 5 s drew 429s within minutes), sends a User-Agent
naming the project, backs off after errors and honours Retry-After.

| Source | Gives | Notes |
|---|---|---|
| adsb.lol | live positions in a radius | free, no key, unfiltered; limits are "dynamic", and a key (for feeders) is planned |
| adsb.fi | live positions in a radius | free for personal, non-commercial use; one request a second; credit with a link |
| airplanes.live | live positions in a radius | free for non-commercial use, about one request a second; now answers 403 until the project is approved by email, so it is off by default |
| OpenSky Network | live positions (fallback) and an aircraft database | free account, daily request credits |
| adsbdb | aircraft (type, registration, operator) by ICAO address; route by callsign | free |
| planespotters.net | a photo of that exact airframe | photographer credit required |

Coverage is what the volunteer receivers see — excellent around Toronto Pearson, thinner for very low flights.

## What it needs to do

1. **Poller (Go)** — query two feeds for a radius around home every ~5 s, merge by ICAO address (freshest report
   wins), normalise into one position event, and publish it to RabbitMQ (see Event flow).
2. **Enrichment** — type, registration, operator, route and photo per aircraft, as jobs on a work queue; results are
   cached (routes for a day, airframes for a month) so the free APIs see a handful of calls per new plane, not one per
   poll.
3. **Live API** — consumes the live queue, keeps the current picture, and serves it over Server-Sent Events.
4. **History** — consumes the history queue: one-minute batches of positions to S3 (gzipped JSONL), loaded into
   Snowflake by Snowpipe;
   positions are stitched into flights (one row per pass overhead) in the warehouse.
5. **Stats** — rarest types this month, firsts, busiest hours, runway-in-use guesses, an approach-path heatmap;
   explored in Pandas notebooks first, then served as screen panels.
6. **Screen (Next.js)** — a 10-foot UX: big type, dark theme, live map, a "closest plane right now" card (photo,
   airline, route, altitude, minutes to landing), today's count and a rare-visitor banner. Must work in a TV browser.
7. **Alerts** — consume the alerts queue: rules for rare types, military traffic, emergency squawks (7500/7600/7700) and first-ever sightings,
   sent to Telegram with no duplicates.
8. **Infra** — S3, IAM, the poller and RabbitMQ (one small Fargate task) on AWS, plus the Snowflake database, stage
   and pipe, all as infrastructure as code in Terraform; one command up, one down.

## Event flow (RabbitMQ)

An event-driven design: the poller publishes, everything else consumes from its own queue, so a slow or crashed
consumer never holds up the others and catches up from its queue when it returns.

| Exchange → queue | Consumer | Delivery |
|---|---|---|
| `positions` (topic) → `live` | live API, SSE to the screen | latest position wins; the queue is capped (max-length), stale points dropped |
| `positions` → `history` | history writer, one-minute batches to S3 | acked only after the S3 upload (at-least-once); batch keys make a redelivery overwrite, never duplicate |
| `positions` → `alerts` | alert rules | deduplicated per aircraft, rule and day |
| `enrich` (work queue) | enrichment workers (adsbdb, planespotters) | rate-limited lookups retry after a delay (TTL + dead-letter exchange); after 5 failures they park in a dead-letter queue |

Why a message broker for one house: the three consumers want different things (the screen the latest point, S3
complete batches, alerts exactly one message) and the free enrichment APIs need back-pressure. RabbitMQ runs as a
container next to the poller; Amazon MQ would cost more than the rest of the project.

Privacy rule for anything public: some private aircraft owners ask not to be identified, so the public demo shows
commercial flights only and never registrations or owners of private aircraft.

Cost: the Snowflake trial is time-limited — develop on DuckDB locally, move to Snowflake for M3, and keep the
warehouse at X-Small with auto-suspend.

## Run it

```sh
make build
./bin/overhead watch                 # live table round Toronto Pearson, redrawn every 5 s; Ctrl-C prints a summary
OVERHEAD_LAT=43.65 OVERHEAD_LON=-79.38 ./bin/overhead watch -radius 20
./bin/overhead watch -for 1h > run.log   # M0's check: one line per poll, then pass/fail on gaps and duplicates
```

`-feeds adsb.lol,adsb.fi` is the default; `airplanes.live` can be added once it grants access. Airport ground
vehicles (ADS-B emitter categories C1–C7) are left out: they are not aircraft.

## Milestones

| | Milestone | Done when |
|---|---|---|
| M0 | Poller + terminal table of planes within 30 km, two feeds merged | a one-hour run with no gap over 30 s and no duplicate aircraft |
| M1 | Event pipeline (RabbitMQ) and enrichment with caching | at least 90% of commercial flights shown with type and route; under 5 API calls per new plane; stopping any consumer for 10 minutes loses no positions |
| M2 | Screen v1: live map + closest-plane card over SSE | runs a full day in the TV browser without a reload |
| M3 | History: S3 → Snowpipe → Snowflake, flights stitched | a week of data; Snowflake row counts reconcile with the poller's own counts |
| M4 | Stats + notebooks, "today / this month" panels on the screen | rarest-type and busiest-hour panels match the notebook numbers |
| M5 | Alerts to Telegram | replaying a stored day fires exactly the expected alerts, no duplicates |
| M6 | Terraform for AWS + Snowflake; public demo | `make up` / `make down` from a clean account; demo follows the privacy rule |
