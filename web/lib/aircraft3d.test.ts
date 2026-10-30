import { describe, expect, it } from "vitest";
import { attitude, draw, lookFor, viewBasis } from "./aircraft3d";
import type { Plane } from "./types";

const plane = (extra: Partial<Plane>): Plane => ({
  hex: "c0ffee",
  lat: 43.62,
  lon: -79.39,
  source: "adsb.fi",
  at: "2026-10-30T18:00:00Z",
  distance_km: 2,
  bearing_deg: 0,
  elevation_deg: 0,
  offset_deg: 0,
  in_view: true,
  ...extra,
});

// The area of an SVG polygon's points, in square pixels.
function area(points: string): number {
  const p = points.split(" ").map((xy) => xy.split(",").map(Number) as [number, number]);
  let s = 0;
  for (let i = 0; i < p.length; i++) {
    const [x1, y1] = p[i]!;
    const [x2, y2] = p[(i + 1) % p.length]!;
    s += x1 * y2 - x2 * y1;
  }
  return Math.abs(s) / 2;
}

describe("planes in 3D", () => {
  it("knows a plane's kind, size and tail colour", () => {
    const dash8 = lookFor(plane({ type_code: "DH8D", callsign: "PTR2943" }));
    expect([dash8.kind, dash8.highWing, dash8.lengthM, dash8.fin]).toEqual([
      "turboprop",
      true,
      32.8,
      "#173a63",
    ]);
    expect(lookFor(plane({ type_code: "A320", callsign: "ACA123" })).kind).toBe("jet");
    expect(lookFor(plane({ type_code: "A333" })).kind).toBe("widebody");
    expect(lookFor(plane({ type_code: "B38M" })).kind).toBe("jet");
    expect(lookFor(plane({ type_code: "CRJ9" })).kind).toBe("rearjet");
    expect(lookFor(plane({ type_code: "R44", callsign: "CGKLD" })).kind).toBe("heli");
    expect(lookFor(plane({ category: "A5" })).kind).toBe("widebody");
    expect(lookFor(plane({ type_code: "C172" })).highWing).toBe(true);
  });

  it("pitches to its descent and banks into its turn", () => {
    // 700 ft/min down at 120 kt is a 3.3° descent; the wing flies 2.5° nose-up of it.
    const a = attitude(plane({ track_deg: 251, speed_kt: 120, vert_rate_fpm: -700 }), 3);
    expect(a.headingDeg).toBe(251);
    expect(a.pitchDeg).toBeCloseTo(-0.8, 1);
    // A 3°/s turn at 120 kt needs about 18° of bank.
    expect(a.rollDeg).toBeCloseTo(18.2, 0);
    expect(attitude(plane({ on_ground: true, track_deg: 70 }))).toEqual({
      headingDeg: 70,
      pitchDeg: 0,
      rollDeg: 0,
    });
  });

  it("looks along the line of sight with right and up square to it", () => {
    const north = viewBasis({ bearingDeg: 0, elevationDeg: 0 });
    expect(north.right.map((v) => Math.round(v * 1000) / 1000)).toEqual([1, -0, 0]);
    expect(north.up).toEqual([-0, -0, 1]);
    const b = viewBasis({ bearingDeg: 163, elevationDeg: -6 });
    const dot = (u: number[], v: number[]) => u.reduce((s, x, i) => s + x * v[i]!, 0);
    expect(dot(b.right, b.along)).toBeCloseTo(0, 9);
    expect(dot(b.up, b.along)).toBeCloseTo(0, 9);
    expect(dot(b.up, b.up)).toBeCloseTo(1, 9);
  });

  it("draws the nose the way the plane flies, and its wings spread when seen from above", () => {
    const look = lookFor(plane({ type_code: "DH8D", callsign: "PTR2943" }));
    const level = { headingDeg: 90, pitchDeg: 0, rollDeg: 0 };
    // Looking north at a plane flying east: its landing light (on the nose) is on the right.
    const side = draw(look, level, { bearingDeg: 0, elevationDeg: 0 }, null, 100, false);
    expect(side.lights.landing[0]).toBeGreaterThan(30);
    // Side-on its wingtips line up; from 35° above, they spread up and down the screen.
    const spread = (d: ReturnType<typeof draw>) => Math.abs(d.lights.port[1] - d.lights.starboard[1]);
    const above = draw(look, level, { bearingDeg: 0, elevationDeg: -35 }, null, 100, false);
    expect(spread(side)).toBeLessThan(8);
    expect(spread(above)).toBeGreaterThan(40);
    expect(above.faces.every((f) => area(f.points) >= 0)).toBe(true);
  });

  it("puts the red light on the left wing and the glare on a plane coming at you", () => {
    const look = lookFor(plane({ type_code: "A320" }));
    const sight = { bearingDeg: 0, elevationDeg: 0 };
    const away = draw(look, { headingDeg: 0, pitchDeg: 0, rollDeg: 0 }, sight, null, 100, true);
    expect(away.lights.port[0]).toBeLessThan(0);
    expect(away.glare).toBe(0);
    const towards = draw(look, { headingDeg: 180, pitchDeg: 0, rollDeg: 0 }, sight, null, 100, true);
    expect(towards.lights.port[0]).toBeGreaterThan(0);
    expect(towards.glare).toBeCloseTo(1, 2);
  });
});
