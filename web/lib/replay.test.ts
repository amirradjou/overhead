import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { check, compact } from "../scripts/record-replay.mjs";
import { frameAt, smoothed, type Recording } from "./replay";
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

describe("a recording's paths", () => {
  const at = (s: number) => new Date(Date.parse(t0) + s * 1000).toISOString();
  const airport = {
    icao: "CYTZ",
    iata: "YTZ",
    name: "Billy Bishop",
    lat: 43.6275,
    lon: -79.3962,
    elevation_ft: 252,
    arrivals_today: 0,
    departures_today: 0,
  };
  // Reports at 0, 4 and 8 s; the frames in between hold the live API's guess, which sails on from the last report.
  const flight: [number, Partial<Plane>][] = [
    [0, { offset_deg: 0, elevation_deg: -5, speed_kt: 100, alt_ft: 400 }],
    [0, { offset_deg: 9, elevation_deg: -6, speed_kt: 100, alt_ft: 400 }],
    [4, { offset_deg: 10, elevation_deg: -6, speed_kt: 80, alt_ft: 300 }],
    [4, { offset_deg: 19, elevation_deg: -7, speed_kt: 80, alt_ft: 300 }],
    [8, { offset_deg: 12, elevation_deg: 0, speed_kt: 40, on_ground: true, distance_km: 1.3 }],
    [8, { offset_deg: 12, elevation_deg: 0, speed_kt: 40, on_ground: true, distance_km: 1.3 }],
  ];
  const rec = smoothed(
    compact(
      flight.map(([reported, extra], i) =>
        view(
          at(i * 2),
          [
            plane("aaaaaa", at(reported), {
              airport: { op: "arriving", phase: "final", distance_km: 1 },
              ...extra,
            }),
          ],
          { airport },
        ),
      ),
      2,
    ),
  );
  const drawn = rec.frames.map((f) => f.planes[0]!);

  it("goes through each report rather than past it", () => {
    expect(drawn[0]!.offset_deg).toBe(0);
    expect(drawn[1]!.offset_deg).toBeGreaterThan(0);
    expect(drawn[1]!.offset_deg).toBeLessThan(10); // not the guess of 9 + the jump back from 19 at the end
    expect(drawn[2]!.offset_deg).toBe(10);
    expect(drawn[3]!.offset_deg).toBeLessThanOrEqual(12);
  });

  it("never runs back past where it stops", () => {
    const offsets = drawn.map((p) => p.offset_deg);
    expect(offsets).toEqual([...offsets].sort((a, b) => a - b));
  });

  it("slows and descends between reports", () => {
    expect(drawn[1]!.speed_kt).toBeCloseTo(90);
    expect(drawn[1]!.alt_ft).toBeCloseTo(350);
  });

  it("stays on screen after touchdown, on the ground", () => {
    expect(drawn[4]!.on_ground).toBe(true);
    expect(drawn[4]!.in_view).toBe(true);
    expect(drawn[4]!.elevation_deg).toBeLessThan(-5);
  });

  it("is not drawn across a long silence", () => {
    const gap = smoothed(
      compact(
        [
          view(at(0), [plane("aaaaaa", at(0), { offset_deg: 0 })]),
          view(at(60), [plane("aaaaaa", at(0), { offset_deg: 5 })]),
          view(at(120), [plane("aaaaaa", at(120), { offset_deg: 40 })]),
        ],
        60,
      ),
    );
    expect(gap.frames[1]!.planes[0]!.offset_deg).toBe(5);
  });

  describe("on the real recording", () => {
    const real = JSON.parse(readFileSync("public/replay.json", "utf8")) as Recording;
    const out = smoothed(real);
    // The biggest sudden change of speed across the window, between one frame's move and the next, for one flight
    // while it is in the air and in view. A plane may truly move fast close to the window; it may not lurch.
    const biggestLurch = (rec: Recording, callsign: string) => {
      let biggest = 0;
      let last: Plane | undefined;
      let move: number | undefined;
      for (const f of rec.frames) {
        const p = f.planes.find((q) => q.callsign === callsign && q.in_view && !q.on_ground);
        const next = p && last ? p.offset_deg - last.offset_deg : undefined;
        if (next !== undefined && move !== undefined) biggest = Math.max(biggest, Math.abs(next - move));
        move = next;
        last = p;
      }
      return biggest;
    };

    it("keeps every frame and every number", () => {
      expect(out.frames.length).toBe(real.frames.length);
      for (const f of out.frames) {
        for (const p of f.planes) {
          expect(Number.isFinite(p.offset_deg)).toBe(true);
          expect(Number.isFinite(p.elevation_deg)).toBe(true);
        }
      }
    });

    it("takes away the lurch each new report made in a take-off", () => {
      // Live's guess falls behind a plane speeding up, then catches up all at once: from 1 degree a frame to 5.4.
      expect(biggestLurch(real, "JZA7974")).toBeGreaterThan(4);
      expect(biggestLurch(out, "JZA7974")).toBeLessThan(1);
    });

    it("keeps a landing on screen from final to its roll-out", () => {
      const seen = out.frames.flatMap((f) => f.planes.filter((p) => p.callsign === "POE2262" && p.in_view));
      expect(seen.some((p) => p.airport?.phase === "final")).toBe(true);
      expect(seen.some((p) => p.on_ground)).toBe(true);
    });
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
