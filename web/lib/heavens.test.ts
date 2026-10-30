import { describe, expect, it } from "vitest";
import { luminance, mix, skyColours, sunGlow } from "./skyColour";
import { horizontal, magnitudeLimit, siderealDeg, starColour, starsAbove } from "./stars";
import { moonAt, moonPath, moonPhaseName, sunTimes } from "./sun";

const toronto = { lat: 43.65, lon: -79.38 };

describe("the sky's colour", () => {
  it("changes smoothly with the sun, never in steps", () => {
    for (let sun = -25; sun < 40; sun += 0.5) {
      const a = skyColours(sun);
      const b = skyColours(sun + 0.5);
      expect(Math.abs(luminance(a.low) - luminance(b.low))).toBeLessThan(0.03);
      expect(Math.abs(luminance(a.top) - luminance(b.top))).toBeLessThan(0.01);
    }
  });

  it("is darkest at night and held well below a light box by day", () => {
    const night = skyColours(-30);
    const day = skyColours(60);
    expect(luminance(night.top)).toBeLessThan(0.005);
    expect(luminance(day.low)).toBeLessThan(0.3);
    expect(luminance(day.top)).toBeLessThan(0.08);
    expect(luminance(day.low)).toBeGreaterThan(luminance(night.low));
    // The roofs stay the darkest thing on screen.
    for (const sun of [-30, -6, 0, 5, 30]) {
      const c = skyColours(sun);
      expect(luminance(c.roof)).toBeLessThan(luminance(c.top));
    }
  });

  it("mixes colours in linear light", () => {
    expect(mix("#000000", "#ffffff", 0)).toBe("#000000");
    expect(mix("#000000", "#ffffff", 1)).toBe("#ffffff");
    expect(mix("#000000", "#ffffff", 0.5)).toBe("#bcbcbc"); // half the light, not half the number
  });

  it("glows round the sun at sunset and not at night", () => {
    expect(sunGlow(-20, 0, 100, 0.72)).toBeNull();
    const sunset = sunGlow(-1, 40, 100, 0.72);
    expect(sunset?.opacity).toBeCloseTo(0.5);
    expect(sunset?.x).toBeCloseTo(0.9);
    // Round the back of the house the glow fades out.
    expect(sunGlow(-1, 150, 100, 0.72)).toBeNull();
    expect(sunGlow(40, 0, 100, 0.3)?.opacity).toBeLessThan(0.2);
  });
});

describe("the stars", () => {
  it("keeps sidereal time: Greenwich's at J2000.0 is 280.46°", () => {
    expect(siderealDeg(new Date("2000-01-01T12:00:00Z"), 0)).toBeCloseTo(280.46, 2);
    // A sidereal day is about 3 min 56 s shorter than a solar one.
    const a = siderealDeg(new Date("2026-10-30T00:00:00Z"), -79.38);
    const b = siderealDeg(new Date("2026-10-31T00:00:00Z"), -79.38);
    expect(b - a).toBeCloseTo(0.9856, 3);
  });

  it("puts Polaris at the height of the latitude, due north", () => {
    const lst = siderealDeg(new Date("2026-10-30T06:00:00Z"), toronto.lon);
    const polaris = horizontal(37.95, 89.26, toronto.lat, lst);
    expect(Math.abs(polaris.alt - toronto.lat)).toBeLessThan(0.8);
    expect(Math.min(polaris.az, 360 - polaris.az)).toBeLessThan(1.2);
  });

  it("has a star due south at its highest when its right ascension is overhead", () => {
    const sirius = horizontal(101.29, -16.72, toronto.lat, 101.29);
    expect(sirius.az).toBeCloseTo(180, 5);
    expect(sirius.alt).toBeCloseTo(90 - toronto.lat - 16.72, 5);
    const east = horizontal(101.29, -16.72, toronto.lat, 101.29 - 60);
    expect(east.az).toBeGreaterThan(90);
    expect(east.az).toBeLessThan(180);
  });

  it("shows more stars the darker the sky", () => {
    const when = new Date("2026-10-30T06:30:00Z");
    expect(magnitudeLimit(10)).toBe(-Infinity);
    const night = starsAbove(toronto.lat, toronto.lon, when, magnitudeLimit(-30));
    const dusk = starsAbove(toronto.lat, toronto.lon, when, magnitudeLimit(-7));
    expect(night.length).toBeGreaterThan(250);
    expect(dusk.length).toBeLessThan(night.length / 5);
    expect(night.every((s) => s.alt > 0 && s.mag <= 4.5)).toBe(true);
    expect(starColour(-0.2)).not.toBe(starColour(1.6));
  });
});

describe("the moon", () => {
  it("knows the phase and where the moon is", () => {
    // 2026-10-26 was a full moon; four days later it is a waning gibbous, high in the south-west before dawn.
    const m = moonAt(toronto.lat, toronto.lon, new Date("2026-10-30T10:40:00Z"));
    expect(moonPhaseName(m.phase)).toBe("waning gibbous");
    expect(m.fraction).toBeGreaterThan(0.6);
    expect(m.fraction).toBeLessThan(0.9);
    expect(m.az).toBeGreaterThan(200);
    expect(m.az).toBeLessThan(250);
    expect(m.alt).toBeGreaterThan(55);
    expect(moonPhaseName(0)).toBe("new moon");
    expect(moonPhaseName(0.5)).toBe("full moon");
    expect(moonPhaseName(0.1)).toBe("waxing crescent");
    expect(moonPhaseName(0.99)).toBe("new moon");
  });

  it("draws a crescent, a half and a full moon", () => {
    expect(moonPath(0.5, 10)).toBe("M-10 0A10 10 0 0 1 10 0A10 0.000 0 0 0 -10 0Z");
    expect(moonPath(1, 10)).toBe("M-10 0A10 10 0 0 1 10 0A10 10.000 0 0 1 -10 0Z");
    expect(moonPath(0.1, 10)).toBe("M-10 0A10 10 0 0 1 10 0A10 8.000 0 0 0 -10 0Z");
  });

  it("gives sunrise and sunset for the day", () => {
    const { sunrise, sunset } = sunTimes(toronto.lat, toronto.lon, new Date("2026-10-30T16:00:00Z"), -240);
    expect(sunrise?.toISOString().slice(0, 16)).toBe("2026-10-30T11:50");
    expect(sunset?.toISOString().slice(0, 16)).toBe("2026-10-30T22:10");
  });
});
