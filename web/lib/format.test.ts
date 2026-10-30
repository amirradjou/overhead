import { describe, expect, it } from "vitest";
import {
  aircraftType,
  altitude,
  arrival,
  direction,
  flightFacts,
  flightName,
  fromTo,
  label,
  landing,
  operator,
  photoCredit,
  speed,
  trend,
  whereFromHome,
} from "./format";
import { comingNext, inView } from "./pick";
import type { Plane } from "./types";

function plane(over: Partial<Plane> = {}): Plane {
  return {
    hex: "c08861",
    lat: 43.6,
    lon: -79.7,
    source: "adsb.fi",
    at: "",
    distance_km: 11.6,
    bearing_deg: 244,
    elevation_deg: 10,
    offset_deg: 0,
    in_view: false,
    ...over,
  };
}

const porter = plane({
  callsign: "POE672",
  type_code: "E295",
  info: {
    hex: "c08861",
    type_name: "ERJ 190-400",
    manufacturer: "Embraer",
    owner: "Porter Airlines (Canada) Limited",
    route: {
      callsign: "POE672",
      airline: "Porter Airlines",
      origin: { iata: "SFO", city: "San Francisco" },
      destination: { icao: "CYYZ", name: "Lester B. Pearson International Airport", city: "Toronto" },
    },
    photo: { thumbnail: "t.jpg", link: "l", photographer: "Frederick Tremblay" },
  },
});

describe("format", () => {
  it("says what the plane is doing in one sentence", () => {
    const max = {
      type_code: "B38M",
      info: { hex: "c0ffee", type_name: "737 MAX 8", manufacturer: "Boeing" },
    };
    expect(flightFacts(plane({ ...max, alt_ft: 9800, vert_rate_fpm: -1200, speed_kt: 280 }))).toBe(
      "Boeing 737 MAX 8, descending through 9,800 ft at 520 km/h",
    );
    expect(flightFacts(plane({ ...max, alt_ft: 36000, vert_rate_fpm: 0, speed_kt: 460 }))).toBe(
      "Boeing 737 MAX 8, level at 36,000 ft, 850 km/h",
    );
    expect(flightFacts(plane({ ...max, alt_ft: 4100 }))).toBe("Boeing 737 MAX 8 at 4,100 ft");
    expect(flightFacts(plane({ alt_ft: 4100, vert_rate_fpm: 1800, speed_kt: 220 }))).toBe(
      "Climbing through 4,100 ft at 405 km/h",
    );
    expect(flightFacts(plane({ ...max, on_ground: true }))).toBe("Boeing 737 MAX 8, on the ground");
    expect(flightFacts(plane({ speed_kt: 220 }))).toBe("405 km/h");
    expect(flightFacts(plane())).toBe("");
  });

  it("writes altitude, trend and speed for a living room", () => {
    const p = plane({ alt_ft: 9712, vert_rate_fpm: -1408, speed_kt: 327.4 });
    expect(altitude(p)).toBe("9,700 ft");
    expect(trend(p)).toBe("descending");
    expect(speed(p)).toBe("605 km/h");
    expect(altitude(plane({ on_ground: true }))).toBe("On the ground");
    expect(trend(plane({ vert_rate_fpm: 120 }))).toBe("level");
    expect(altitude(plane())).toBe("");
  });

  it("says where a plane is from home in words", () => {
    expect(direction(244)).toBe("south-west");
    expect(direction(359)).toBe("north");
    expect(direction(-90)).toBe("west");
    expect(whereFromHome(plane())).toBe("12 km south-west");
    expect(whereFromHome(plane({ distance_km: 3.24, bearing_deg: 90 }))).toBe("3.2 km east");
    expect(whereFromHome(plane({ distance_km: 0.3 }))).toBe("overhead");
  });

  it("gives a landing time only to planes coming down", () => {
    expect(landing(plane({ eta_min: 4.4, vert_rate_fpm: -900, alt_ft: 3000 }))).toBe("Lands in about 4 min");
    expect(landing(plane({ eta_min: 0.4, vert_rate_fpm: -700, alt_ft: 300 }))).toBe("Landing now");
    expect(landing(plane({ eta_min: 6, vert_rate_fpm: 2000, alt_ft: 3000 }))).toBe("");
    expect(landing(plane({ eta_min: 9, vert_rate_fpm: 0, alt_ft: 35000 }))).toBe("");
  });

  it("names the flight, the aircraft and the route the way people say them", () => {
    expect(flightName(porter)).toBe("Porter Airlines 672");
    expect(flightName(plane({ callsign: "N123AB" }))).toBe("N123AB");
    expect(flightName(plane())).toBe("C08861");
    expect(fromTo(porter)).toBe("From San Francisco to Toronto");
    expect(fromTo(plane())).toBe("");
    expect(aircraftType(porter)).toBe("Embraer ERJ 190-400");
    expect(
      aircraftType(plane({ info: { hex: "x", type_name: "A350-1041", manufacturer: "Airbus Sas" } })),
    ).toBe("Airbus A350-1041");
    expect(operator(porter)).toBe("Porter Airlines");
    expect(label(porter)).toBe("POE672");
    expect(photoCredit(porter)).toBe("Photo by Frederick Tremblay, planespotters.net");
  });

  it("says where and when a plane comes into view", () => {
    const c = (side: NonNullable<Plane["coming"]>["side"], in_s: number) =>
      plane({ coming: { side, in_s, offset_deg: 0, elevation_deg: 10 } });
    expect(arrival(c("left", 130))).toBe("from the left in about 2 min");
    expect(arrival(c("rooftops", 30))).toBe("over the rooftops any moment now");
    expect(arrival(c("above", 300))).toBe("over the house in about 5 min");
    expect(arrival(plane())).toBe("");
  });
});

describe("pick", () => {
  const low = plane({ hex: "a", in_view: true, elevation_deg: 8 });
  const high = plane({ hex: "b", in_view: true, elevation_deg: 30 });
  const soon = plane({ hex: "c", coming: { side: "left", in_s: 40, offset_deg: -45, elevation_deg: 12 } });
  const later = plane({ hex: "d", coming: { side: "right", in_s: 200, offset_deg: 45, elevation_deg: 9 } });
  const away = plane({ hex: "e" });

  it("puts the highest plane in view first", () => {
    expect(inView([low, soon, high, away]).map((p) => p.hex)).toEqual(["b", "a"]);
  });

  it("lists what is coming, soonest first, without what is already in view", () => {
    expect(comingNext([later, low, soon, away]).map((p) => p.hex)).toEqual(["c", "d"]);
  });
});
