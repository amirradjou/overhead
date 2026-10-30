# overhead

The planes you can see from your window, live: a calm, screensaver-like view of the sky outside, on the TV or a spare
screen, with what each plane is, where it is going and which one is coming next. No radio hardware: it reads free
community flight-tracking feeds.

> **Status: M0 (poller + terminal table) built.** This README is the spec. The milestones at the end track what is
> built.

## What it is

Every airliner broadcasts its position, altitude and identity (ADS-B). Thousands of hobbyists receive those
broadcasts and share them through community networks that publish the merged live picture. overhead asks those
networks what is flying around home every few seconds and turns the answer into the view through your window. It
knows which way the window faces, how wide it is and how far down you can see over the buildings opposite, so it
draws each plane where you would see it, in the same direction and at the same height above the horizon, and names
it: the airline, where it is flying from and to, the aircraft, how high and how fast. Between planes it says which
one is coming next, from which side and in about how long, and offers a fact worth knowing: the rarest visitor this
month, the first time an airframe has passed, the busiest hour over the street.

It is meant to be left on, like a screensaver: slow, quiet, dim at night and safe for a TV that is on all day. Behind
it, everything it saw is kept, so it can answer questions later, such as when an A380 last came by.

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

1. **Poller (Go)** — query two feeds for a 60 km circle round home every ~5 s (wide enough to see a plane coming a
   few minutes before it reaches the window), merge by ICAO address (freshest report
   wins), normalise into one position event, and publish it to RabbitMQ (see Event flow).
2. **Enrichment** — type, registration, operator, route and photo per aircraft, as jobs on a work queue; results are
   cached (routes for a day, airframes for a month) so the free APIs see a handful of calls per new plane, not one per
   poll.
3. **Live API** — consumes the live queue, keeps the current picture, works out where each plane appears from the
   window (its direction and its angle above the horizon) and which planes come into view next, by projecting each
   track a few minutes ahead, and serves it all over Server-Sent Events.
4. **History** — consumes the history queue: one-minute batches of positions to S3 (gzipped JSONL), loaded into
   Snowflake by Snowpipe;
   positions are stitched into flights (one row per pass overhead) in the warehouse.
5. **Stats** — facts for the screen to show between planes: the rarest types this month, firsts (an airframe or
   airline never seen before), how often a route passes the window, busiest hours, the runway in use; explored in
   Pandas notebooks first.
6. **Screen (Next.js)** — the window view, made to be left on. The sky through your window, drawn to match what you
   see (the window's direction, width and lowest visible angle are settings); each plane where you would see it; the
   one in view named in large type (airline, from and to, aircraft, altitude, speed, a photo of that airframe); the
   next one coming, with a countdown and the side it will appear from; a small overview of everything around; a fact
   between planes. Calm by design: slow motion, dimmer at night, a slight drift over time so nothing burns into a TV,
   and days of running without a reload in a TV browser.
7. **Heads-up** — consumes the alerts queue: an optional message on the phone (Telegram) when something worth
   getting up for is about to pass the window: a rare type, an unusually low pass, an emergency squawk
   (7500/7600/7700), a first-ever visitor. Each is sent once, with no duplicates.
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
| M0 ✅ | Poller + terminal table of planes within 30 km, two feeds merged | a one-hour run with no gap over 30 s and no duplicate aircraft — **720 polls, longest gap 5 s, no duplicates, see [M0 results](#m0-results)** |
| M1 | Event pipeline (RabbitMQ) and enrichment with caching | at least 90% of commercial flights shown with type and route; under 5 API calls per new plane; stopping any consumer for 10 minutes loses no positions |
| M2 | Window view v1: the sky through your window, the plane in view and the next one coming, over SSE | runs a full day in the TV browser without a reload; on one evening, 20 planes seen from the window appear on screen on the right side, and "coming next" is right to within a minute |
| M3 | History: S3 → Snowpipe → Snowflake, flights stitched | a week of data; Snowflake row counts reconcile with the poller's own counts |
| M4 | Stats + notebooks; a fact between planes on the screen | the facts shown match the notebook numbers |
| M5 | Heads-up on the phone (Telegram) | replaying a stored day sends exactly the expected heads-ups, no duplicates |
| M6 | Terraform for AWS + Snowflake; public demo | `make up` / `make down` from a clean account; demo follows the privacy rule |

## M0 results

`overhead watch -for 1h`, run 2026-10-29 from 22:06 to 23:06 EDT on the 30 km circle round Toronto Pearson
(commit 74f6dd1; the later 583104d only strips control characters from feed text):

| | |
|---|---|
| Polls | 720, every 5 s; all 720 had data |
| Longest gap between updates | 5 s (limit 30 s) |
| adsb.fi | 720 answers, 0 errors |
| adsb.lol | 240 answers (one every 15 s), 0 errors, no 429s |
| Aircraft | 87 distinct, 10–25 at once |
| Duplicates | none: in no poll was one callsign on two addresses, or two airborne aircraft within 300 m and 200 ft |

Before adsb.lol was spaced to 15 s, asking it every 5 s drew HTTP 429 after about three and a half minutes; the
poller backed off and adsb.fi carried the picture with no gap.
