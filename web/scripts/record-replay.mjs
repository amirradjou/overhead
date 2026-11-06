#!/usr/bin/env node
// Records the public instance's view into web/public/replay.json, for the public page to play while the live feed is
// away:
//
//   node scripts/record-replay.mjs --api http://127.0.0.1:8091 --minutes 20
//
// The recording is published data, so it may hold only what the public instance may show: it refuses any stream that
// is not the public one, and any route or photo. See docs/setup.md, section 7.
import { writeFileSync } from "node:fs";
import { pathToFileURL } from "node:url";

const allowedSources = ["adsb.lol"];
const airline = /^[A-Z]{3}[0-9][0-9A-Z]{0,3}$/; // as internal/live: Commercial

/** Numbers to five decimals (about a metre of latitude), which is most of what a recording can lose for free. */
function rounded(value) {
  return JSON.parse(JSON.stringify(value, (_, v) => (typeof v === "number" ? Math.round(v * 1e5) / 1e5 : v)));
}

/** Checks one snapshot may be published, and throws if not: a recording goes on the web and into the repo for good. */
export function check(view) {
  if (!view.public)
    throw new Error("not the public instance: it may hold private planes and other feeds' data");
  const others = view.sources.filter((s) => !allowedSources.includes(s));
  if (others.length) throw new Error(`data from ${others.join(", ")} may not be published`);
  for (const p of view.planes) {
    if (!airline.test(p.callsign ?? "") || p.military || p.private) {
      throw new Error(`${p.hex} is not a commercial flight`);
    }
    if (p.info?.route || p.info?.photo || p.to_destination_km !== undefined || p.eta_min !== undefined) {
      throw new Error(`${p.hex} carries a route or a photo, which may not be published`);
    }
  }
}

/** Snapshots, one every stepS seconds, as a Recording (lib/replay.ts). */
export function compact(views, stepS) {
  const aircraft = {};
  const frames = views.map((view) => {
    check(view);
    const planes = view.planes.map(({ info, ...plane }) => {
      if (info) aircraft[plane.hex] ??= info;
      return plane;
    });
    return { at: view.at, today: view.today, airport: view.airport, planes };
  });
  const { at: _at, today: _today, airport: _airport, planes: _planes, ...rest } = views[0];
  return rounded({ recorded_at: views[0].at, step_s: stepS, view: rest, aircraft, frames });
}

async function record(api, minutes) {
  const stop = new AbortController();
  const timer = setTimeout(() => stop.abort(), minutes * 60_000);
  const views = [];
  try {
    const resp = await fetch(`${api}/api/events`, {
      headers: { "Accept-Encoding": "identity" },
      signal: stop.signal,
    });
    if (!resp.ok) throw new Error(`${api}: ${resp.status}`);
    let pending = "";
    for await (const chunk of resp.body.pipeThrough(new TextDecoderStream())) {
      pending += chunk;
      let end;
      while ((end = pending.indexOf("\n\n")) >= 0) {
        const data = pending
          .slice(0, end)
          .split("\n")
          .find((l) => l.startsWith("data: "));
        pending = pending.slice(end + 2);
        if (!data) continue;
        const view = JSON.parse(data.slice(6));
        check(view);
        views.push(view);
        if (views.length % 30 === 0) console.error(`${views.length} snapshots`);
      }
    }
  } catch (err) {
    if (err.name !== "AbortError") throw err;
  } finally {
    clearTimeout(timer);
  }
  return views;
}

if (import.meta.url === pathToFileURL(process.argv[1]).href) {
  const flag = (name, fallback) => {
    const i = process.argv.indexOf(`--${name}`);
    return i > 0 ? process.argv[i + 1] : fallback;
  };
  const views = await record(flag("api", "http://127.0.0.1:8091"), Number(flag("minutes", "20")));
  if (views.length < 2) throw new Error(`only ${views.length} snapshots`);
  const stepS = Math.round(
    (Date.parse(views.at(-1).at) - Date.parse(views[0].at)) / 1000 / (views.length - 1),
  );
  const out = flag("out", new URL("../public/replay.json", import.meta.url).pathname);
  writeFileSync(out, JSON.stringify(compact(views, stepS)) + "\n");
  const flown = new Set(views.flatMap((v) => v.planes.filter((p) => p.focus).map((p) => p.hex)));
  console.error(`${out}: ${views.length} snapshots, ${stepS} s apart, ${flown.size} planes in focus`);
}
