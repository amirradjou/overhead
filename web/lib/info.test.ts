import { describe, expect, it } from "vitest";
import { sizeLike, typeFactLines, typeFacts } from "./aircraft";
import { hhmm, moonLine, sunLine } from "./almanac";
import { duration, flightProgress, greatCircleKm, km, progressText } from "./progress";
import type { Plane, Window } from "./types";

const yyz = { iata: "YYZ", city: "Toronto", lat: 43.6777, lon: -79.6248 };
const sfo = { iata: "SFO", city: "San Francisco", lat: 37.6213, lon: -122.379 };
const yvr = { iata: "YVR", city: "Vancouver", lat: 49.1939, lon: -123.1844 };

function plane(over: Partial<Plane> = {}): Plane {
  return {
    hex: "c08861",
    lat: 43.6,
    lon: -79.4,
    source: "adsb.fi",
    at: "",
    distance_km: 5,
    bearing_deg: 200,
    elevation_deg: 20,
    offset_deg: 0,
    in_view: true,
    ...over,
  };
}

const route = (origin: object, destination: object) => ({
  info: { hex: "c08861", route: { callsign: "POE1712", origin, destination } },
});

describe("flight progress", () => {
  it("measures great circles", () => {
    expect(greatCircleKm(sfo, yyz)).toBeGreaterThan(3600);
    expect(greatCircleKm(sfo, yyz)).toBeLessThan(3650);
    expect(greatCircleKm(yyz, yyz)).toBe(0);
  });

  it("says how far the plane has come and how long it has to go", () => {
    // Over Toronto, inbound from San Francisco, level at 460 kt.
    const p = plane({ ...route(sfo, yyz), alt_ft: 36000, vert_rate_fpm: 0, speed_kt: 460 });
    const pr = flightProgress(p)!;
    expect(pr.fromCode).toBe("SFO");
    expect(pr.toName).toBe("Toronto");
    expect(pr.done).toBeGreaterThan(0.99);
    expect(progressText(p, pr)).toMatch(/^3,6\d0 km flown, about 1 min to go$/);
  });

  it("leaves the time out while climbing out, and when the screen already says when it lands", () => {
    const out = plane({
      ...route(yyz, yvr),
      alt_ft: 9000,
      vert_rate_fpm: 2500,
      speed_kt: 280,
      lat: 43.75,
      lon: -80,
    });
    const pr = flightProgress(out)!;
    expect(pr.minutesLeft).toBeUndefined();
    expect(progressText(out, pr)).toMatch(/^3\d km flown, 3,3\d0 km to go$/);
    const landingSoon = plane({
      ...route(sfo, yyz),
      alt_ft: 3000,
      vert_rate_fpm: -800,
      speed_kt: 180,
      eta_min: 4,
    });
    expect(progressText(landingSoon, flightProgress(landingSoon)!)).toMatch(
      /^3,6\d0 km flown, \d+ km to go$/,
    );
  });

  it("needs both airports' positions", () => {
    expect(flightProgress(plane())).toBeNull();
    expect(flightProgress(plane(route({ iata: "SFO" }, yyz)))).toBeNull();
    expect(flightProgress(plane(route({ iata: "SFO", lat: 0, lon: 0 }, yyz)))).toBeNull();
  });

  it("writes distances and durations plainly", () => {
    expect(km(7.4)).toBe("7 km");
    expect(km(384)).toBe("380 km");
    expect(km(3642)).toBe("3,640 km");
    expect(duration(0.2)).toBe("1 min");
    expect(duration(25.4)).toBe("25 min");
    expect(duration(62)).toBe("1 h");
    expect(duration(64)).toBe("1 h 05 min");
    expect(duration(250)).toBe("4 h 10 min");
  });
});

describe("aircraft facts", () => {
  it("pictures a size", () => {
    expect(sizeLike(35.9)).toBe("about three city buses end to end");
    expect(sizeLike(28.4)).toBe("about two and a half city buses end to end");
    expect(sizeLike(24.9)).toBe("about the width of a hockey rink");
    expect(sizeLike(60.1)).toBe("about the length of a hockey rink");
    expect(sizeLike(79.8)).toBe("longer than a hockey rink");
    expect(sizeLike(12.5)).toBe("about the length of a city bus");
  });

  it("has a few lines for the common types and none for the rest", () => {
    expect(typeFactLines(plane({ type_code: "B38M" }))).toEqual([
      "Wingspan 35.9 m, about three city buses end to end.",
      "The 737 MAX 8 first flew in 2016.",
      "Airlines fit it with 160 to 190 seats.",
    ]);
    expect(typeFactLines(plane({ type_code: "E75L" }))[0]).toBe(
      "31.7 m long, about two and a half city buses end to end.",
    );
    expect(typeFactLines(plane({ type_code: "DH8D" })).at(-1)).toMatch(/Downsview/);
    expect(typeFactLines(plane({ info: { hex: "x", type_code: "A388" } }))[0]).toBe(
      "Wingspan 79.8 m, longer than a hockey rink.",
    );
    expect(typeFactLines(plane({ type_code: "C172" }))).toEqual([]);
    expect(typeFactLines(plane())).toEqual([]);
  });

  it("keeps every entry sensible", () => {
    for (const [code, t] of Object.entries(typeFacts)) {
      expect(code).toMatch(/^[A-Z0-9]{3,4}$/);
      expect(t.firstFlight).toBeGreaterThan(1960);
      expect(t.firstFlight).toBeLessThan(2026);
      expect(t.lengthM).toBeGreaterThan(20);
      expect(t.lengthM).toBeLessThan(80);
      if (t.wingspanM) expect(t.wingspanM).toBeLessThan(81);
      if (t.seats) expect(t.seats[0]).toBeLessThanOrEqual(t.seats[1]);
    }
  });
});

describe("the almanac", () => {
  const edt = -240; // Toronto in October
  const lat = 43.65;
  const lon = -79.38;
  const window: Window = { azimuth_deg: 200, fov_deg: 100, min_elevation_deg: 8, home_elevation_m: 0 };

  it("says the next sunrise or sunset", () => {
    expect(hhmm(new Date("2026-10-30T22:10:56Z"), edt)).toBe("18:10");
    expect(sunLine(lat, lon, new Date("2026-10-30T09:00:00Z"), edt)).toBe("Sunrise at 07:50.");
    expect(sunLine(lat, lon, new Date("2026-10-30T16:00:00Z"), edt)).toBe("Sunset at 18:10.");
    expect(sunLine(lat, lon, new Date("2026-10-31T01:00:00Z"), edt)).toBe("Sunrise tomorrow at 07:52.");
  });

  it("says where the moon is and how full", () => {
    expect(moonLine(lat, lon, new Date("2026-10-30T10:40:00Z"), window, edt)).toBe(
      "The moon in your window is a waning gibbous, 76% lit.",
    );
    expect(moonLine(lat, lon, new Date("2026-10-30T04:00:00Z"), window, edt)).toMatch(
      /^The moon is up in the east, a waning gibbous, 79% lit\.$/,
    );
    expect(moonLine(lat, lon, new Date("2026-10-30T20:00:00Z"), window, edt)).toMatch(
      /^The moon rises at 2\d:\d\d, a waning gibbous, \d\d% lit\.$/,
    );
  });
});
