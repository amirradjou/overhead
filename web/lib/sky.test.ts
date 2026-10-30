import { describe, expect, it } from "vitest";
import {
  compassTicks,
  nearestTurn,
  overviewTurn,
  polar,
  roofline,
  rooftops,
  seeded,
  stars,
  toScreen,
  topDeg,
} from "./sky";
import { skyPhase, sunElevation } from "./sun";
import type { Window } from "./types";

const south: Window = { azimuth_deg: 180, fov_deg: 90, min_elevation_deg: 5, home_elevation_m: 0 };
const allRound: Window = { azimuth_deg: 0, fov_deg: 360, min_elevation_deg: 5, home_elevation_m: 0 };

describe("the window on screen", () => {
  it("puts the window's centre in the middle and its edges at the sides", () => {
    expect(toScreen(south, 0, 5)).toEqual({ x: 0.5, y: roofline });
    expect(toScreen(south, -45, topDeg).x).toBe(0);
    expect(toScreen(south, 45, topDeg)).toEqual({ x: 1, y: 0 });
    expect(toScreen(south, 0, 89).y).toBe(0); // above the top of the frame: held at the top edge
    expect(toScreen(south, 0, (5 + topDeg) / 2).y).toBeCloseTo(roofline / 2);
  });

  it("labels the compass points the window looks across", () => {
    expect(compassTicks(south)).toEqual([
      { label: "SE", x: 0 },
      { label: "S", x: 0.5 },
      { label: "SW", x: 1 },
    ]);
    const round = compassTicks(allRound);
    expect(round).toHaveLength(8);
    expect(round.find((t) => t.label === "N")?.x).toBe(0.5);
    expect(round.find((t) => t.label === "S")?.x).toBe(1);
  });

  it("turns the overview so the window faces up, north wherever it falls", () => {
    expect(overviewTurn(south)).toBe(180);
    expect(overviewTurn(allRound)).toBe(0);
    // Facing south, a plane due south of home is straight up the disc and north is at the bottom.
    const up = polar(180 - overviewTurn(south), 10);
    expect(up.x).toBeCloseTo(0);
    expect(up.y).toBeCloseTo(-10);
    const north = polar(0 - overviewTurn(south), 10);
    expect(north.y).toBeCloseTo(10);
    // Facing south-south-west (200°), a plane due west (270°) is 70° right of the top.
    const west = polar(270 - 200, 10);
    expect(west.x).toBeGreaterThan(0);
    expect(west.y).toBeLessThan(0);
  });

  it("turns a plane the short way round", () => {
    expect(nearestTurn(179, -179)).toBe(181);
    expect(nearestTurn(-170, 170)).toBe(-190);
    expect(nearestTurn(725, 10)).toBe(730);
    expect(nearestTurn(0, 90)).toBe(90);
    expect(nearestTurn(0, -90)).toBe(-90);
  });

  it("draws the same rooftops and stars every time, never above the roofline", () => {
    expect(rooftops()).toBe(rooftops());
    const path = rooftops();
    const tops = [...path.matchAll(/[\d.]+,([\d.]+)/g)].map((m) => Number(m[1]));
    expect(Math.min(...tops)).toBeGreaterThanOrEqual(roofline * 1000 - 16); // chimneys poke up a little
    // One outline: a single move, closed at the end, so the fill can't come apart.
    expect(path.match(/M/g)).toHaveLength(1);
    expect(path.endsWith("Z")).toBe(true);
    const s = stars(50);
    expect(s).toEqual(stars(50));
    expect(s.every((st) => st.y < roofline)).toBe(true);
    const r = seeded(1);
    expect(r()).not.toBe(r());
  });
});

describe("the sun over Toronto", () => {
  // Solar noon in Toronto is about 17:20 UTC. At the solstices the sun peaks at 90 − 43.65 ± 23.44 degrees.
  it("is as high as it gets at midsummer noon and low at midwinter noon", () => {
    expect(sunElevation(43.65, -79.38, new Date("2026-06-21T17:21:00Z"))).toBeCloseTo(69.8, 0);
    expect(sunElevation(43.65, -79.38, new Date("2026-12-21T17:15:00Z"))).toBeCloseTo(22.9, 0);
  });

  it("is below the horizon at midnight", () => {
    const midnight = sunElevation(43.65, -79.38, new Date("2026-10-30T04:00:00Z"));
    expect(midnight).toBeLessThan(-30);
    expect(skyPhase(midnight)).toBe("night");
    expect(skyPhase(-3)).toBe("twilight");
    expect(skyPhase(4)).toBe("golden");
    expect(skyPhase(30)).toBe("day");
  });
});
