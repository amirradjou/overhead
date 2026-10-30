import { describe, expect, it } from "vitest";
import { profile } from "./profile";
import type { AtAirport } from "./types";

const final: AtAirport = {
  op: "arriving",
  phase: "final",
  runway: "26",
  distance_km: 3,
  height_ft: 520,
  in_min: 0.8,
};

describe("the side view of a flight", () => {
  it("brings an arrival in from the left to the runway on the right", () => {
    const p = profile(
      final,
      [
        { km: 5, m: 270 },
        { km: 4, m: 215 },
      ],
      141,
    )!;
    expect(p.departing).toBe(false);
    expect(p.runwayX).toBe(1);
    expect(p.plane[0]).toBeGreaterThan(p.trail[1]![0]); // nearer the runway than a moment ago
    expect(p.trail[0]![0]).toBeLessThan(p.trail[1]![0]);
    // 520 ft is 158 m at 3 km: the 3° path is 157 m there.
    expect(p.path).toBe("on the 3° path");
    expect(p.windowY).toBeGreaterThan(0);
    expect(p.windowY).toBeLessThan(1);
    expect(p.glide[0]).toEqual([1, 1]);
  });

  it("judges an arrival against the 3° path", () => {
    expect(profile({ ...final, height_ft: 1200 }, [], 141)!.path).toBe("above the 3° path");
    expect(profile({ ...final, height_ft: 250 }, [], 141)!.path).toBe("below the 3° path");
    expect(profile({ ...final, distance_km: 20, height_ft: 4000 }, [], 141)!.path).toBe("");
  });

  it("sends a departure off to the right from the runway on the left", () => {
    const p = profile({ op: "departing", phase: "climbing", distance_km: 6, height_ft: 2600 }, [], 141)!;
    expect(p.runwayX).toBe(0);
    expect(p.plane[0]).toBeGreaterThan(0.5);
    expect(p.path).toBe("");
  });
});
