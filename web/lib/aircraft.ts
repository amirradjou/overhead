// A few facts about the airliner types that pass over Toronto most, by ICAO type designator, for one calm line while
// a plane is in view. Only facts that do not change from one airframe to the next (first flight, length, wingspan as
// built) and seat counts as airlines usually fit them; written for this screen, not copied from anywhere.
import type { Plane } from "./types";

interface TypeFacts {
  /** What people call it: "737 MAX 8". */
  name: string;
  firstFlight: number;
  lengthM: number;
  /** Left out where winglets fitted later make it differ between airframes. */
  wingspanM?: number;
  seats?: [number, number];
  note?: string;
}

const composite = "Half of it, by weight, is carbon-fibre composite rather than aluminium.";
const crj = "A Canadian design: Bombardier built the CRJ near Montreal.";
const cseries = "Designed in Canada, as the Bombardier CSeries.";

export const typeFacts: Record<string, TypeFacts> = {
  A319: { name: "A319", firstFlight: 1995, lengthM: 33.8, seats: [120, 150] },
  A320: { name: "A320", firstFlight: 1987, lengthM: 37.6, seats: [150, 180] },
  A321: { name: "A321", firstFlight: 1993, lengthM: 44.5, seats: [180, 220] },
  A20N: { name: "A320neo", firstFlight: 2014, lengthM: 37.6, wingspanM: 35.8, seats: [150, 190] },
  A21N: { name: "A321neo", firstFlight: 2016, lengthM: 44.5, wingspanM: 35.8, seats: [180, 220] },
  A332: { name: "A330-200", firstFlight: 1997, lengthM: 58.8, wingspanM: 60.3 },
  A333: { name: "A330-300", firstFlight: 1992, lengthM: 63.7, wingspanM: 60.3 },
  A339: { name: "A330-900", firstFlight: 2017, lengthM: 63.7, wingspanM: 64 },
  A359: { name: "A350-900", firstFlight: 2013, lengthM: 66.8, wingspanM: 64.8, note: composite },
  A35K: { name: "A350-1000", firstFlight: 2016, lengthM: 73.8, wingspanM: 64.8, note: composite },
  A388: {
    name: "A380",
    firstFlight: 2005,
    lengthM: 72.7,
    wingspanM: 79.8,
    note: "The largest airliner ever built, with two full decks.",
  },
  AT76: { name: "ATR 72", firstFlight: 1988, lengthM: 27.2, wingspanM: 27.1, seats: [68, 78] },
  B737: { name: "737-700", firstFlight: 1997, lengthM: 33.6, seats: [126, 149] },
  B738: { name: "737-800", firstFlight: 1997, lengthM: 39.5, seats: [160, 189] },
  B739: { name: "737-900", firstFlight: 2000, lengthM: 42.1, seats: [170, 215] },
  B38M: { name: "737 MAX 8", firstFlight: 2016, lengthM: 39.5, wingspanM: 35.9, seats: [160, 190] },
  B39M: { name: "737 MAX 9", firstFlight: 2017, lengthM: 42.2, wingspanM: 35.9, seats: [178, 220] },
  B752: { name: "757-200", firstFlight: 1982, lengthM: 47.3 },
  B763: { name: "767-300", firstFlight: 1986, lengthM: 54.9, wingspanM: 47.6 },
  B744: { name: "747-400", firstFlight: 1988, lengthM: 70.7, wingspanM: 64.4 },
  B748: { name: "747-8", firstFlight: 2010, lengthM: 76.3, wingspanM: 68.4 },
  B772: { name: "777-200", firstFlight: 1994, lengthM: 63.7, wingspanM: 60.9 },
  B77W: { name: "777-300ER", firstFlight: 2003, lengthM: 73.9, wingspanM: 64.8 },
  B788: { name: "787-8", firstFlight: 2009, lengthM: 56.7, wingspanM: 60.1, note: composite },
  B789: { name: "787-9", firstFlight: 2013, lengthM: 62.8, wingspanM: 60.1, note: composite },
  B78X: { name: "787-10", firstFlight: 2017, lengthM: 68.3, wingspanM: 60.1, note: composite },
  BCS1: {
    name: "A220-100",
    firstFlight: 2013,
    lengthM: 35,
    wingspanM: 35.1,
    seats: [100, 135],
    note: cseries,
  },
  BCS3: {
    name: "A220-300",
    firstFlight: 2015,
    lengthM: 38.7,
    wingspanM: 35.1,
    seats: [120, 160],
    note: cseries,
  },
  CRJ2: { name: "CRJ200", firstFlight: 1991, lengthM: 26.8, wingspanM: 21.2, seats: [50, 50], note: crj },
  CRJ9: { name: "CRJ900", firstFlight: 2001, lengthM: 36.2, wingspanM: 24.9, seats: [76, 90], note: crj },
  DH8D: {
    name: "Dash 8-400",
    firstFlight: 1998,
    lengthM: 32.8,
    wingspanM: 28.4,
    seats: [68, 90],
    note: "Designed by de Havilland Canada and built at Downsview, in Toronto.",
  },
  E75L: { name: "E175", firstFlight: 2003, lengthM: 31.7, seats: [76, 88] },
  E75S: { name: "E175", firstFlight: 2003, lengthM: 31.7, seats: [76, 88] },
  E190: { name: "E190", firstFlight: 2004, lengthM: 36.2, wingspanM: 28.7, seats: [96, 114] },
  E195: { name: "E195", firstFlight: 2004, lengthM: 38.7, wingspanM: 28.7, seats: [100, 124] },
  E290: { name: "E190-E2", firstFlight: 2016, lengthM: 36.2, wingspanM: 33.7, seats: [96, 114] },
  E295: { name: "E195-E2", firstFlight: 2017, lengthM: 41.5, wingspanM: 35.1, seats: [120, 146] },
};

const rinkLengthM = 61; // an NHL rink is 200 ft long
const rinkWidthM = 25.9; // and 85 ft wide
const busM = 12; // a city bus

const halves = ["", "one", "two", "three", "four", "five", "six", "seven"];

/** Something the size of a span or length, to picture it by: "about three city buses end to end". */
export function sizeLike(m: number): string {
  if (Math.abs(m - rinkLengthM) / rinkLengthM < 0.05) return "about the length of a hockey rink";
  if (m > rinkLengthM) return "longer than a hockey rink";
  if (Math.abs(m - rinkWidthM) / rinkWidthM < 0.05) return "about the width of a hockey rink";
  const n = Math.round((m / busM) * 2) / 2;
  const whole = Math.floor(n);
  if (n < 1.5) return "about the length of a city bus";
  const words = n === whole ? halves[whole] : `${halves[whole]} and a half`;
  return `about ${words} city buses end to end`;
}

/** The type's facts as short sentences, for one line at a time; none for a type not in the table. */
export function typeFactLines(p: Plane): string[] {
  const code = p.type_code ?? p.info?.type_code;
  const t = code ? typeFacts[code] : undefined;
  if (!t) return [];
  const lines: string[] = [];
  if (t.wingspanM) lines.push(`Wingspan ${t.wingspanM.toFixed(1)} m, ${sizeLike(t.wingspanM)}.`);
  else lines.push(`${t.lengthM.toFixed(1)} m long, ${sizeLike(t.lengthM)}.`);
  lines.push(`The ${t.name} first flew in ${t.firstFlight}.`);
  if (t.seats) {
    const [lo, hi] = t.seats;
    lines.push(lo === hi ? `It seats ${lo}.` : `Airlines fit it with ${lo} to ${hi} seats.`);
  }
  if (t.note) lines.push(t.note);
  return lines;
}
