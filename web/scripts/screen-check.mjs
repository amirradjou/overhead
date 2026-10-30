#!/usr/bin/env node
// screen-check: look at the screen the way a TV will, without the TV or the live API.
//
// It serves the built screen (web/out) with a made-up /api/events stream, opens it in the Chromium you name (an old
// one stands in for a TV browser: webOS 6 runs Chromium 79), and for each scene fixes the page's clock at a time of
// day, waits, saves a screenshot and checks the page: no script errors, the planes on screen, every word of the
// overlay inside the TV-safe area and no text smaller than the 10-foot minimum, and no layout work per frame while
// planes move. It exits non-zero when a check fails.
//
//   pnpm build
//   CHROME=/path/to/chrome node scripts/screen-check.mjs [--out dir] [--only night,day] [--size 1920x1080] [--no-sandbox]
//   node scripts/screen-check.mjs --serve 3999      # just the fake live API and screen, for a browser by hand
//
// Chromium 79 for Linux: `npx @puppeteer/browsers install chromium@706915`.
import { spawn } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import { extname, join, normalize, resolve, sep } from "node:path";
import { parseArgs } from "node:util";

const { values: args } = parseArgs({
  options: {
    out: { type: "string", default: "screen-check" },
    only: { type: "string" },
    size: { type: "string", default: "1920x1080" },
    serve: { type: "string" },
    chrome: { type: "string" },
    tz: { type: "string", default: "America/Toronto" },
    // Old Chromium builds crash their GPU process inside the sandbox on current kernels; the page is our own.
    "no-sandbox": { type: "boolean", default: false },
  },
});

const webOut = resolve(import.meta.dirname, "..", "out");
const home = { lat: 43.65, lon: -79.38 };
const window_ = { azimuth_deg: 200, fov_deg: 100, min_elevation_deg: 8, home_elevation_m: 0 };

// Scenes: a time of day (UTC) and what is flying. The times are for Toronto on 2026-10-30, when the waning moon
// crosses the window before dawn and the sun sets inside it.
const scenes = [
  { name: "night", at: "2026-10-30T06:30:00Z", traffic: "busy" },
  { name: "moon", at: "2026-10-30T10:40:00Z", traffic: "coming" },
  { name: "day", at: "2026-10-30T17:00:00Z", traffic: "busy" },
  { name: "golden", at: "2026-10-30T21:45:00Z", traffic: "busy" },
  { name: "twilight", at: "2026-10-30T22:25:00Z", traffic: "empty" },
];

const airports = {
  YVR: { iata: "YVR", city: "Vancouver", lat: 49.1939, lon: -123.1844 },
  YYC: { iata: "YYC", city: "Calgary", lat: 51.1315, lon: -114.0106 },
  YYZ: { iata: "YYZ", city: "Toronto", lat: 43.6777, lon: -79.6248 },
  SFO: { iata: "SFO", city: "San Francisco", lat: 37.6213, lon: -122.379 },
  ATL: { iata: "ATL", city: "Atlanta", lat: 33.6407, lon: -84.4277 },
};

// A plane crossing the window: it starts at (offset, elevation) and moves (dOffset, dElevation) degrees per 10 s,
// as the real `motion` field says.
function plane(t, p) {
  const offset = p.offset + (p.dOffset * t) / 10;
  const elevation = p.elevation + (p.dElevation * t) / 10;
  const inView = Math.abs(offset) <= window_.fov_deg / 2 && elevation >= window_.min_elevation_deg;
  return {
    hex: p.hex,
    callsign: p.callsign,
    type_code: p.type,
    lat: p.lat,
    lon: p.lon,
    alt_ft: p.alt,
    speed_kt: p.kt,
    vert_rate_fpm: p.fpm,
    source: "adsb.fi",
    at: new Date().toISOString(),
    distance_km: p.km,
    bearing_deg: window_.azimuth_deg + offset,
    elevation_deg: elevation,
    offset_deg: offset,
    in_view: inView,
    motion: { offset_deg: p.dOffset, elevation_deg: p.dElevation },
    ...(p.eta && { to_destination_km: p.eta * 7, eta_min: p.eta }),
    ...(p.notable && { notable: p.notable }),
    info: {
      hex: p.hex,
      type_code: p.type,
      type_name: p.typeName,
      manufacturer: p.maker,
      route: {
        callsign: p.callsign,
        airline: p.airline,
        origin: airports[p.from],
        destination: airports[p.to],
      },
      ...(p.photo && {
        photo: { thumbnail: "/screen-check/photo.svg", link: "#", photographer: "A. Spotter" },
      }),
    },
  };
}

function snapshot(traffic, t) {
  const planes = [];
  if (traffic === "busy") {
    planes.push(
      plane(t, {
        hex: "c0ffee",
        callsign: "ACA123",
        airline: "Air Canada",
        from: "YVR",
        to: "YYZ",
        type: "B38M",
        typeName: "737 MAX 8",
        maker: "Boeing",
        alt: 6200,
        kt: 250,
        fpm: -800,
        km: 9,
        lat: 43.62,
        lon: -79.45,
        offset: -40,
        elevation: 20,
        dOffset: 3,
        dElevation: 0.3,
        eta: 4,
      }),
      plane(t, {
        hex: "abc123",
        callsign: "WJA456",
        airline: "WestJet",
        from: "YYC",
        to: "YYZ",
        type: "B38M",
        typeName: "737 MAX 8",
        maker: "Boeing",
        alt: 9800,
        kt: 280,
        fpm: -1200,
        km: 14,
        lat: 43.55,
        lon: -79.3,
        offset: 30,
        elevation: 38,
        dOffset: -3,
        dElevation: -0.2,
        photo: true,
      }),
      plane(t, {
        hex: "def456",
        callsign: "POE1712",
        airline: "Porter Airlines",
        from: "YYZ",
        to: "SFO",
        type: "E295",
        typeName: "ERJ 190-400",
        maker: "Embraer S.A.",
        alt: 4100,
        kt: 220,
        fpm: 1800,
        km: 6,
        lat: 43.6,
        lon: -79.5,
        offset: 10,
        elevation: 14,
        dOffset: -2,
        dElevation: 0.4,
      }),
    );
  }
  if (traffic !== "empty") {
    planes.push({
      ...plane(t, {
        hex: "aaa111",
        callsign: "DAL55",
        airline: "Delta Air Lines",
        from: "ATL",
        to: "YYZ",
        type: "A321",
        typeName: "A321-211",
        maker: "Airbus",
        alt: 11000,
        kt: 300,
        fpm: -500,
        km: 25,
        lat: 43.5,
        lon: -79.9,
        offset: 60,
        elevation: 7,
        dOffset: 0,
        dElevation: 0,
      }),
      in_view: false,
      coming: { in_s: 90, side: "right", offset_deg: 50, elevation_deg: 12 },
    });
  }
  return {
    at: new Date().toISOString(),
    center: home,
    radius_km: 60,
    window: window_,
    planes,
    today: 42,
    sources: ["adsb.fi", "adsb.lol"],
    facts: ["The busiest hour this week was 18:00, with 31 planes past the window."],
  };
}

const types = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript",
  ".css": "text/css",
  ".woff2": "font/woff2",
  ".svg": "image/svg+xml",
  ".txt": "text/plain",
  ".json": "application/json",
  ".ico": "image/x-icon",
};

const photo = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 300 200"><rect width="300" height="200" fill="#5d7b96"/><path d="M40 120h220l-30 12H70z" fill="#d9dee4"/></svg>`;

// serve starts the fake live API and the static screen; scene() picks what the stream sends.
function serve(port) {
  let traffic = "busy";
  const streams = new Set();
  const server = createServer((req, res) => {
    const path = decodeURIComponent(new URL(req.url ?? "/", "http://x").pathname);
    if (path === "/api/events") {
      res.writeHead(200, { "Content-Type": "text/event-stream", "Cache-Control": "no-cache" });
      const start = Date.now();
      const send = () =>
        res.write(
          `event: snapshot\ndata: ${JSON.stringify(snapshot(traffic, (Date.now() - start) / 1000))}\n\n`,
        );
      send();
      const id = setInterval(send, 2000);
      streams.add(res);
      res.on("close", () => {
        clearInterval(id);
        streams.delete(res);
      });
      return;
    }
    if (path === "/screen-check/photo.svg") {
      res.writeHead(200, { "Content-Type": types[".svg"] });
      res.end(photo);
      return;
    }
    let file = normalize(join(webOut, path === "/" ? "index.html" : path));
    if (!file.startsWith(webOut + sep)) {
      res.writeHead(403).end();
      return;
    }
    if (!existsSync(file) && existsSync(file + ".html")) file += ".html";
    try {
      const body = readFileSync(file);
      res.writeHead(200, { "Content-Type": types[extname(file)] ?? "application/octet-stream" });
      res.end(body);
    } catch {
      res.writeHead(404).end();
    }
  });
  return new Promise((ok) =>
    server.listen(port, "127.0.0.1", () =>
      ok({
        port: server.address().port,
        setTraffic: (t) => (traffic = t),
        close: () => {
          for (const s of streams) s.destroy();
          server.closeAllConnections();
          server.close();
        },
      }),
    ),
  );
}

// cdp is a minimal DevTools-protocol client over the browser's page WebSocket.
async function cdp(wsUrl) {
  const ws = new WebSocket(wsUrl);
  let id = 0;
  const pending = new Map();
  const listeners = [];
  ws.onmessage = (m) => {
    const msg = JSON.parse(m.data);
    if (msg.id && pending.has(msg.id)) {
      const { ok, fail } = pending.get(msg.id);
      pending.delete(msg.id);
      msg.error ? fail(new Error(`${msg.error.message} (${msg.error.code})`)) : ok(msg.result);
    } else if (msg.method) {
      for (const l of listeners) l(msg);
    }
  };
  await new Promise((ok, fail) => {
    ws.onopen = ok;
    ws.onerror = () => fail(new Error("cannot reach the browser's page: did it crash? (try --no-sandbox)"));
    setTimeout(() => fail(new Error("the browser's page did not answer")), 10_000);
  });
  return {
    send: (method, params = {}) =>
      new Promise((ok, fail) => {
        id += 1;
        pending.set(id, { ok, fail });
        ws.send(JSON.stringify({ id, method, params }));
      }),
    on: (l) => listeners.push(l),
    close: () => ws.close(),
  };
}

async function waitFor(fn, ms) {
  const end = Date.now() + ms;
  for (;;) {
    try {
      return await fn();
    } catch (e) {
      if (Date.now() > end) throw e;
      await new Promise((r) => setTimeout(r, 200));
    }
  }
}

// The checks run in the page. They return a list of problems, empty when all is well.
const pageChecks = `(() => {
  const vw = innerWidth, vh = innerHeight, problems = [];
  const root = getComputedStyle(document.documentElement);
  const px = (v, size) => v.trim().endsWith("%") ? parseFloat(v) / 100 * size : parseFloat(v);
  const safeX = px(root.getPropertyValue("--safe-x") || "0", vw), safeY = px(root.getPropertyValue("--safe-y") || "0", vh);
  const drift = 14; // the overlay drifts a few pixels
  const minFont = 23 * Math.min(vw / 1920, vh / 1080) - 0.5; // tvOS's minimum, 23 pt at 1080p
  let smallest = Infinity;
  const walker = document.createTreeWalker(document.querySelector(".screen"), NodeFilter.SHOW_TEXT);
  while (walker.nextNode()) {
    const node = walker.currentNode, el = node.parentElement;
    if (!node.textContent.trim() || !el || el.closest("svg")) continue;
    const style = getComputedStyle(el);
    if (style.visibility === "hidden" || style.display === "none") continue;
    const size = parseFloat(style.fontSize);
    smallest = Math.min(smallest, size);
    if (size < minFont) problems.push("text under " + minFont.toFixed(1) + "px: " + size + "px " + JSON.stringify(node.textContent.trim().slice(0, 40)));
    if (!el.closest(".overlay")) continue;
    const range = document.createRange();
    range.selectNodeContents(node);
    for (const r of range.getClientRects()) {
      if (r.left < safeX - drift || r.right > vw - safeX + drift || r.top < safeY - drift || r.bottom > vh - safeY + drift)
        problems.push("outside the safe area: " + JSON.stringify(node.textContent.trim().slice(0, 40)) + " at " + [r.left, r.top, r.right, r.bottom].map(Math.round).join(","));
    }
  }
  return JSON.stringify({
    problems,
    crafts: document.querySelectorAll(".craft").length,
    smallestText: smallest,
    caption: (document.querySelector(".caption") || {}).innerText,
  });
})()`;

async function check(chrome) {
  const [width, height] = args.size.split("x").map(Number);
  const server = await serve(0);
  const profile = mkdtempSync(join(tmpdir(), "screen-check-"));
  const debugPort = 9300 + Math.floor(Math.random() * 500);
  const proc = spawn(
    chrome,
    [
      "--headless",
      "--disable-gpu",
      "--hide-scrollbars",
      "--no-first-run",
      "--no-default-browser-check",
      "--mute-audio",
      ...(args["no-sandbox"] ? ["--no-sandbox"] : []),
      `--remote-debugging-port=${debugPort}`,
      `--user-data-dir=${profile}`,
      `--window-size=${width},${height}`,
      "about:blank",
    ],
    { env: { ...process.env, TZ: args.tz }, stdio: "ignore" },
  );
  const failures = [];
  let page;
  try {
    const targets = await waitFor(async () => {
      const list = await (await fetch(`http://127.0.0.1:${debugPort}/json`)).json();
      if (!list.some((t) => t.type === "page")) throw new Error("no page yet");
      return list;
    }, 15_000);
    page = await cdp(targets.find((t) => t.type === "page").webSocketDebuggerUrl);
    const errors = [];
    page.on((msg) => {
      if (msg.method === "Runtime.exceptionThrown") {
        const d = msg.params.exceptionDetails;
        errors.push(d.exception?.description ?? d.text);
      } else if (msg.method === "Runtime.consoleAPICalled" && msg.params.type === "error") {
        errors.push(msg.params.args.map((a) => a.value ?? a.description).join(" "));
      } else if (msg.method === "Log.entryAdded" && msg.params.entry.level === "error") {
        errors.push(`${msg.params.entry.text} ${msg.params.entry.url ?? ""}`.trim());
      }
    });
    await page.send("Runtime.enable");
    await page.send("Page.enable");
    await page.send("Log.enable");
    await page.send("Performance.enable");
    await page.send("Emulation.setDeviceMetricsOverride", {
      width,
      height,
      deviceScaleFactor: 1,
      mobile: false,
    });
    const version = await page.send("Runtime.evaluate", {
      expression: "navigator.userAgent",
      returnByValue: true,
    });
    console.log(version.result.value);
    mkdirSync(args.out, { recursive: true });

    const only = args.only?.split(",");
    let clockScript;
    for (const scene of scenes.filter((s) => !only || only.includes(s.name))) {
      errors.length = 0;
      server.setTraffic(scene.traffic);
      if (clockScript)
        await page.send("Page.removeScriptToEvaluateOnNewDocument", { identifier: clockScript });
      // The page's clock starts at the scene's time and runs on from there.
      ({ identifier: clockScript } = await page.send("Page.addScriptToEvaluateOnNewDocument", {
        source: `(() => { const R = Date, offset = ${Date.parse(scene.at)} - R.now();
          class D extends R { constructor(...a) { if (a.length) super(...a); else super(R.now() + offset); }
            static now() { return R.now() + offset; } }
          window.Date = D; })();`,
      }));
      await page.send("Page.navigate", { url: `http://127.0.0.1:${server.port}/` });
      await new Promise((r) => setTimeout(r, 5000));
      // Layout work while the planes glide: motion that runs on the compositor needs none between snapshots.
      const metric = async () =>
        Object.fromEntries((await page.send("Performance.getMetrics")).metrics.map((m) => [m.name, m.value]));
      const before = await metric();
      await new Promise((r) => setTimeout(r, 6000));
      const after = await metric();
      const layouts = after.LayoutCount - before.LayoutCount;
      const res = await page.send("Runtime.evaluate", { expression: pageChecks, returnByValue: true });
      const report = JSON.parse(res.result.value);
      const shot = await page.send("Page.captureScreenshot", { format: "png" });
      const file = join(args.out, `${scene.name}.png`);
      writeFileSync(file, Buffer.from(shot.data, "base64"));

      const problems = [...report.problems, ...errors.map((e) => `script error: ${e}`)];
      const wantCrafts = scene.traffic === "busy" ? 3 : 0;
      if (report.crafts !== wantCrafts)
        problems.push(`${report.crafts} planes on screen, want ${wantCrafts}`);
      if (layouts > 12) problems.push(`${layouts} layouts in 6 s: something animates layout`); // a few per snapshot
      console.log(
        `${problems.length ? "FAIL" : "ok  "} ${scene.name.padEnd(9)} ${file}  layouts/6s=${layouts} smallest text=${report.smallestText}px`,
      );
      for (const p of problems) console.log(`       ${p}`);
      failures.push(...problems.map((p) => `${scene.name}: ${p}`));
    }
  } finally {
    page?.close();
    proc.kill();
    server.close();
    await new Promise((r) => setTimeout(r, 300));
    rmSync(profile, { recursive: true, force: true });
  }
  return failures;
}

if (args.serve) {
  const server = await serve(Number(args.serve));
  console.log(`screen and fake live API on http://127.0.0.1:${server.port}/ (Ctrl-C to stop)`);
} else {
  const chrome = args.chrome ?? process.env.CHROME;
  if (!chrome) {
    console.error("name a Chromium: CHROME=/path/to/chrome or --chrome /path/to/chrome");
    process.exit(2);
  }
  if (!existsSync(join(webOut, "index.html"))) {
    console.error(`no build in ${webOut}: run pnpm build first`);
    process.exit(2);
  }
  const failures = await check(chrome);
  if (failures.length) {
    console.log(`\n${failures.length} problem(s)`);
    process.exit(1);
  }
}
