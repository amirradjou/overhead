import { describe, expect, it } from "vitest";
import {
  airportDay,
  airportStatus,
  bearingDeg,
  board,
  fromToHere,
  heightFromWindow,
  heightOver,
  lookWhere,
  shown,
  vertical,
} from "./airport";
import { inView } from "./pick";
import type { AtAirport, FollowedAirport, Plane, View, Window } from "./types";

const ytz: FollowedAirport = {
  icao: "CYTZ",
  iata: "YTZ",
  name: "Billy Bishop",
  lat: 43.6275,
  lon: -79.3962,
  elevation_ft: 252,
  runway_in_use: "26",
  arrivals_today: 12,
  departures_today: 1,
};
const high: Window = {
  azimuth_deg: 163,
  fov_deg: 100,
  min_elevation_deg: -10,
  max_elevation_deg: 30,
  home_elevation_m: 218,
};

function plane(hex: string, airport?: Partial<AtAirport>, extra: Partial<Plane> = {}): Plane {
  return {
    hex,
    callsign: "PTR2943",
    lat: 43.62,
    lon: -79.35,
    source: "adsb.fi",
    at: "2026-10-30T18:00:00Z",
    distance_km: 3,
    bearing_deg: 120,
    elevation_deg: -3,
    offset_deg: 12,
    in_view: true,
    airport: airport ? { op: "arriving", phase: "approach", distance_km: 3, ...airport } : undefined,
    ...extra,
  };
}

const route = {
  callsign: "PTR2943",
  flight_iata: "PD2943",
  airline: "Porter Airlines",
  origin: { icao: "KBOS", iata: "BOS", city: "Boston" },
  destination: {
    icao: "CYTZ",
    iata: "YTZ",
    city: "Toronto",
    name: "Billy Bishop Toronto City Centre Airport",
  },
};

describe("following an airport", () => {
  it("reads a flight's height over the runway, not over the sea", () => {
    const final = { op: "arriving" as const, phase: "final" as const, distance_km: 0.4 };
    expect(heightOver(plane("a", { ...final, height_ft: 12 }, { alt_ft: 350, alt_msl_ft: 264 }))).toBe(
      "0 ft",
    );
    expect(heightOver(plane("a", undefined, { alt_ft: 3000 }))).toBe("3,000 ft");
    expect(heightOver(plane("a", { ...final }, { on_ground: true }))).toBe("On the ground");
  });

  it("says a plane on final is touching down once it is over the runway's end", () => {
    const final = { op: "arriving" as const, phase: "final" as const, distance_km: 0.4, in_min: 0.1 };
    expect(airportStatus(plane("a", { ...final, height_ft: 300 }), ytz)).toBe(
      "On final for runway 26, touchdown any moment",
    );
    expect(airportStatus(plane("a", { ...final, height_ft: 10 }), ytz)).toBe("Touching down on runway 26");
    expect(airportStatus(plane("a", final), ytz)).toBe("On final for runway 26, touchdown any moment");
  });

  it("shows only the focus planes on a focused screen", () => {
    const planes = [plane("a", {}, { focus: true }), plane("b")];
    const view = { planes, focused: true } as unknown as View;
    expect(shown(view).map((p) => p.hex)).toEqual(["a"]);
    expect(shown({ ...view, focused: false }).map((p) => p.hex)).toEqual(["a", "b"]);
    expect(shown(null)).toEqual([]);
  });

  it("features a landing over a plane higher in the sky", () => {
    const climbing = plane("climb", { op: "departing", phase: "climbing" }, { elevation_deg: 20 });
    const landing = plane("land", { phase: "landing" }, { elevation_deg: -6 });
    const near = plane("near", undefined, { near: true, elevation_deg: 2 });
    expect(inView([climbing, near, landing]).map((p) => p.hex)).toEqual(["land", "climb", "near"]);
  });

  it("says what the plane is doing there", () => {
    const say = (a: Partial<AtAirport>) => airportStatus(plane("x", a), ytz);
    expect(say({ phase: "final", runway: "26", in_min: 1.6 })).toBe(
      "On final for runway 26, touchdown in about 2 min",
    );
    expect(say({ phase: "final", in_min: 0.4 })).toBe("On final for runway 26, touchdown any moment");
    expect(say({ phase: "approach", in_min: 7.2 })).toBe("On approach, landing in about 7 min");
    expect(say({ phase: "landing", runway: "26" })).toBe("Landing on runway 26");
    expect(say({ phase: "taxiing" })).toBe("Landed, taxiing to the gate");
    expect(say({ op: "departing", phase: "taxiing" })).toBe("Taxiing out to runway 26");
    expect(say({ op: "departing", phase: "takeoff", runway: "08" })).toBe("Taking off from runway 08");
    expect(say({ op: "departing", phase: "takeoff", runway: "08", height_ft: 300 })).toBe(
      "Lifting off from runway 08",
    );
    expect(say({ op: "departing", phase: "climbing", ago_min: 3.2 })).toBe(
      "Climbing out, took off about 3 min ago",
    );
    expect(airportStatus(plane("n", undefined, { near: true }))).toBe("Very close to your window");
  });

  it("names the airport by its own name in the route", () => {
    expect(fromToHere(plane("x", {}, { info: { hex: "x", route } }), ytz)).toBe(
      "From Boston to Billy Bishop",
    );
    expect(fromToHere(plane("x"), ytz)).toBe("");
  });

  it("measures the plane against the window", () => {
    const at = (alt_ft: number) => heightFromWindow(plane("x", {}, { alt_ft }), high);
    expect(at(575)).toBe("45 m below your window"); // 175 m against 218 m
    expect(at(715)).toBe("level with your window");
    expect(at(1800)).toBe("330 m above your window");
    expect(at(9000)).toBe("2.5 km above your window");
    expect(heightFromWindow(plane("x", {}, { on_ground: true }), high)).toBe("on the ground");
    expect(vertical(plane("x", {}, { vert_rate_fpm: -704 }))).toBe("descending 700 ft/min");
    expect(vertical(plane("x", {}, { vert_rate_fpm: 1830 }))).toBe("climbing 1,850 ft/min");
    expect(vertical(plane("x", {}, { vert_rate_fpm: 100 }))).toBe("level");
  });

  it("says where to look", () => {
    expect(lookWhere(plane("x", {}, { elevation_deg: -5.6, offset_deg: 12 }))).toBe(
      "6° below the horizon, a little to the right",
    );
    expect(lookWhere(plane("x", {}, { elevation_deg: 0.4, offset_deg: -3 }))).toBe(
      "on the horizon, straight ahead",
    );
    expect(lookWhere(plane("x", {}, { elevation_deg: 14, offset_deg: -40 }))).toBe("14° up, to the left");
  });

  it("lists the board busiest first", () => {
    const view = {
      airport: ytz,
      planes: [
        plane(
          "gate",
          { op: "departing", phase: "gate" },
          {
            info: {
              hex: "gate",
              route: { ...route, origin: route.destination, destination: { iata: "YOW", city: "Ottawa" } },
            },
          },
        ),
        plane("far", { phase: "approach", in_min: 9 }),
        plane("near", { phase: "approach", in_min: 4 }, { info: { hex: "near", route } }),
        plane("roll", { phase: "landing" }),
        plane("other"),
      ],
    } as unknown as View;
    const rows = board(view);
    expect(rows.map((r) => r.hex)).toEqual(["roll", "near", "far", "gate"]);
    expect(rows[1]).toEqual({
      hex: "near",
      arrow: "↓",
      flight: "PD 2943",
      place: "Boston",
      status: "lands in 4 min",
    });
    expect(rows[3]).toMatchObject({ arrow: "↑", place: "Ottawa", status: "at the gate" });
    const cessna = plane(
      "c172",
      { op: "ground", phase: "taxiing" },
      { callsign: "CGRYY", info: { hex: "c172", registration: "C-GRYY" } },
    );
    expect(board({ ...view, planes: [cessna] })[0]).toMatchObject({
      arrow: "·",
      flight: "C-GRYY",
      status: "taxiing",
    });
  });

  it("sums up the airport's day", () => {
    expect(airportDay(ytz)).toBe("Runway 26 in use. 12 landings and 1 take-off today.");
    expect(airportDay({ ...ytz, runway_in_use: undefined, arrivals_today: 1, departures_today: 0 })).toBe(
      "1 landing and 0 take-offs today.",
    );
  });

  it("finds the airport's direction from home", () => {
    expect(bearingDeg({ lat: 43.6426, lon: -79.3871 }, ytz)).toBeCloseTo(203.6, 0);
  });
});

describe("the day's counts", () => {
  const ap: FollowedAirport = {
    icao: "CYTZ",
    iata: "YTZ",
    name: "Billy Bishop",
    lat: 43.6275,
    lon: -79.3962,
    elevation_ft: 252,
    arrivals_today: 0,
    departures_today: 0,
  };

  it("say nothing they cannot back", () => {
    expect(airportDay({ ...ap, uncounted: true })).toBe("");
    expect(airportDay({ ...ap, uncounted: true, runway_in_use: "26" })).toBe("Runway 26 in use.");
    expect(airportDay(ap)).toBe("0 landings and 0 take-offs today.");
  });
});
