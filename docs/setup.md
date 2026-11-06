# Setting up overhead at home

overhead runs without any of this: `docker compose up -d` with no `.env` watches the 60 km circle round Toronto
Pearson and shows the whole sky. Each section below makes one more part real, and the milestones that are still open
([SPEC: Milestones](SPEC.md#milestones)) need them. Each says what to get, where it goes and how to check it.

Settings and secrets go in two places, never in the repo:

- `.env` in the repo directory (copy `.env.example`; git ignores it): home, the window, the time zone, the Telegram
  bot. `docker compose` reads it.
- `~/.config/overhead/cloud.env` (made by you, `chmod 600`): the AWS profile name and the Snowflake login, read only
  by `make up` and `make down`.

| Section | Needed for |
|---|---|
| [1. Home and window](#1-home-and-window) | M2: planes drawn where you see them |
| [2. The always-on machine and the TV](#2-the-always-on-machine-and-the-tv) | M2's 24 hours; M3's seven days of history |
| [3. The evening log](#3-the-evening-log) | M2's check at the window |
| [4. Heads-ups on the phone](#4-heads-ups-on-the-phone) | M5 |
| [5. AWS and Snowflake](#5-aws-and-snowflake) | M3, M4, M6 |
| [6. The public page](#6-the-public-page) | M7 |
| [7. What the sources allow on a public page](#7-what-the-sources-allow-on-a-public-page) | M7 |
| [8. Development](#8-development) | the integration tests |

## 1. Home and window

Everything in `.env`:

| Setting | What it is | How to find it |
|---|---|---|
| `OVERHEAD_LAT`, `OVERHEAD_LON` | home | right-click your building on a map and copy the coordinates; three decimals (about 100 m) is plenty, since 100 m moves a plane 10 km away by under 1° |
| `OVERHEAD_HOME_ELEV` | the window's height above sea level, in metres | the ground's height from a map that shows elevation (Google Earth shows it under the pointer), plus about 3 m a floor |
| `OVERHEAD_WINDOW_AZ`, `OVERHEAD_WINDOW_FOV` | the direction the window faces and how wide a slice of sky it shows, in degrees | from where you sit, below |
| `OVERHEAD_WINDOW_MIN_EL` | the lowest angle you can see, over the roofs opposite; negative from high up, where you look down past the horizon | from where you sit, below |
| `OVERHEAD_WINDOW_MAX_EL` | the window's top edge, as an angle above the horizon | from where you sit, below |
| `OVERHEAD_TZ` | the time zone of "today" | `America/Toronto` |

Measure from where you usually sit, since the window shows a different slice of sky from every spot in the room:

1. **Left and right edges.** Use a compass app set to true north (magnetic north is about 10° off in Toronto; the
   iPhone Compass has a "Use True North" setting). Point the top of the phone at the window's left edge and note the
   bearing, then the right edge. `OVERHEAD_WINDOW_AZ` is halfway between them and `OVERHEAD_WINDOW_FOV` is the
   difference: left 150°, right 250° gives 200 and 100. A window that spans north, left 330° and right 30°, gives 0
   and 60. Phone compasses go wrong near speakers and metal, so check the direction against a map: the window faces
   at right angles to its wall.
2. **The roofline and the top edge.** Use a clinometer app that sights through the camera (the phone's level works if
   you can lay its edge along your line of sight). Aim at the top of the roofs opposite for `OVERHEAD_WINDOW_MIN_EL`
   and at the window's top edge for `OVERHEAD_WINDOW_MAX_EL`. Without an app, the angle of the top edge is
   atan(how far it is above your eyes ÷ how far it is from your eyes): 1 m above and 1.7 m away is 30°.
   From high up with nothing opposite, the lowest angle is below the horizon: the bottom of the glass, or the
   balcony's edge, as seen from your seat (down to -45).
3. To follow an airport you can see, add `OVERHEAD_AIRPORT=CYTZ` (Billy Bishop) and `OVERHEAD_NEAR_KM=3`: the screen
   then shows only that airport's landings and take-offs and planes within 3 km of the window, each in full.
4. Apply it: `docker compose up -d` recreates the services whose settings changed.

**Check:** the next time a plane crosses the window, it should be on the screen at the same place. If planes are
always a little to one side, move `OVERHEAD_WINDOW_AZ` by that many degrees; if they appear on the screen while
still hidden behind the roofs, raise `OVERHEAD_WINDOW_MIN_EL`. (the [SPEC](SPEC.md#what-it-needs-to-do)'s item 9, a setup view, a full-sky strip to do
this against every plane round home, is not built yet.)

The live API serves home's coordinates and every plane, private ones too, so port 8090 stays on the home network. Only
`live-public` faces the internet (section 6).

## 2. The always-on machine and the TV

The whole stack runs on one machine that stays on, and the TV shows `http://<that machine>:8090` full screen.

**The machine.** Any computer with Docker that never sleeps. The stack's memory limits add up to about 1.5 GB.

- Fedora or another GNOME desktop suspends after 15 idle minutes even on mains power, which stops the stack and leaves
  a gap in the history: Settings → Power → Automatic Suspend → off when plugged in, or
  `gsettings set org.gnome.settings-daemon.plugins.power sleep-inactive-ac-type nothing`. A laptop also sleeps when
  its lid closes unless logind is told otherwise.
- Docker must start at boot: `sudo systemctl enable --now docker`. The services restart with it.
- If the TV can't open the page, the firewall is in the way: on Fedora,
  `sudo firewall-cmd --permanent --add-port=8090/tcp && sudo firewall-cmd --reload`.

**The TV.** Two ways, best first:

1. **A Raspberry Pi (4 or 5) or a mini PC on the TV's HDMI**, running Chromium in kiosk mode at login:

   ```sh
   chromium --kiosk --noerrdialogs --disable-session-crashed-bubble \
     --user-data-dir="$HOME/.config/overhead-kiosk" --remote-debugging-port=9222 http://<machine>:8090
   ```

   Turn screen blanking off (Raspberry Pi OS: `sudo raspi-config` → Display Options → Screen Blanking) and turn SSH
   on: M2's 24-hour check reads the page's memory through the debugging port, over SSH. A Pi 5 with 4 GB or more can
   also be the always-on machine: `docker compose up -d` builds the arm64 images itself.
2. **The TV's own browser.** Open the address and keep it on screen. TV browsers are old Chromium (webOS 6 is 79,
   Samsung's 2021 sets 76) and `pnpm screen-check` covers 79; anything older is best effort. Turn off the TV's
   automatic power-off and its screen saver (usually under General, Eco or Power settings), or it will switch off
   after a few hours with no remote presses.

M2 names the device in its results, so note its make, model and browser version.

## 3. The evening log

M2's check at the window: on one or two clear evenings, log 20 planes you see through the window, with the screen on.
For each plane, one line in a text file:

```text
2026-11-02
19:41:05  30  20  y
19:44:50  80  55  y  climbing, lights only
19:52:12  10  5   n  low, behind the tree?
```

- the time to the second, from the phone's clock;
- how far across the window it was, 0 at the left edge to 100 at the right;
- how far up, 0 at the roofs to 100 at the top edge;
- whether the screen showed it at that moment (y or n), and a note if anything was odd.

Each line is compared with the positions stored for that second (`data/history`): at least 18 of 20 must have been on
screen, each within 15° of where it was seen (15 on the 0–100 scale for a 100° window). While the screen runs,
`/api/predictions` keeps scoring "coming next" on its own.

## 4. Heads-ups on the phone

1. In Telegram, message **@BotFather**, send `/newbot`, and choose a name and a username ending in `bot`. It answers
   with a token like `123456789:AA...`. The token is a password.
2. Open a chat with the new bot and send it anything: a bot can only write to someone who wrote to it first.
3. Find the chat id: open `https://api.telegram.org/bot<token>/getUpdates` in a browser; it is the number after
   `"chat":{"id":`.
4. In `.env`:

   ```sh
   OVERHEAD_TELEGRAM_TOKEN=123456789:AA...
   OVERHEAD_TELEGRAM_CHAT=987654321
   ```

5. `docker compose up -d headsup`.

**Check:** `curl -s "https://api.telegram.org/bot<token>/sendMessage" -d chat_id=<chat id> -d text=overhead` reaches the
phone. If the token leaks, `/revoke` it in BotFather and put the new one in `.env`.

## 5. AWS and Snowflake

A test, not a home for the stack ([SPEC: Cost](SPEC.md#event-flow-rabbitmq)): open both accounts on the same day, once `data/history` holds seven
consecutive days, run M3, M4 and M6 in one session, then `make down`. The Snowflake trial lasts 30 days and needs no
card; the AWS parts cost a few dollars at most, and the US$5 budget alert is the first thing `make up` creates.

**AWS**

1. Create an account at aws.amazon.com (it asks for a card) and turn on MFA for the root user.
2. IAM → Users → Create user `overhead-terraform`, with the `AdministratorAccess` policy attached: Terraform creates
   IAM roles, ECS, ECR, S3 and a budget, and the account holds nothing else.
3. On that user, Security credentials → Create access key → Command Line Interface.
4. On the machine that runs `make up`, install the AWS CLI v2 and store the key in a profile; it lands in
   `~/.aws/credentials`:

   ```sh
   aws configure --profile overhead   # the key, the secret, region ca-central-1, output json
   ```

5. After `make down`, deactivate the key (same page as step 3).

**Snowflake**

1. Start a trial at signup.snowflake.com: any edition, cloud **AWS**, region **Canada (Central)**, the bucket's region.
2. Make a key pair for Terraform on the machine that runs `make up` (Snowflake service users sign in with a key, not a
   password):

   ```sh
   mkdir -p ~/.config/overhead && cd ~/.config/overhead
   openssl genrsa 2048 | openssl pkcs8 -topk8 -nocrypt -out snowflake.p8
   openssl rsa -in snowflake.p8 -pubout -out snowflake.pub
   chmod 600 snowflake.p8
   ```

3. In Snowsight, open a SQL worksheet as ACCOUNTADMIN and create the user, pasting `snowflake.pub` without its
   BEGIN and END lines, on one line. ACCOUNTADMIN because the storage integration needs it:

   ```sql
   CREATE USER overhead_tf TYPE = SERVICE DEFAULT_ROLE = ACCOUNTADMIN RSA_PUBLIC_KEY = 'MIIBIjANBgkqh...';
   GRANT ROLE ACCOUNTADMIN TO USER overhead_tf;
   ```

4. Note the organization name and the account name: Snowsight → your name, bottom left → Account → View account
   details. The account identifier is `ORGNAME-ACCOUNTNAME`.

**Running it.** Put the settings in `~/.config/overhead/cloud.env` (`chmod 600`):

```sh
AWS_PROFILE=overhead
TF_VAR_budget_email=you@example.com
SNOWFLAKE_ORGANIZATION_NAME=ORGNAME
SNOWFLAKE_ACCOUNT_NAME=ACCOUNTNAME
SNOWFLAKE_USER=OVERHEAD_TF
SNOWFLAKE_AUTHENTICATOR=SNOWFLAKE_JWT
```

then

```sh
set -a; . ~/.config/overhead/cloud.env; set +a
export SNOWFLAKE_PRIVATE_KEY="$(cat ~/.config/overhead/snowflake.p8)"
make up      # also builds the image and pushes it to ECR: needs Docker and a few GB of disk
make down    # the same day
```

## 6. The public page

The page is the screen as a static site. Its planes come from `live-public`, a second instance of the live API that
only gets what section 7 allows. No server is needed: the stack's machine runs it, a tunnel reaches it, and the page
plays its recording whenever the machine is off. Anyone can read this instance's centre, window and height: that is
what it shows. To keep your own home out of it, set its `OVERHEAD_LAT`, `OVERHEAD_LON` and window to a public spot.

1. Name the feed it may show, in `.env`:

   ```sh
   OVERHEAD_PUBLIC_FEED=adsb.lol
   ```

2. Start it. It listens on `127.0.0.1:8091` only, and has no `.env`, no data volume and a read-only filesystem:

   ```sh
   docker compose --profile public up -d
   curl -s 127.0.0.1:8091/api/now | head -c 300
   ```

3. Open a tunnel to that port and nothing else. With [Tailscale Funnel](https://tailscale.com/kb/1223/funnel) (free,
   and your home address stays hidden behind Tailscale's relays), on a port of its own so that anything already on 443
   is left alone:

   ```sh
   tailscale funnel --bg --https=8443 http://127.0.0.1:8091
   tailscale funnel --https=8443 off   # to close it again
   ```

4. Build the page with that address in it and publish it (the Netlify CLI, logged in):

   ```sh
   make site        # or: make site OVERHEAD_PUBLIC_API=https://other.host
   ```

5. A new recording, at a busy hour; 25 minutes is about 1 MB:

   ```sh
   (cd web && pnpm record-replay --minutes 25)
   make site
   ```

**Check:** open the page. With the tunnel open it says "The planes are live"; with `live-public` stopped it says "a
recording" within seconds, and goes back by itself when it returns.

## 7. What the sources allow on a public page

Read on 2026-11-05. This is a reading of their terms, not legal advice.

| Source | What it says | What the public page does |
|---|---|---|
| adsb.lol | "The license for the API as well as all data ADSB.lol makes public is ODbL"; "If you want to use the API for production purposes, please contact me" | shows only this feed's own reports, with the ODbL notice; the recording is offered under the ODbL |
| adsb.fi | "open data is for personal, non-commercial use only"; "You must cite adsb.fi and include a link to our home page" | not used: a public page is not clearly personal, and its data cannot be relicensed to merge under the ODbL. It may still hide a plane: its private and military flags count, though nothing it says is shown |
| adsbdb | flight route data "may not be copied, published, or incorporated into other databases without the explicit permission of David J Taylor, Edinburgh" | no routes in any response or in the recording; a route only decides, unseen, whether a plane belongs to the followed airport. No such limit was found on its aircraft data, which is shown |
| planespotters.net | its pages refuse scripted reads (403), so its terms are unread | no photos |
| Open-Meteo | free for "Private or non-profit websites or apps that do not have subscriptions or advertising"; "You must include a link next to any location Open-Meteo data are displayed" and link the licence (CC BY 4.0) | credited with both links, and says the clouds are drawn from it |
| aviationweather.gov | "rate limited to 100 requests per minute"; "Set a custom user agent"; US government data, public domain | one request every 10 minutes, with a user agent |
| OpenStreetMap | "If you are distributing OSM in data form, please name and link directly to the license(s)" | "Map © OpenStreetMap contributors (ODbL)" on screen; the extract's licence in [NOTICE.md](../NOTICE.md) |

**What only you can do.** Each one lets the page show more, or makes what it shows firmer. Nothing here is urgent: the
page keeps to what is plainly allowed until you have an answer.

1. **adsb.lol: tell the owner.** They ask to be contacted about production use. Say what overhead is (a free hobby page,
   no ads, one request every 15 s for a 60 km circle, the same rate whatever the number of viewers), with the page's
   address. Contact details are on [adsb.lol](https://adsb.lol) and in its
   [GitHub project](https://github.com/adsblol/api).
2. **adsb.fi (optional): ask whether a free public page counts as "personal, non-commercial".** With a yes, and a way
   past the licence mix, the public page could update every 5 s, not every 15 s.
3. **adsbdb's route data: ask David J Taylor** for permission to show routes on the page, which is the difference
   between "POE2262" and "Boston to Billy Bishop". The same letter can ask whether a day-long private cache of them
   (`data/enrich.db`) is fine: "incorporated into other databases" may cover it. With a yes:
   `OVERHEAD_PUBLIC_ROUTES=on` in `.env`, then `docker compose --profile public up -d`.
4. **planespotters.net: read the terms in a browser**, [photo API](https://www.planespotters.net/photo/api) and
   [terms of use](https://www.planespotters.net/legal/termsofuse), and check hotlinked thumbnails with a linked
   photographer credit are allowed on a free public page. If so: `OVERHEAD_PUBLIC_PHOTOS=on`.

**airplanes.live** (optional, a third feed at home): it answers 403 to projects it has not approved. Email
contact@airplanes.live saying what overhead is, that it is personal and non-commercial, the request rate (one request
every 5 s for a 60 km circle) and the User-Agent it sends. Once approved, run the poller with
`-feeds adsb.lol,adsb.fi,airplanes.live` (in compose: the `poll` service's `command`).

## 8. Development

The broker integration tests use a vhost of their own, created once:

```sh
docker compose exec rabbitmq sh -c 'rabbitmqctl add_vhost overhead-test; rabbitmqctl set_permissions -p overhead-test overhead ".*" ".*" ".*"'
OVERHEAD_TEST_AMQP=amqp://overhead:overhead@localhost:5673/overhead-test make test
```
