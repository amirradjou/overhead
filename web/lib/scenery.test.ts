import { describe, expect, it } from "vitest";
import { blocks, clipFront, harbour, projector, ringPath, sceneryFor } from "./scenery";
import { roofline } from "./sky";
import type { Window } from "./types";

// The CN Tower's foot, standing in for a window high over the harbour.
const tower = { lat: 43.6426, lon: -79.3871 };
const high: Window = {
  azimuth_deg: 163,
  fov_deg: 100,
  min_elevation_deg: -10,
  max_elevation_deg: 30,
  home_elevation_m: 218,
};

describe("the harbour from a high window", () => {
  it("is drawn only for a window that looks down on it", () => {
    expect(sceneryFor(tower, high)).toBe(harbour);
    expect(sceneryFor(tower, { ...high, min_elevation_deg: 5 })).toBeNull();
    expect(sceneryFor({ lat: 43.6777, lon: -79.6248 }, high)).toBeNull(); // Pearson: not by the harbour
  });

  it("puts the lake's horizon a third of a degree below level, from 143 m above the water", () => {
    const p = projector(tower, high, harbour.lake_m);
    const dip = -Math.sqrt((2 * (218 - 74.5)) / (6371008.8 * (4 / 3))) * (180 / Math.PI);
    expect(dip).toBeCloseTo(-0.333, 2);
    expect(p.horizonY).toBeCloseTo(roofline * (1 - (dip + 10) / 40), 6);
  });

  it("places a point as the planes are placed", () => {
    const p = projector(tower, high, harbour.lake_m);
    // 1 km straight out of the window, on the water: in the middle, about 8.2° down.
    const [x, y] = p.screen([1000, 0], 74.5);
    expect(x).toBeCloseTo(0.5, 6);
    const el = (Math.atan(-143.5 / 1000 - 1000 / (2 * 6371008.8 * (4 / 3))) * 180) / Math.PI;
    expect(el).toBeCloseTo(-8.17, 2);
    expect(y).toBeCloseTo(roofline * (1 - (el + 10) / 40), 6);
    // 45° to the right is 45/100 of the width right of the middle.
    expect(p.screen([1000, 1000], 74.5)[0]).toBeCloseTo(0.95, 6);
  });

  it("leaves out what lies under or behind the window", () => {
    expect(
      clipFront([
        [-100, -10],
        [500, -10],
        [500, 10],
        [-100, 10],
      ]),
    ).toEqual([
      [60, -10],
      [500, -10],
      [500, 10],
      [60, 10],
    ]);
    expect(
      clipFront([
        [-100, 0],
        [-50, 10],
        [-80, 20],
      ]),
    ).toEqual([]);
  });

  it("shows Billy Bishop's runway in the window, below the horizon", () => {
    const p = projector(tower, high, harbour.lake_m);
    const rw = harbour.runways.find((r) => r.ref === "08/26")!;
    const ends = rw.line.map((q) => p.screen(p.local(q), harbour.ground_m));
    for (const [x, y] of ends) {
      expect(y).toBeGreaterThan(p.horizonY);
      expect(y).toBeLessThan(roofline);
    }
    expect(ends.some(([x]) => x > 0 && x < 1)).toBe(true);
    expect(ringPath(p, harbour.islands[0]!, harbour.ground_m)).toMatch(/^M.*Z$/);
  });

  it("draws the buildings farthest first, each with walls facing the window", () => {
    const p = projector(tower, high, harbour.lake_m);
    const bs = blocks(p, harbour, 0);
    expect(bs.length).toBeGreaterThan(20);
    for (let i = 1; i < bs.length; i++) expect(bs[i]!.distance).toBeLessThanOrEqual(bs[i - 1]!.distance);
    expect(bs.every((b) => b.walls.every((w) => w.sunlit >= 0 && w.sunlit <= 1))).toBe(true);
  });
});
