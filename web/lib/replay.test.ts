import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { check, compact } from "../scripts/record-replay.mjs";
import { frameAt, type Recording } from "./replay";
import type { Plane, View } from "./types";

const plane = (hex: string, at: string, extra: Partial<Plane> = {}): Plane => ({
  hex,
  callsign: "POE123",
  lat: 43.6,
  lon: -79.4,
  source: "adsb.lol",
  at,
  distance_km: 1.23456789,
  bearing_deg: 180,
  elevation_deg: -5,
  offset_deg: 0,
  in_view: true,
  focus: true,
  info: { hex, type_code: "DH8D", owner: "Porter Airlines" },
  ...extra,
});

const view = (at: string, planes: Plane[], extra: Partial<View> = {}): View => ({
  at,
  center: { lat: 43.64, lon: -79.39 },
  radius_km: 60,
  window: { azimuth_deg: 163, fov_deg: 100, min_elevation_deg: -10, home_elevation_m: 218 },
  planes,
  today: 1,
  sources: ["adsb.lol"],
  facts: [],
  public: true,
  ...extra,
});

const t0 = "2026-11-05T18:00:00.000Z";
const t2 = "2026-11-05T18:00:02.000Z";

describe("a recording", () => {
  const rec = compact([view(t0, [plane("aaaaaa", t0)]), view(t2, [plane("aaaaaa", t2)])], 2);

  it("keeps each aircraft's details once and rounds its numbers", () => {
    expect(Object.keys(rec.aircraft)).toEqual(["aaaaaa"]);
    expect(rec.frames[0]!.planes[0]!.info).toBeUndefined();
    expect(rec.frames[0]!.planes[0]!.distance_km).toBe(1.23457);
  });

  it("plays as a live snapshot of now", () => {
    const now = new Date("2026-12-01T12:00:00.000Z");
    const v = frameAt(rec, 2500, now);
    expect(v.at).toBe(now.toISOString());
    expect(v.public).toBe(true);
    expect(v.planes[0]!.at).toBe(now.toISOString()); // reported at the frame's own moment
    expect(v.planes[0]!.info?.owner).toBe("Porter Airlines");
  });

  it("loops", () => {
    const now = new Date();
    expect(frameAt(rec, 4000, now).planes).toEqual(frameAt(rec, 0, now).planes);
  });
});

describe("what may be recorded", () => {
  it("is the public instance's view", () => {
    expect(() => check(view(t0, []))).not.toThrow();
    expect(() => check(view(t0, [], { public: false }))).toThrow(/public instance/);
  });

  it("is one feed's data", () => {
    expect(() => check(view(t0, [], { sources: ["adsb.fi", "adsb.lol"] }))).toThrow(/adsb\.fi/);
  });

  it("is commercial flights", () => {
    for (const p of [
      plane("a", t0, { callsign: "N123AB" }),
      plane("a", t0, { callsign: undefined }),
      plane("a", t0, { private: true }),
      plane("a", t0, { military: true }),
    ]) {
      expect(() => check(view(t0, [p]))).toThrow(/commercial/);
    }
  });

  it("has no routes or photos", () => {
    const origin = { icao: "CYTZ" };
    for (const info of [
      { hex: "a", route: { callsign: "POE123", origin, destination: origin } },
      { hex: "a", photo: { thumbnail: "t", link: "l", photographer: "p" } },
    ]) {
      expect(() => check(view(t0, [plane("a", t0, { info })]))).toThrow(/route or a photo/);
    }
    expect(() => check(view(t0, [plane("a", t0, { eta_min: 3 })]))).toThrow(/route or a photo/);
  });
});

// The recording goes on the web and into the repo for good, so every frame of it is held to the same rule in CI.
describe("the committed recording", () => {
  const rec = JSON.parse(
    readFileSync(new URL("../public/replay.json", import.meta.url), "utf8"),
  ) as Recording;

  it("holds only what may be published", () => {
    expect(rec.frames.length).toBeGreaterThan(100);
    expect(JSON.stringify(rec)).not.toMatch(/plnspttrs|planespotters|"route"|"photo"|adsb\.fi/);
    rec.frames.forEach((_, i) => {
      check(frameAt(rec, i * rec.step_s * 1000, new Date()));
    });
  });

  it("has planes to show", () => {
    const focused = rec.frames.filter((f) => f.planes.some((p) => p.focus && p.in_view));
    expect(focused.length / rec.frames.length).toBeGreaterThan(0.25);
  });
});
