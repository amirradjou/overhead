import { describe, expect, it } from "vitest";
import { groundElevationDeg, placeOnGround } from "./ground";
import type { FollowedAirport, Plane, Window } from "./types";

const w: Window = { azimuth_deg: 163, fov_deg: 100, min_elevation_deg: -10, home_elevation_m: 218 };
const airport: FollowedAirport = {
  icao: "CYTZ",
  iata: "YTZ",
  name: "Billy Bishop",
  lat: 43.6275,
  lon: -79.3962,
  elevation_ft: 252,
  arrivals_today: 0,
  departures_today: 0,
};
const taxiing: Plane = {
  hex: "c060b1",
  lat: 43.63,
  lon: -79.4,
  source: "adsb.lol",
  at: "2026-11-05T23:46:37Z",
  distance_km: 1.338,
  bearing_deg: 203,
  elevation_deg: 0, // what the live API sends: it knows no height
  offset_deg: 40,
  in_view: false,
  on_ground: true,
  airport: { op: "arriving", phase: "taxiing", distance_km: 0.2 },
};

describe("a plane on the ground", () => {
  it("is seen as far below the window as the airport is", () => {
    expect(groundElevationDeg(1.338, 76.8, 218)).toBeCloseTo(-6.02, 1);
  });

  it("is in view at the followed airport", () => {
    const [p] = placeOnGround([taxiing], w, airport);
    expect(p!.in_view).toBe(true);
    expect(p!.elevation_deg).toBeLessThan(-5);
  });

  it("is out of view beyond the window's side", () => {
    const [p] = placeOnGround([{ ...taxiing, offset_deg: 52 }], w, airport);
    expect(p!.in_view).toBe(false);
  });

  it("stays as reported away from the followed airport", () => {
    const elsewhere = { ...taxiing, airport: undefined };
    expect(placeOnGround([elsewhere], w, airport)[0]).toBe(elsewhere);
    expect(placeOnGround([taxiing], w)[0]).toBe(taxiing);
  });

  it("leaves planes in the air alone", () => {
    const flying = { ...taxiing, on_ground: false, elevation_deg: -3 };
    expect(placeOnGround([flying], w, airport)[0]).toBe(flying);
  });
});
