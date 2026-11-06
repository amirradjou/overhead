# overhead

The planes you can see from your window, live: a calm, screensaver-like view of the sky outside, on the TV or a spare
screen, with what each plane is, where it is going and which one is coming next. No radio hardware: it reads free
community flight-tracking feeds.

**See it: [overhead-window.netlify.app](https://overhead-window.netlify.app).** This is a real window, on a high floor over
Toronto's harbour, with Billy Bishop airport on the island opposite.

| The window | The screen |
|---|---|
| ![The view from the window: Toronto's harbour, the islands and the runway](docs/images/window.jpg) | ![The screen by day, with the harbour drawn from OpenStreetMap and a Porter Dash 8 landing on runway 26](docs/images/screen-day.jpg) |

The planes are drawn where you would see them through the glass, so a landing touches down on the drawn runway. That is
a real landing, from the page's recording, played at 13:30: the sun, the moon and the stars are worked out for the moment
you look, and the clouds come from the weather.

It is one small event-driven system, in Go, with RabbitMQ between the parts: a poller, an enrichment worker with a
dead-letter queue, a history writer, a heads-up service and two instances of the live API, plus a Next.js screen that
runs for days in a TV browser as old as Chromium 79, and a DuckDB job for the facts. Terraform can take the history
through S3 and Snowpipe into Snowflake for a test on AWS. The whole stack runs from one `docker compose up -d`.

> **Status (2026-11-05): M0 and M1 meet their "Done when". M2–M6 are built and wait on things only the owner can
> provide (the window's measurements, a TV device, AWS and Snowflake accounts): [docs/setup.md](docs/setup.md) says how
> to provide each, and [Milestones](#milestones) what each still needs. M7, the public page, is built and running.**
> There is no server behind the public page: its live feed comes from the same small stack at home, so it is live when
> that machine is on, and plays a labelled recording when it is not. This README is the spec.

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

Personal, non-commercial use; every source is credited on screen, and linked where a pointer exists (adsb.fi asks for
a link to its home page). What the public page may use is narrower: see [The public page](#the-public-page). Terms and
limits were checked on 2026-10-29 and 2026-11-05: overhead
asks adsb.fi every 5 s and adsb.lol every 15 s (one request every 5 s drew 429s within minutes), sends a User-Agent
naming the project, backs off after errors and honours Retry-After.

| Source | Gives | Notes |
|---|---|---|
| adsb.lol | live positions in a radius | free, no key, unfiltered, under the ODbL; limits are "dynamic", and a key (for feeders) is planned |
| adsb.fi | live positions in a radius | free for personal, non-commercial use; one request a second; credit with a link |
| airplanes.live | live positions in a radius | free for non-commercial use, about one request a second; now answers 403 until the project is approved by email, so it is off by default |
| adsbdb | aircraft (type, registration, operator) by ICAO address; route by callsign | free, no key; one call returns both; route data "may not be copied, published, or incorporated into other databases" without its owner's permission, so routes stay in the day-long cache, never reach the history and are not on the public page |
| planespotters.net | a photo of that exact airframe | photographer credit on every photo, linked to its photo page where a pointer exists; hotlinked, never proxied or stored; terms to read in a browser (the page refuses scripted fetches) |
| Open-Meteo | the weather at home: cloud cover by layer, wind, visibility | free for non-commercial use, no key, credited and linked; asked once every 10 minutes by the live API whatever the number of screens; `OVERHEAD_WEATHER=off` stops it |
| aviationweather.gov | the followed airport's weather report (METAR): its cloud layers, gusts and pressure | free, no key; asked once every 10 minutes; preferred to the model where both speak, since on 2026-10-30 Open-Meteo drew a nearly clear sky under the broken deck both Toronto airports reported; `OVERHEAD_METAR` names another station |
| OpenStreetMap | Toronto harbour's shores, islands, runways, piers and buildings, for the view from a high window | © OpenStreetMap contributors (ODbL), credited and linked on screen; extracted once into `web/lib/scenery/` by `web/scripts/scenery/fetch.sh` |

Coverage is what the volunteer receivers see — excellent around Toronto Pearson, thinner for very low flights.

The data was checked against independent sources on 2026-10-30 (OpenSky's state vectors and aircraft database,
hexdb.io, OurAirports, the airports' METARs, published type specifications). Positions agree with OpenSky to a
median of 46 m and 25 ft; Billy Bishop's position, elevation and runways match OurAirports; wind and temperature
match the METARs; every type's first flight, length and wingspan is right. What was wrong is corrected: routes
that do not fit what the plane is doing (6 of 43, often a reused flight number's old leg) are no longer shown; Air
Canada Rouge and Air Canada Express are named as they fly; registrations read "C-GFJD", not "CA-GFJD"; the feeds'
type is preferred where the airframe database disagrees; airport vehicles without an emitter category are dropped;
heights are true altitudes (ADS-B's pressure altitude corrected for the day's pressure, which was 300 ft on the
day); a time to landing is given only to a plane heading for its airport; a plane is "on final" only heading in
along the runway; and the day's landings and take-offs are counted from touchdowns and lift-offs and rebuilt from
the history when the live API restarts.

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
4. **History** — consumes the history queue: one-minute batches of positions (gzipped JSONL) on local disk, stitched
   into passes (one row per aircraft per pass) with DuckDB. That is what the facts and the heads-ups read, always on
   and free. The same batches can go to S3 for Snowpipe to load into Snowflake, where the passes are rebuilt in SQL:
   a test path, run on the Snowflake trial and taken down after (see Cost).
5. **Facts** — short, true sentences. About the plane in view: its first pass here or how many this month, how far
   it still has to go and the local time where it lands. Between planes: planes through the window today, the
   busiest hour, this month's rarest type and regular, how often a flight number passes the window, and which way
   Pearson is landing (so which side planes come from). Counted from the passes by flight number, never by route
   (adsbdb's routes may not be stored). A fact appears only once the history backs it.
6. **Screen (Next.js)** — the window view, made to be left on. The sky through your window, drawn to match what you
   see (the window's direction, width and lowest and highest visible angles are settings): its colour follows the
   sun, and the moon and the brightest stars are drawn where they stand. Each plane where you would see it; the one in
   view named in large type (airline, from and to with how far along the trip it is, aircraft, altitude, speed, a
   photo of that airframe) and a fact about its type; the next one coming, with a countdown and the side it will
   appear from; a small overview of everything around, turned to the window (its direction at the top), not to
   north; between planes, a fact from the history, the next sunrise or sunset and the moon. Calm by design: slow
   motion, dimmer at night, a slight drift of the whole scene so nothing burns into a TV, and days of running without
   a reload in a TV browser as old as Chromium 79. Readable from the sofa: no text under 23 px at 1080p, every label
   at least 4.5:1 against the sky behind it, all text inside a 5% safe area. From high up, where the window looks
   down past the horizon, the bottom of the view is water or land below the horizon rather than rooftops; high over
   Toronto's harbour it is the harbour itself, drawn from OpenStreetMap with the planes' own geometry (the lake out
   to its horizon, the islands and their trees, Billy Bishop's runways, the piers, the harbour front's buildings
   with their floors, the tall ship at its quay), so a landing plane touches the drawn runway; at night the runway's
   edge lights, the shore's lamps and lit windows. The clouds are the weather's: cumulus, altocumulus and cirrus in
   perspective, or an overcast deck, coloured by the sun and drifting with the wind; rain and snow show too. Between
   planes the screen says the weather and the wind, and the runway it puts in use. Each plane is a small 3D model of
   its kind (high-wing turboprop, jet, rear-engined jet, widebody, light aircraft, helicopter) at its type's size and
   in its airline's tail colour, turned to its heading, pitched to its climb and banked into its turns, seen as the
   window sees it (from above when it is below the window) and shaded by the sun; at night its navigation lights sit
   on the right wingtips and its landing lights glare when it points at the window. The plane in view gets a card:
   what it is doing, the airline, type and registration, the trip from airport code to airport code with the time it
   lands, its height, speed and climb, and for a flight at the followed airport a side view of its approach or climb
   against the 3° path and the window's height.
   It can follow one airport (`OVERHEAD_AIRPORT`, Billy Bishop for now): then it shows only that airport's landings
   and take-offs and planes very close to the window (`OVERHEAD_NEAR_KM`), each told in full (on final for runway
   26 and when it touches down, or how long ago it took off; its height against the window's; where to look; the
   airframe), with a board of what is landing, taking off and taxiing there and the runway in use. Still to come:
   the planets, saying so when cloud hides the planes, and dimming nearly to black late at night when nothing is
   flying.
7. **Heads-up** — consumes the alerts queue: an optional message on the phone (Telegram) when something worth
   getting up for is about to pass the window: a rare type, an unusually low pass, an emergency squawk
   (7500/7600/7700), a first-ever visitor. Each is sent once, with no duplicates.
8. **Infra (tests only)** — S3, IAM, the poller and RabbitMQ (one small Fargate task) on AWS, plus the Snowflake
   database, stage and pipe, all as infrastructure as code in Terraform; one command up, one down, with a budget
   alert. It goes up to prove the cloud path and comes down after; the screen never depends on it.
9. **Window setup** — the window is its direction (a phone compass set to true north, or the wall's bearing on a map;
   magnetic north is about 10° off in Toronto), its left and right edges and its top as seen from where you sit, the
   lowest angle visible over the roofs opposite (a phone level sighted along the roofline; negative, below the
   horizon, from high up with nothing opposite), and the home's height
   above sea level (OVERHEAD_HOME_ELEV); [docs/setup.md](docs/setup.md#1-home-and-window) says how to measure each.
   Still to come: a setup view that draws every plane round home on a full-sky strip with the window over it, so the
   numbers can be nudged until a plane you see enters where the screen says.
10. **Where it runs** — at home, one always-on small machine runs the whole stack with one `docker compose up -d`
    (RabbitMQ, poll, enrich, history, live, headsup and the facts job), restarting on boot. The TV shows it full
    screen, best from a Raspberry Pi or HDMI stick in Chromium kiosk mode; the TV's own browser is best effort, and its
    auto power-off timer must be off. The machine must never sleep: a desktop's automatic suspend leaves a gap in the
    history. The live API stays on the home network (it shows every plane, private ones too). The public page is a
    second instance on the same machine: see [The public page](#the-public-page).

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

Privacy rule for anything public: some private aircraft owners ask not to be identified, so the public page shows
commercial flights only and never registrations or owners of private aircraft.

Cost: nothing that runs all the time costs money beyond the home box; the public page is free static hosting and a
free tunnel. The Snowflake trial
is time-limited and used once, for M3, M4 and M6, after a week of history exists locally; the warehouse is X-Small
with a 60 s auto-suspend. AWS goes up for that check and down the same day: the Fargate task (0.5 vCPU, 1 GB, a public
IPv4, ca-central-1) left running would cost roughly US$25 a month, so a US$5 budget alert comes first.

## The public page

[overhead-window.netlify.app](https://overhead-window.netlify.app) is the same screen looking out of the same window, for
people who have never seen it. The page itself is static, on Netlify. Its planes come over one stream from `live-public`,
a second instance of the live API on the machine at home, reached through a tunnel. There is no server that stays on
for this, so the page says which it is showing: live while that machine is up, and otherwise a recording of a real
evening, labelled as one, until the feed returns.

The home screen says more. This is its test scene, with made-up traffic, routes and a placeholder photo:

![The home screen's test scene: the flight card with its route, photo and approach profile, and the airport's board](docs/images/home-test-scene.jpg)

The public instance is one more consumer on the bus, with its own queues: nothing it does can reach the home screen or
add a request to any feed. It shows less, because the sources allow less ([docs/setup.md](docs/setup.md#7-what-the-sources-allow-on-a-public-page)
has their words):

| | Home screen | Public page |
|---|---|---|
| Planes | every aircraft | commercial flights only |
| Positions | adsb.fi and adsb.lol, merged | adsb.lol's own reports (ODbL), unmerged |
| Routes and photos | yes | no: adsbdb's routes need its owner's permission, and planespotters' terms are still unread |
| Facts and the day's counts | from the history | none: they are drawn from merged data, and a restart would make the counts untrue |
| Reachable from | the home network | the internet, through a tunnel to one port |
| Runs with | `.env`, the data volume | settings named one by one, no secrets, no volume, a read-only filesystem, no capabilities |

The work per tick does not grow with the viewers. Each snapshot is encoded once, as a standalone deflate block that any
stream can carry whatever it was sent before, so every viewer shares one copy and `/api/now` answers from it too.
Measured on 2026-11-05 with 198 streams open at once: 16 MB of memory, about 0.1% of a core, and the poller's rate
unchanged. Past 200 streams the instance answers 503, and the page plays its recording for whoever is turned away. A
viewer costs 0.7 KB/s, down from the 24 KB/s of the home stream, by leaving out the planes a focused screen never
draws and compressing the rest.

| At night | On a phone |
|---|---|
| ![The screen at night: the runway's edge lights, the shore's lamps and a departing flight's navigation lights](docs/images/screen-night.jpg) | <img src="docs/images/phone.jpg" alt="The public page on a phone" width="240"> |

The recording ([web/public/replay.json](web/public/replay.json)) is published data, so `pnpm record-replay` refuses
anything but the public instance's stream: no other feed's data, no private flights, no routes, no photos.

## Run it

```sh
make build
./bin/overhead watch                 # live table round Toronto Pearson, redrawn every 5 s; Ctrl-C prints a summary
OVERHEAD_LAT=43.65 OVERHEAD_LON=-79.38 ./bin/overhead watch -radius 20
./bin/overhead watch -for 1h > run.log   # M0's check: one line per poll, then pass/fail on gaps and duplicates
```

The whole stack on an always-on machine (README item 10): put your home and window in `.env` (see `.env.example`, and
[docs/setup.md](docs/setup.md) for how to measure the window and set up the TV), then

```sh
docker compose up -d                 # RabbitMQ, poll, enrich, history, live, headsup and the facts job
open http://<this machine>:8090      # on the TV; the RabbitMQ UI is on http://localhost:15673 (overhead / overhead)
docker compose --profile public up -d   # also the public instance, on 127.0.0.1:8091 (docs/setup.md, section 6)
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

## Milestones

| | Milestone | Done when |
|---|---|---|
| M0 ✅ | Poller + terminal table of planes within 30 km, two feeds merged | a one-hour run with no gap over 30 s and no duplicate aircraft — **720 polls, longest gap 5 s, no duplicates, see [M0 results](#m0-results)** |
| M1 ✅ | Event pipeline (RabbitMQ) and enrichment with caching | at least 90% of commercial flights shown with type and route; under 5 API calls per new plane; stopping any consumer for 10 minutes loses no positions — **92% (24 of 26), 2.26 calls per new plane, 8,014 of 8,014 positions stored after a 10-minute outage, see [M1 results](#m1-results)** |
| M2 🔶 built | Window view v1: the sky through your window, the plane in view and the next one coming, over SSE | on the TV's own device (named in M2 results): 24 h without a reload, through one restart of `overhead live`, with the JS heap at 24 h under twice its 1 h size; screenshot tests at night, twilight, golden hour and day with no text under 23 px at 1080p; over one or two clear evenings, of 20 planes logged at the window at least 18 were on screen at that moment, each within 15° of where they were seen; and at least 80% of 1–5-minute "coming next" predictions over those evenings (at least 30, misses counted as wrong) came into view within a minute (`/api/predictions`) |
| M3 🔶 built | History: S3 → Snowpipe → Snowflake, passes stitched | seven consecutive days of history (collected at home, or by Fargate) loaded through S3 and Snowpipe; for every run, Snowflake's distinct (run, seq) count equals `overhead verify`'s, and its passes per day equal DuckDB's `passes.sql`; at least one hour of it written to S3 by the Fargate task itself |
| M4 🔶 built | Facts: about the plane in view and between planes | on three days of history, every figure a fact shows equals the same figure recomputed from Snowflake's passes (`02_passes.sql`), not from the same DuckDB query; while the feed is up, the caption is never empty between planes |
| M5 🔶 built | Heads-up on the phone (Telegram) | replaying seven stored days sends exactly the heads-ups on a list made by hand from the passes (each emergency squawk, low pass over the window, rare or first-seen type), none twice; one live heads-up reaches the phone at least two minutes before its plane enters the window |
| M6 🔶 built | Terraform for AWS + Snowflake | from a clean account, `make up` brings everything up and the Fargate task's batches reach Snowflake; `make down` leaves nothing (`terraform state list` empty, bucket gone); the test costs under US$5 |
| M7 🔶 built | Public page | over seven days the page always answers, live or plainly labelled as a recording: every plane shown has an airline callsign, the only feed credited and used is one whose licence allows it, no route or photo appears in any response or in the recording, the credits link to each source, and the feeds see the same request rate with 1 viewer or 200 — **built and load-tested 2026-11-05 (198 streams: 16 MB, poll rate unchanged); the seven days are still to run** |

What the open milestones wait for (sections of [docs/setup.md](docs/setup.md)):

- **M2**: the window measured (1), the TV device on a machine that never sleeps (2), and one or two clear evenings
  for the sighting log (3).
- **M3, M4, M6**: seven consecutive days of history (the home stack stopped on 2026-10-30 and has collected again since
  2026-11-05 23:21 UTC, so 2026-11-12 at the earliest), then the AWS account and the Snowflake trial for one session (5).
- **M5**: a Telegram bot (4); the hand-made list for the replay comes from the stored passes.
- **M7**: seven days of the page up (6), and four replies only the owner can get, which would let it show more (7).

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

## M1 results

The whole pipeline (poll, enrich, history, live) ran round Toronto Pearson on 2026-10-29 from 23:30 to 00:47 EDT
(commit 26d1d42 plus the fixes up to ce3b884; the later fixes in this milestone were not in that binary).

| | |
|---|---|
| Outage | history and enrich stopped from 23:34 to 23:45 (just over ten minutes) while the poller kept publishing; live restarted at 23:47 |
| Positions | 8,014 published, none lost by the poller; `overhead verify`: all 8,014 stored, none missing, none duplicated |
| Durable queues | the alerts queue (no consumer yet) held all 8,014; history and enrich drained to zero after the restart |
| Coverage | of the 26 airline flights a minute or more airborne in view after live's restart, 24 had type and route on screen (92%); the two without, CPA075 and PTR2161, have no route in adsbdb |
| API calls | 131 calls for 58 new aircraft across both enrich runs: 2.26 per new plane (cache hits: 239) |
| Dead letters | 5 jobs, all with the callsign "@@@@@@@@" that readsb prints when it cannot decode one; adsbdb answered 400. Such callsigns are now dropped at the feed (7e03ff2) |
| Poller | 921 polls, longest gap 5 s |

## Licence

The code is under the [MIT licence](LICENSE). The data this repository holds keeps its own:

- `web/lib/scenery/` is extracted from [OpenStreetMap](https://www.openstreetmap.org/copyright), © OpenStreetMap
  contributors, available under the [Open Database License](https://opendatacommons.org/licenses/odbl/1-0/).
- `web/public/replay.json` contains information from [adsb.lol](https://adsb.lol), made available under the
  [Open Database License](https://opendatacommons.org/licenses/odbl/1-0/), and aircraft details from
  [adsbdb](https://www.adsbdb.com). The recording is offered under the same licence.
- The font, Atkinson Hyperlegible Next, is under the SIL Open Font License, bundled from its npm package.
- `docs/images/window.jpg`, a photograph, is the author's and is not covered by the MIT licence.
