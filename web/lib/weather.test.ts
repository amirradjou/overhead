import { describe, expect, it } from "vitest";
import { cloudscape, cloudColours } from "./clouds";
import type { FollowedAirport, Weather } from "./types";
import { cloudLine, compass16, sky, weatherLine } from "./weather";

// As Open-Meteo reported over the harbour on 2026-10-30.
const now: Weather = {
  at: "2026-10-30T19:00:00Z",
  temp_c: 13.8,
  code: 3,
  cloud_cover: 100,
  cloud_low: 100,
  cloud_mid: 0,
  cloud_high: 0,
  visibility_m: 37000,
  wind_kmh: 18.2,
  wind_from_deg: 300,
  gust_kmh: 29.2,
};
const ytz = { name: "Billy Bishop", runway_in_use: "26" } as FollowedAirport;

describe("the weather in words", () => {
  it("ties the runway in use to the wind it faces", () => {
    expect(weatherLine(now, ytz)).toBe(
      "14°C and overcast. Wind from the west-north-west at 18 km/h, gusting 29, so planes land and take off on runway 26, into it.",
    );
    // Runway 08 faces away from a west-north-west wind: no claim about why.
    expect(weatherLine(now, { ...ytz, runway_in_use: "08" })).toBe(
      "14°C and overcast. Wind from the west-north-west at 18 km/h, gusting 29.",
    );
    expect(weatherLine({ ...now, wind_kmh: 3, code: 0, visibility_m: 4000 })).toBe(
      "14°C and clear. Almost no wind. You can see about 4 km.",
    );
  });

  it("names skies and directions", () => {
    expect([sky(2), sky(45), sky(63), sky(75), sky(95)]).toEqual([
      "partly cloudy",
      "foggy",
      "raining",
      "snowing",
      "thunderstorms",
    ]);
    expect([compass16(0), compass16(300), compass16(251), compass16(359)]).toEqual([
      "north",
      "west-north-west",
      "west-south-west",
      "north",
    ]);
    expect(cloudLine(now)).toMatch(/^Low cloud/);
    expect(cloudLine({ ...now, cloud_low: 40 })).toBe("");
  });
});

describe("the clouds", () => {
  const at = (off: number, el: number): [number, number] => [0.5 + off / 100, 0.72 * (1 - (el + 10) / 40)];
  const draw = (w: Partial<Weather>, sun = 30) => cloudscape({ ...now, ...w }, sun, 100, 30, 218, at);

  it("follow the cover: none when clear, a deck when overcast, cumulus between", () => {
    const clear = draw({ cloud_low: 0, cloud_mid: 0, cloud_high: 0 });
    expect([clear.puffs.length, clear.streaks.length, clear.deck]).toEqual([0, 0, 0]);
    const overcast = draw({ cloud_low: 100 });
    expect(overcast.deck).toBe(1);
    expect(overcast.puffs.length).toBe(0);
    const broken = draw({ cloud_low: 50, cloud_high: 40 });
    expect(broken.deck).toBe(0);
    expect(broken.puffs.length).toBeGreaterThan(50);
    expect(broken.streaks.length).toBeGreaterThan(0);
    expect(draw({ cloud_low: 50, cloud_high: 40 })).toEqual(broken); // the same reading, the same sky
  });

  it("stay in the sky, above the horizon", () => {
    const broken = draw({ cloud_low: 60 });
    const horizon = at(0, 0)[1];
    expect(broken.puffs.every((p) => p.cy <= horizon + 0.001)).toBe(true);
  });

  it("dim at night, lit only by the city", () => {
    expect(cloudColours(-20).opacity).toBeLessThan(cloudColours(20).opacity);
  });
});
