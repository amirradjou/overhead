// The screen's side of what an independent check of its data found wrong on 2026-10-30.
import { describe, expect, it } from "vitest";
import { board, feet, heightFromWindow } from "./airport";
import { aircraftType, altitude } from "./format";
import type { Plane, View, Window } from "./types";
import { cover, weatherLine } from "./weather";

const plane = (extra: Partial<Plane>): Plane => ({
  hex: "c080dc",
  lat: 43.6,
  lon: -79.4,
  source: "adsb.fi",
  at: "2026-10-30T20:00:00Z",
  distance_km: 3,
  bearing_deg: 170,
  elevation_deg: -3,
  offset_deg: 5,
  in_view: true,
  ...extra,
});

describe("checked against independent sources", () => {
  it("names the type the feeds agree on when the airframe database disagrees", () => {
    // C-GWUT: an A220-300 (BCS3) in both feeds and its photo, an A220-100 in adsbdb.
    const gwut = plane({
      type_code: "BCS3",
      info: { hex: "c080dc", type_code: "BCS1", type_name: "BD-500-1A11" },
    });
    expect(aircraftType(gwut)).toBe("A220-300");
    expect(aircraftType(plane({ type_code: "ZZZZ", info: { hex: "x", type_code: "BCS1" } }))).toBe("ZZZZ");
    // When they agree, the database's fuller name stays.
    const dash8 = plane({
      type_code: "DH8D",
      info: { hex: "x", type_code: "DH8D", type_name: "DHC-8-402", manufacturer: "De Havilland Canada" },
    });
    expect(aircraftType(dash8)).toBe("De Havilland Canada DHC-8-402");
  });

  it("shows the true altitude, corrected for today's pressure", () => {
    const w: Window = { azimuth_deg: 163, fov_deg: 100, min_elevation_deg: -10, home_elevation_m: 218 };
    const p = plane({ alt_ft: 75, alt_msl_ft: 382 });
    expect(altitude(p)).toBe("375 ft");
    expect(feet(p)).toBe("375 ft");
    expect(heightFromWindow(p, w)).toBe("100 m below your window"); // 116 m against 218 m
    expect(altitude(plane({ alt_ft: 3000 }))).toBe("3,000 ft"); // no correction known: as reported
  });

  it("does not split a flight number that has no airline code", () => {
    const at = { op: "arriving" as const, phase: "approach" as const, distance_km: 20, in_min: 6 };
    const rows = (flight_iata: string, callsign: string) =>
      board({
        airport: {
          icao: "CYTZ",
          iata: "YTZ",
          name: "Billy Bishop",
          lat: 0,
          lon: 0,
          elevation_ft: 252,
          arrivals_today: 0,
          departures_today: 0,
        },
        planes: [
          plane({
            callsign,
            airport: at,
            info: { hex: "x", route: { callsign, flight_iata, origin: {}, destination: {} } },
          }),
        ],
      } as unknown as View)[0]!.flight;
    expect(rows("1813", "ROU1813")).toBe("ROU1813");
    expect(rows("RV1813", "ROU1813")).toBe("RV 1813");
    expect(rows("622", "FLE622")).toBe("FLE622");
    expect(rows("3K622", "JSA622")).toBe("3K 622");
  });

  it("says a broken deck is mostly cloudy", () => {
    expect([cover(5), cover(20), cover(45), cover(75), cover(100)]).toEqual([
      "clear",
      "mostly clear",
      "partly cloudy",
      "mostly cloudy",
      "overcast",
    ]);
    const bkn = {
      at: "",
      temp_c: 13,
      code: 2,
      cloud_cover: 75,
      cloud_low: 75,
      cloud_mid: 0,
      cloud_high: 0,
      visibility_m: 30000,
      wind_kmh: 18.5,
      wind_from_deg: 300,
      gust_kmh: 38.9,
    };
    expect(weatherLine(bkn)).toBe(
      "13°C and mostly cloudy. Wind from the west-north-west at 19 km/h, gusting 39.",
    );
  });
});
