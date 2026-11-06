# Guide

How to run every part of overhead. For how to measure a window, set up the TV or open the cloud accounts, see
[setup.md](setup.md); for what each part has to do, [SPEC.md](SPEC.md).

## Commands

```sh
make build
./bin/overhead watch                 # live table round Toronto Pearson, redrawn every 5 s; Ctrl-C prints a summary
OVERHEAD_LAT=43.65 OVERHEAD_LON=-79.38 ./bin/overhead watch -radius 20
./bin/overhead watch -for 1h > run.log   # M0's check: one line per poll, then pass/fail on gaps and duplicates
```

The whole stack on an always-on machine ([SPEC](SPEC.md#what-it-needs-to-do), item 10): put your home and window in `.env` (see `.env.example`, and
[setup.md](setup.md) for how to measure the window and set up the TV), then

```sh
docker compose up -d                 # RabbitMQ, poll, enrich, history, live, headsup and the facts job
open http://<this machine>:8090      # on the TV; the RabbitMQ UI is on http://localhost:15673 (overhead / overhead)
docker compose --profile public up -d   # also the public instance, on 127.0.0.1:8091 (setup.md, section 6)
```

For development, the same event pipeline (M1) with the commands on the host, each in its own terminal:

```sh
docker compose up -d rabbitmq        # just the broker
./bin/overhead enrich                # enrichment worker; cache in data/enrich.db
./bin/overhead history               # one-minute batches in data/history/positions/dt=.../hour=.../
./bin/overhead live                  # http://localhost:8090/api/now, /api/events (SSE), /api/coverage
./bin/overhead poll                  # polls the feeds, publishes positions and enrichment jobs
./bin/overhead verify                # checks data/history: every published position stored exactly once
```

The window view (M2): build the screen once, then let the live API serve it and open it full screen on the TV.

```sh
(cd web && pnpm install && pnpm build)          # static files in web/out
export OVERHEAD_LAT=43.65 OVERHEAD_LON=-79.38   # the window, not the airport
export OVERHEAD_WINDOW_AZ=200                   # it faces south-south-west
export OVERHEAD_WINDOW_FOV=100                  # and shows 100 degrees of sky
export OVERHEAD_WINDOW_MIN_EL=8                 # over rooftops 8 degrees high
export OVERHEAD_WINDOW_MAX_EL=40                # up to the window's top edge, 40 degrees, as seen from the sofa
export OVERHEAD_TZ=America/Toronto              # "today" where the window is (a container runs in UTC)
./bin/overhead live -web web/out                # open http://<this machine>:8090 on the TV
```

TV browsers are old: webOS 6 runs Chromium 79, and Samsung's 2021 sets Chromium 76. `pnpm screen-check` opens the
built screen in the Chromium you name, against a made-up live feed, at night, under the moon, by day, at golden hour
and at twilight. It saves a screenshot of each and fails on script errors, layout work while planes move, text
smaller than 23 px at 1080p or text outside the TV-safe area:

```sh
npx @puppeteer/browsers install chromium@706915 --path ~/.cache/screen-check   # Chromium 79, once
export CHROME=~/.cache/screen-check/chromium/linux-706915/chrome-linux/chrome
(cd web && pnpm build && pnpm screen-check --no-sandbox)   # screenshots in web/screen-check/
(cd web && node scripts/screen-check.mjs --serve 3999)  # the made-up feed alone, for a browser by hand
```

Facts between planes (M4) come from the history, worked out by a small Python job with DuckDB; the live API rereads
the file it writes:

```sh
(cd analysis && uv run overhead-stats facts --every 10)   # rewrites data/facts.json every 10 minutes
(cd analysis && uv run overhead-stats passes > passes.csv) # one row per aircraft per pass, for a notebook
```

It uses the same OVERHEAD_LAT/LON and window settings, plus OVERHEAD_TZ (default America/Toronto) for days and
hours. A fact appears only once the history can back it: "first time today" after two days of history, the
month's rarest type and regulars after three.

Without window settings the screen shows the whole sky round home, down to 5 degrees.

To follow Billy Bishop's landings and take-offs and the planes within 3 km of the window, and nothing else:

```sh
export OVERHEAD_AIRPORT=CYTZ OVERHEAD_NEAR_KM=3   # in .env for compose
./bin/overhead live -web web/out
```

Heads-ups (M5) watch the alerts queue with the same window settings; with a Telegram bot they reach the phone,
without one they are only logged. A stored day can be replayed to see what would have been sent:

```sh
export OVERHEAD_TELEGRAM_TOKEN=123:abc OVERHEAD_TELEGRAM_CHAT=42   # from @BotFather; optional
./bin/overhead headsup
./bin/overhead headsup -replay data/history -day 2026-10-30
```

The rules: an emergency squawk anywhere in the circle; a rare airframe, a low pass nearly straight over the window
or a type never seen here (after a first week of learning) when it is due in view within three minutes. Each
aircraft gets each heads-up once a day, kept in `data/headsup.db` so a restart does not repeat them. `pnpm dev` in `web/` serves the
screen with hot reload; add `?api=http://localhost:8090` to the address to point it at a running live API.

History to S3 and Snowflake (M3): the writer puts the same batches in a bucket, and Snowpipe loads them.

```sh
./bin/overhead history -s3 s3://<bucket>/history       # AWS_REGION and credentials as usual
# then, in Snowflake, snowflake/01_load.sql (stage, table, pipe), 02_passes.sql, and 03_reconcile.sql to check counts
docker compose --profile s3 up -d s3                     # a local S3 (rclone) to try it without AWS:
OVERHEAD_S3_ENDPOINT=http://localhost:9010 AWS_ACCESS_KEY_ID=overhead AWS_SECRET_ACCESS_KEY=overhead-s3-local \
  AWS_REGION=us-east-1 ./bin/overhead history -s3 s3://overhead/history
```

AWS and Snowflake in one command (M6), for tests: AWS is not where the live screen runs (that is one small server),
so it goes up for a check and down after.

```sh
make up      # Terraform: bucket, ECR, a Fargate task (RabbitMQ + poll + history to S3), Snowflake pipe; pushes the image
make down    # removes all of it, the bucket's data included
```

`TF_VAR_snowflake=false make up` leaves Snowflake out. Snowflake's credentials come from the usual `SNOWFLAKE_*`
variables; the warehouse is X-Small and suspends after a minute idle.

Every position event carries the poller's run id and a sequence number, so `verify` can prove nothing was lost or
stored twice. The history writer acknowledges a batch only after writing it, and a batch's file name comes from its
first sequence number, so a redelivered batch overwrites its own file. Changing a queue's arguments in
`internal/bus` needs that queue deleted first (RabbitMQ refuses a redeclaration that differs).

`-feeds adsb.lol,adsb.fi` is the default; `airplanes.live` can be added once it grants access. Airport ground
vehicles (ADS-B emitter categories C1–C7) are left out: they are not aircraft.

## The pictures in the README

`pnpm record-demo` makes them again from the built page, frame by frame on a clock it controls, so the result is the
same every run. It plays the page's own recording, the only flight data that may be published. It needs a Chromium,
ffmpeg and gifski (`cargo install gifski`).

```sh
(cd web && pnpm build)
(cd web && CHROME=/path/to/chrome pnpm record-demo)                # all of them, in docs/images/
(cd web && CHROME=/path/to/chrome pnpm record-demo --only dusk,gif)
```

The home screen's picture, `docs/images/home-test-scene.jpg`, is `pnpm screen-check`'s island scene, with made-up
traffic: real routes may not be published.

## Code layout

| Path | What is there |
|---|---|
| `cmd/overhead/` | the CLI: `watch`, `poll`, `enrich`, `live`, `history`, `verify`, `headsup` |
| `internal/feeds`, `internal/poller` | readsb feed clients, the merge by ICAO address, polling with per-feed backoff and gap stats |
| `internal/events`, `internal/bus`, `internal/relay` | wire formats, the RabbitMQ topology and confirmed publisher, snapshots to position events |
| `internal/enrich` | adsbdb and planespotters lookups, the bbolt cache, retry and dead-letter handling |
| `internal/live` | the current picture, the window (sight, in view, coming next), prediction scoring, the SSE server |
| `internal/history` | one-minute batches to disk or S3, `verify` |
| `internal/headsup` | heads-up rules, the ledger, Telegram, replay |
| `internal/geo` | distance, bearing, elevation above the horizon |
| `web/` | the screen: Next.js static export, served by `overhead live -web` and, as the public page, by Netlify; `web/public/replay.json` is its recording |
| `analysis/` | passes (`passes.sql`) and facts with DuckDB, as the `overhead-stats` job |
| `snowflake/`, `terraform/` | M3's SQL and M6's infrastructure |
| `docs/setup.md` | what the owner provides: the window, the TV, the bot, the accounts, the server |
