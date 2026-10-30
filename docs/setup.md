# Setting up overhead at home

overhead runs without any of this: `docker compose up -d` with no `.env` watches the 60 km circle round Toronto
Pearson and shows the whole sky. Each section below makes one more part real, and the milestones that are still open
(README: Milestones) need them. Each says what to get, where it goes and how to check it.

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
| [6. The public demo server](#6-the-public-demo-server) | M7 |
| [7. Before anything is public](#7-before-anything-is-public) | M7 |

## 1. Home and window

Everything in `.env`:

| Setting | What it is | How to find it |
|---|---|---|
| `OVERHEAD_LAT`, `OVERHEAD_LON` | home | right-click your building on a map and copy the coordinates; three decimals (about 100 m) is plenty, since 100 m moves a plane 10 km away by under 1° |
| `OVERHEAD_HOME_ELEV` | the window's height above sea level, in metres | the ground's height from a map that shows elevation (Google Earth shows it under the pointer), plus about 3 m a floor |
| `OVERHEAD_WINDOW_AZ`, `OVERHEAD_WINDOW_FOV` | the direction the window faces and how wide a slice of sky it shows, in degrees | from where you sit, below |
| `OVERHEAD_WINDOW_MIN_EL` | the lowest angle you can see, over the roofs opposite | from where you sit, below |
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
3. Apply it: `docker compose up -d` recreates the services whose settings changed.

**Check:** the next time a plane crosses the window, it should be on the screen at the same place. If planes are
always a little to one side, move `OVERHEAD_WINDOW_AZ` by that many degrees; if they appear on the screen while
still hidden behind the roofs, raise `OVERHEAD_WINDOW_MIN_EL`. (README item 9's setup view, a full-sky strip to do
this against every plane round home, is not built yet.)

Home's coordinates are private: the live API serves them, so port 8090 stays on the home network.

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

A test, not a home for the stack (README: Cost): open both accounts on the same day, once `data/history` holds seven
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

## 6. The public demo server

M7 runs the same stack on a small rented server, in `-public` mode (commercial flights only) and centred on a public
spot by the airport, never on home's `.env`.

1. A server with 2 vCPU and 4 GB, Ubuntu 24.04, anywhere (the feeds are worldwide). On Hetzner Cloud: create a
   project, then a server, and add the deploying machine's SSH public key (`cat ~/.ssh/id_ed25519.pub`) while
   creating it.
2. A firewall that lets in 22, 80 and 443 only.
3. For HTTPS, a domain or subdomain with an A record pointing at the server.

## 7. Before anything is public

- **planespotters.net's terms**: their site refuses scripted fetches, so read them in a browser and check that
  hotlinked thumbnails with a linked photographer credit are allowed for a public, non-commercial page.
- **The aircraft type facts** in `web/lib/aircraft.ts` are written from general knowledge, not a cited source: read them
  and fix or cut any that are wrong.
- **airplanes.live** (optional, a third feed): it answers 403 to projects it has not approved. Email
  contact@airplanes.live saying what overhead is, that it is personal and non-commercial, the request rate (one
  request every 5 s for a 60 km circle) and the User-Agent it sends. Once approved, run the poller with
  `-feeds adsb.lol,adsb.fi,airplanes.live` (in compose: the `poll` service's `command`).
