import { describe, expect, it } from "vitest";
import {
  compassTicks,
  horizonY,
  inFrame,
  looksDown,
  nearestTurn,
  offsetFrom,
  overviewTurn,
  polar,
  roofline,
  rooftops,
  seeded,
  sill,
  toScreen,
  screenTop,
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

  it("knows what the window frames", () => {
    expect(offsetFrom(south, 180)).toBe(0);
    expect(offsetFrom(south, 225)).toBe(45);
    expect(offsetFrom(south, 90)).toBe(-90);
    expect(offsetFrom(allRound, 350)).toBe(-10);
    expect(inFrame(south, 44, 30)).toBe(true);
    expect(inFrame(south, 46, 30)).toBe(false);
    expect(inFrame(south, 0, 4)).toBe(false); // behind the roofs
    expect(inFrame(south, 0, topDeg + 1)).toBe(false); // above the top of the frame
  });

  it("from high up, draws the horizon above the bottom of the window and a flat sill", () => {
    const high: Window = {
      azimuth_deg: 163,
      fov_deg: 100,
      min_elevation_deg: -5,
      max_elevation_deg: 30,
      home_elevation_m: 218,
    };
    expect(looksDown(high)).toBe(true);
    expect(looksDown(south)).toBe(false);
    expect(horizonY(south)).toBeUndefined();
    // 0° is a seventh of the way up from -5° to 30°.
    expect(horizonY(high)).toBeCloseTo(roofline * (1 - 5 / 35));
    expect(sill()).toBe(`M-40,1040 L-40,${roofline * 1000} L1040,${roofline * 1000} L1040,1040 Z`);
  });

  it("draws the same rooftops every time, never above the roofline", () => {
    expect(rooftops()).toBe(rooftops());
    const path = rooftops();
    const tops = [...path.matchAll(/[\d.]+,([\d.]+)/g)].map((m) => Number(m[1]));
    expect(Math.min(...tops)).toBeGreaterThanOrEqual(roofline * 1000 - 16); // chimneys poke up a little
    // One outline: a single move, closed at the end, so the fill can't come apart.
    expect(path.match(/M/g)).toHaveLength(1);
    expect(path.endsWith("Z")).toBe(true);
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

describe("a window with a top edge", () => {
  const framed: Window = {
    azimuth_deg: 180,
    fov_deg: 90,
    min_elevation_deg: 5,
    max_elevation_deg: 40,
    home_elevation_m: 0,
  };
  it("puts the top edge at the top of the screen, and nothing above it in the frame", () => {
    expect(screenTop(framed)).toBe(40);
    expect(toScreen(framed, 0, 40).y).toBe(0);
    expect(toScreen(framed, 0, 22.5).y).toBeCloseTo(roofline / 2);
    expect(inFrame(framed, 0, 41)).toBe(false);
    expect(inFrame(framed, 0, 39)).toBe(true);
  });
  it("falls back to the usual top without an edge, or with none worth drawing", () => {
    expect(screenTop({ ...framed, max_elevation_deg: 90 })).toBe(topDeg);
    expect(screenTop({ ...framed, max_elevation_deg: undefined })).toBe(topDeg);
    expect(screenTop({ ...framed, max_elevation_deg: 3 })).toBe(topDeg);
  });
});
