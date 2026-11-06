#!/usr/bin/env node
// Records the pictures in the README from the built page, so they can be made again after the screen changes:
//
//   pnpm build
//   CHROME=/path/to/chrome node scripts/record-demo.mjs [--only gif,day,dusk,night,phone] [--no-sandbox]
//
// It plays the page's own recording (web/public/replay.json, the only flight data that may be published) on a clock it
// controls, one frame at a time, so the result is the same every run and as smooth as its frame rate: nothing depends
// on how fast this machine draws. Pictures go to docs/images/. Needs ffmpeg and gifski (`cargo install gifski`).
import { spawn, spawnSync } from "node:child_process";
import { mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import { extname, join, normalize, resolve, sep } from "node:path";
import { parseArgs } from "node:util";

const { values: args } = parseArgs({
  options: {
    only: { type: "string" },
    out: { type: "string", default: resolve(import.meta.dirname, "../../docs/images") },
    chrome: { type: "string" },
    "no-sandbox": { type: "boolean", default: false },
  },
});

const webOut = resolve(import.meta.dirname, "..", "out");
const size = { width: 1920, height: 1080 };

// POE2262's landing on runway 26, as seconds into the recording: announced at the window's edge, on final, down, and
// rolled out. The page's clock is set so it happens in the last of the afternoon's light.
const landing = { from: 326, to: 406, clock: "2026-11-05T21:23:00Z" };
// 33.33 frames a second is what a GIF can truly hold: its delays are whole hundredths of a second, here 3.
const gif = { speed: 5, fps: 100 / 3, fadeFrames: 20, width: 960 };
const stills = [
  { name: "screen-day", clock: "2026-11-05T17:25:00Z", at: 372 },
  { name: "screen-dusk", clock: "2026-11-05T21:55:00Z", at: 372 },
  { name: "screen-night", clock: "2026-11-06T00:30:00Z", at: 548 },
];
const phone = { name: "phone", clock: "2026-11-05T17:25:00Z", at: 372, width: 390, height: 844, dpr: 2 };

/** The page's own clock, timers and animations, moved only by __tick: nothing in the page runs on real time. */
const virtualTime = (start) => `(() => {
  const RealDate = Date, realTimeout = setTimeout, realFrame = requestAnimationFrame;
  let now = ${start}, seq = 1;
  const timers = new Map(), frames = new Map(), started = new WeakMap();
  window.Date = class extends RealDate {
    constructor(...a) { if (a.length) super(...a); else super(now); }
    static now() { return now; }
  };
  performance.now = () => now - ${start};
  Math.random = () => 0.5; // the scene's drift stays put, so only what really moves changes between frames
  window.setTimeout = (fn, ms = 0, ...a) => { const id = seq++; timers.set(id, { at: now + ms, fn: () => fn(...a) }); return id; };
  window.setInterval = (fn, ms = 0, ...a) => { const id = seq++; timers.set(id, { at: now + ms, every: Math.max(1, ms), fn: () => fn(...a) }); return id; };
  window.clearTimeout = window.clearInterval = (id) => timers.delete(id);
  window.requestAnimationFrame = (fn) => { const id = seq++; frames.set(id, fn); return id; };
  window.cancelAnimationFrame = (id) => frames.delete(id);
  const breathe = () => new Promise((r) => realTimeout(r, 0)); // lets React commit, which it does on real tasks
  // A transition starts from where the last frame left things, so it has been running since then. Timed from this
  // frame instead, the plane would stand still for one frame at every snapshot: a hitch you can feel.
  let lastFrame = now;
  const seek = () => {
    for (const a of document.getAnimations()) {
      if (!started.has(a)) { started.set(a, lastFrame); a.pause(); }
      a.currentTime = now - started.get(a);
    }
  };
  window.__tick = async (ms) => {
    lastFrame = now;
    const end = now + ms;
    for (;;) {
      let due = null;
      for (const [id, t] of timers) if (t.at <= end && (!due || t.at < due.t.at)) due = { id, t };
      if (!due) break;
      now = due.t.at;
      if (due.t.every) due.t.at += due.t.every; else timers.delete(due.id);
      due.t.fn();
      await breathe();
    }
    now = end;
    const callbacks = [...frames.values()];
    frames.clear();
    for (const fn of callbacks) fn(now - ${start});
    await breathe();
    await breathe();
    seek();
    await new Promise((r) => realFrame(() => realFrame(r)));
    seek();
  };
})();`;

const types = {
  ".html": "text/html",
  ".js": "text/javascript",
  ".css": "text/css",
  ".json": "application/json",
  ".svg": "image/svg+xml",
  ".woff2": "font/woff2",
  ".jpg": "image/jpeg",
};

/** The built page, with no live API behind it. */
function serve() {
  const server = createServer((req, res) => {
    const path = new URL(req.url, "http://x").pathname;
    const file = normalize(join(webOut, path === "/" ? "index.html" : path));
    try {
      if (!file.startsWith(webOut + sep)) throw new Error("outside");
      const body = readFileSync(file);
      res.writeHead(200, { "content-type": types[extname(file)] ?? "application/octet-stream" });
      res.end(body);
    } catch {
      res.writeHead(404);
      res.end();
    }
  });
  return new Promise((r) => server.listen(0, "127.0.0.1", () => r(server)));
}

async function openBrowser() {
  const chrome = args.chrome ?? process.env.CHROME;
  if (!chrome) throw new Error("name a Chromium: CHROME=/path/to/chrome or --chrome");
  const profile = mkdtempSync(join(tmpdir(), "record-demo-"));
  const proc = spawn(
    chrome,
    [
      "--headless",
      "--disable-gpu",
      "--hide-scrollbars",
      `--user-data-dir=${profile}`,
      "--remote-debugging-port=0",
      ...(args["no-sandbox"] ? ["--no-sandbox"] : []),
      "about:blank",
    ],
    { stdio: ["ignore", "ignore", "pipe"] },
  );
  const url = await new Promise((res, rej) => {
    let log = "";
    proc.stderr.on("data", (d) => {
      log += d;
      const m = log.match(/ws:\/\/\S+/);
      if (m) res(m[0]);
    });
    setTimeout(() => rej(new Error(`no DevTools address from ${chrome}\n${log}`)), 15_000);
  });
  const base = url.replace("ws://", "http://").replace(/\/devtools.*/, "");
  const page = (await (await fetch(`${base}/json/list`)).json()).find((t) => t.type === "page");
  const ws = new WebSocket(page.webSocketDebuggerUrl);
  await new Promise((r) => (ws.onopen = r));
  let id = 0;
  const waiting = new Map();
  ws.onmessage = (e) => {
    const msg = JSON.parse(e.data);
    if (msg.id && waiting.has(msg.id)) {
      waiting.get(msg.id)(msg);
      waiting.delete(msg.id);
    } else if (msg.method === "Runtime.exceptionThrown") {
      throw new Error(`the page failed: ${JSON.stringify(msg.params).slice(0, 400)}`);
    }
  };
  const send = (method, params = {}) =>
    new Promise((res, rej) => {
      waiting.set(++id, (m) => (m.error ? rej(new Error(`${method}: ${m.error.message}`)) : res(m.result)));
      ws.send(JSON.stringify({ id, method, params }));
    });
  return {
    send,
    close() {
      ws.close();
      proc.kill();
      rmSync(profile, { recursive: true, force: true });
    },
  };
}

/** The page opened on the recording, its clock at `clock`, in a window of the given size. */
async function openPage(browser, port, { clock, width = size.width, height = size.height, dpr = 1 }) {
  const { send } = browser;
  const evaluate = async (expression) => {
    const r = await send("Runtime.evaluate", { expression, awaitPromise: true, returnByValue: true });
    if (r.exceptionDetails) throw new Error(JSON.stringify(r.exceptionDetails).slice(0, 400));
    return r.result.value;
  };
  await send("Runtime.enable");
  await send("Page.enable");
  await send("Emulation.setDeviceMetricsOverride", {
    width,
    height,
    deviceScaleFactor: dpr,
    mobile: width < 900,
  });
  await send("Emulation.setTimezoneOverride", { timezoneId: "America/Toronto" });
  const { identifier } = await send("Page.addScriptToEvaluateOnNewDocument", {
    source: virtualTime(Date.parse(clock)),
  });
  await send("Page.navigate", { url: `http://127.0.0.1:${port}/?replay` });
  const end = Date.now() + 15_000;
  while (!(await evaluate(`document.readyState === "complete"`).catch(() => false))) {
    if (Date.now() > end) throw new Error("the page did not load");
    await new Promise((r) => setTimeout(r, 100));
  }
  await new Promise((r) => setTimeout(r, 1500)); // hydrated, and replay.json arrived
  await evaluate("document.fonts.ready.then(() => 1)");
  let t = 0;
  return {
    /** Moves on to s seconds into the recording, a snapshot at a time, so trails build up as they would. */
    async advanceTo(s, step = 500) {
      while (t < s * 1000) {
        const dt = Math.min(step, s * 1000 - t);
        await evaluate(`window.__tick(${dt})`);
        t += dt;
      }
    },
    async screenshot(file, format = "png") {
      const { data } = await send("Page.captureScreenshot", {
        format,
        ...(format === "jpeg" ? { quality: 88 } : {}),
      });
      writeFileSync(file, Buffer.from(data, "base64"));
    },
    close: () => send("Page.removeScriptToEvaluateOnNewDocument", { identifier }),
  };
}

function run(command, list) {
  const r = spawnSync(command, list, { stdio: ["ignore", "ignore", "inherit"] });
  if (r.error) throw new Error(`${command}: ${r.error.message}`);
  if (r.status !== 0) throw new Error(`${command} exited with ${r.status}`);
}

async function recordGif(browser, port) {
  const frames = mkdtempSync(join(tmpdir(), "record-demo-frames-"));
  const page = await openPage(browser, port, { clock: landing.clock });
  await page.advanceTo(landing.from);
  const count = Math.round(((landing.to - landing.from) * gif.fps) / gif.speed);
  const stepS = gif.speed / gif.fps;
  for (let i = 0; i < count; i++) {
    await page.advanceTo(landing.from + stepS * (i + 1), (stepS * 1000) / 2);
    await page.screenshot(join(frames, `f${String(i).padStart(5, "0")}.png`));
    if (i % 50 === 0) console.log(`gif: frame ${i} of ${count}`);
  }
  await page.close();

  // The end dissolves into the start, so the loop has no cut: only the plane and the words differ between the two.
  const lengthS = (count - gif.fadeFrames) / gif.fps;
  const fadeS = gif.fadeFrames / gif.fps;
  const loop = mkdtempSync(join(tmpdir(), "record-demo-loop-"));
  run("ffmpeg", [
    "-y",
    "-loglevel",
    "error",
    "-framerate",
    "100/3",
    "-i",
    join(frames, "f%05d.png"),
    "-filter_complex",
    `[0]split[body][head];` +
      `[body]trim=start_frame=${gif.fadeFrames},setpts=PTS-STARTPTS[rest];` +
      `[head]trim=end_frame=${gif.fadeFrames},setpts=PTS-STARTPTS[start];` +
      `[rest][start]xfade=transition=fade:duration=${fadeS}:offset=${lengthS - fadeS},` +
      `scale=${gif.width}:-2:flags=lanczos`,
    join(loop, "f%05d.png"),
  ]);
  rmSync(frames, { recursive: true, force: true });
  const out = join(args.out, "demo.gif");
  const pngs = readdirSync(loop)
    .sort()
    .map((f) => join(loop, f));
  run("gifski", ["--quiet", "--fps", gif.fps.toFixed(2), "--quality", "88", "-o", out, ...pngs]);
  rmSync(loop, { recursive: true, force: true });
  console.log(`${out}: ${(statSync(out).size / 1e6).toFixed(1)} MB`);
}

async function recordStill(browser, port, still) {
  const page = await openPage(browser, port, still);
  await page.advanceTo(still.at);
  const out = join(args.out, `${still.name}.jpg`);
  await page.screenshot(out, "jpeg");
  await page.close();
  console.log(out);
}

const wanted = (name) => !args.only || args.only.split(",").includes(name);
mkdirSync(args.out, { recursive: true });
const server = await serve();
const { port } = server.address();
const jobs = [
  ...stills.map((s) => [s.name.replace("screen-", ""), (b) => recordStill(b, port, s)]),
  ["phone", (b) => recordStill(b, port, phone)],
  ["gif", (b) => recordGif(b, port)],
].filter(([name]) => wanted(name));
// One browser per picture: each starts its clock from scratch.
for (const [, job] of jobs) {
  const browser = await openBrowser();
  try {
    await job(browser);
  } finally {
    browser.close();
  }
}
server.close();
