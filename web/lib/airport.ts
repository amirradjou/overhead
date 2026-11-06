// The airport the screen follows (overhead live -airport): which planes a focused screen shows, and the words for a
// flight landing at it or taking off from it, seen from a window that may look down on it.
import { altFt, place } from "./format";
import { duration } from "./progress";
import type { FollowedAirport, Plane, View, Window } from "./types";

const thousands = new Intl.NumberFormat("en-CA");

/** The planes the screen talks about: on a focused screen, only those marked focus. */
export function shown(view: View | null): Plane[] {
  const planes = view?.planes ?? [];
  return view?.focused ? planes.filter((p) => p.focus) : planes;
}

// Which plane to feature when several are in view: the one doing something worth watching right now.
const phaseRank: Record<string, number> = {
  landing: 0,
  takeoff: 0,
  runway: 1,
  final: 1,
  climbing: 2,
  approach: 3,
  taxiing: 4,
  gate: 5,
  arrived: 5,
};

/** Lower is more worth featuring: a landing or take-off first, then a near pass, then the rest. */
export function rank(p: Plane): number {
  if (p.airport) return phaseRank[p.airport.phase] ?? 6;
  return p.near ? 2 : 6;
}

/** "Billy Bishop" for the followed airport's own end of a route, else the city: "Boston". */
export function endName(
  a: { iata?: string; icao?: string; city?: string; name?: string },
  ap?: FollowedAirport,
): string {
  if (ap && (a.icao === ap.icao || a.iata === ap.iata)) return ap.name;
  return place(a).name || place(a).code;
}

/** "From Boston to Billy Bishop", naming the followed airport by its own name rather than its city. */
export function fromToHere(p: Plane, ap?: FollowedAirport): string {
  const r = p.info?.route;
  if (!r) return "";
  const from = endName(r.origin, ap);
  const to = endName(r.destination, ap);
  return from && to ? `From ${from} to ${to}` : "";
}

const inMin = (m?: number) => (m === undefined ? "" : m < 0.75 ? "any moment" : `in about ${duration(m)}`);
const agoMin = (m?: number) => (m === undefined ? "" : m < 0.75 ? "just now" : `about ${duration(m)} ago`);

/**
 * What the plane is doing at the airport, as the line over its name: "On final for runway 26, touchdown in about
 * 2 min", "Taking off from runway 08", "Climbing out, took off about 3 min ago".
 */
export function airportStatus(p: Plane, ap?: FollowedAirport): string {
  const a = p.airport;
  if (!a) return p.near ? "Very close to your window" : "";
  const rw = a.runway ?? ap?.runway_in_use;
  const onRw = (prep: string) => (rw ? ` ${prep} runway ${rw}` : "");
  switch (a.phase) {
    case "approach": {
      const when = inMin(a.in_min);
      return `On approach${when && `, landing ${when}`}`;
    }
    case "final": {
      const when = inMin(a.in_min);
      return `On final${onRw("for")}${when && `, touchdown ${when}`}`;
    }
    case "landing":
      return `Landing${onRw("on")}`;
    case "taxiing":
      return a.op === "arriving"
        ? "Landed, taxiing to the gate"
        : a.op === "departing"
          ? `Taxiing out${onRw("to")}`
          : "Taxiing";
    case "arrived":
      return "Arrived, at the gate";
    case "gate":
      return "At the gate, getting ready to leave";
    case "takeoff":
      return a.height_ft !== undefined ? `Lifting off${onRw("from")}` : `Taking off${onRw("from")}`;
    case "climbing": {
      const ago = agoMin(a.ago_min);
      return `Climbing out${ago && `, took off ${ago}`}`;
    }
    case "runway":
      return `On the runway${rw ? ` ${rw}` : ""}`;
  }
}

/**
 * The plane's height against the window's: "45 m below your window", "320 m above your window", "level with your
 * window". Up to a kilometre in metres, then in kilometres.
 */
export function heightFromWindow(p: Plane, w: Window): string {
  if (p.on_ground) return "on the ground";
  const ft = altFt(p);
  if (ft === undefined) return "";
  const diff = ft * 0.3048 - w.home_elevation_m;
  if (Math.abs(diff) < 15) return "level with your window";
  const size = Math.abs(diff);
  const amount = size < 1000 ? `${Math.round(size / 5) * 5} m` : `${(size / 1000).toFixed(1)} km`;
  return `${amount} ${diff < 0 ? "below" : "above"} your window`;
}

/** "1,150 ft", "On the ground". */
export function feet(p: Plane): string {
  if (p.on_ground) return "On the ground";
  const ft = altFt(p);
  return ft === undefined ? "" : `${thousands.format(Math.round(ft / 25) * 25)} ft`;
}

/** "descending 700 ft/min", "climbing 1,800 ft/min", "level", or "" on the ground or when unknown. */
export function vertical(p: Plane): string {
  if (p.on_ground || p.vert_rate_fpm === undefined) return "";
  const v = Math.round(Math.abs(p.vert_rate_fpm) / 50) * 50;
  if (v < 300) return "level";
  return `${p.vert_rate_fpm < 0 ? "descending" : "climbing"} ${thousands.format(v)} ft/min`;
}

/** Where to look from the window: "6° below the horizon, a little to the right". */
export function lookWhere(p: Plane): string {
  const e = Math.round(p.elevation_deg);
  const up =
    Math.abs(p.elevation_deg) < 1 ? "on the horizon" : e < 0 ? `${-e}° below the horizon` : `${e}° up`;
  const off = Math.abs(p.offset_deg);
  const side = p.offset_deg < 0 ? "left" : "right";
  const across = off < 8 ? "straight ahead" : off < 25 ? `a little to the ${side}` : `to the ${side}`;
  return `${up}, ${across}`;
}

/** Where to look, in a few words for a tile: "4° down, right", "14° up, ahead". */
export function lookShort(p: Plane): string {
  const e = Math.round(p.elevation_deg);
  const up = Math.abs(p.elevation_deg) < 1 ? "level" : e < 0 ? `${-e}° down` : `${e}° up`;
  const side = Math.abs(p.offset_deg) < 8 ? "ahead" : p.offset_deg < 0 ? "left" : "right";
  return `${up}, ${side}`;
}

/** One row of the airport's board. */
export interface BoardRow {
  hex: string;
  /** ↓ landing, ↑ taking off, · on the ground with no route. */
  arrow: string;
  flight: string;
  /** Where from (arriving) or to (departing). */
  place: string;
  status: string;
}

// Times on the board are to touchdown and since take-off, said as such: the cue on the sky counts down to the
// window instead.
const lands = (p: Plane, otherwise: string) =>
  p.airport?.in_min !== undefined && p.airport.in_min >= 0.75
    ? `lands in ${duration(p.airport.in_min)}`
    : otherwise;
const boardStatus: Record<string, (p: Plane) => string> = {
  approach: (p) => lands(p, "approach"),
  final: (p) => lands(p, "landing"),
  landing: () => "landing",
  taxiing: () => "taxiing",
  arrived: () => "arrived",
  gate: () => "at the gate",
  takeoff: () => "taking off",
  climbing: (p) =>
    p.airport?.ago_min !== undefined ? `left ${duration(p.airport.ago_min)} ago` : "departed",
  runway: () => "on the runway",
};

/** The flight as the board lists it: "PD 2943", an airline callsign, else the registration ("C-GRYY"). */
function boardFlight(p: Plane): string {
  // An airline code has a letter in it: "1813" (as adsbdb gives some flights) is not "18 13".
  const iata = p.info?.route?.flight_iata?.match(/^([A-Z][A-Z0-9]|[0-9][A-Z])(\d+[A-Z]?)$/);
  if (iata) return `${iata[1]} ${iata[2]}`;
  if (p.callsign && /^[A-Z]{3}\d/.test(p.callsign)) return p.callsign;
  return p.info?.registration || p.registration || p.callsign || p.hex.toUpperCase();
}

/**
 * The airport's board: every plane landing, taking off or moving there, the busiest first: rolling and on final,
 * then the next arrivals by time to touchdown, then departures climbing out, then the ground.
 */
export function board(view: View | null, max = 6): BoardRow[] {
  if (!view?.airport) return [];
  const ap = view.airport;
  return view.planes
    .filter((p) => p.airport)
    .sort((a, b) => rank(a) - rank(b) || (a.airport?.in_min ?? 99) - (b.airport?.in_min ?? 99))
    .slice(0, max)
    .map((p) => {
      const at = p.airport!;
      const r = p.info?.route;
      const other = at.op === "arriving" ? r?.origin : at.op === "departing" ? r?.destination : undefined;
      return {
        hex: p.hex,
        arrow: at.op === "arriving" ? "↓" : at.op === "departing" ? "↑" : "·",
        flight: boardFlight(p),
        place: other ? endName(other, ap) : "",
        status: boardStatus[at.phase]?.(p) ?? "",
      };
    });
}

/** "Runway 26 in use. 12 landings and 11 take-offs today." */
export function airportDay(ap: FollowedAirport): string {
  if (ap.uncounted) return ap.runway_in_use ? `Runway ${ap.runway_in_use} in use.` : "";
  const n = (k: number, one: string, many: string) => `${k} ${k === 1 ? one : many}`;
  const day = `${n(ap.arrivals_today, "landing", "landings")} and ${n(ap.departures_today, "take-off", "take-offs")} today`;
  return ap.runway_in_use
    ? `Runway ${ap.runway_in_use} in use. ${day}.`
    : `${day.charAt(0).toUpperCase()}${day.slice(1)}.`;
}

/** The bearing from a to b, degrees clockwise from north. */
export function bearingDeg(a: { lat: number; lon: number }, b: { lat: number; lon: number }): number {
  const r = Math.PI / 180;
  const y = Math.sin((b.lon - a.lon) * r) * Math.cos(b.lat * r);
  const x =
    Math.cos(a.lat * r) * Math.sin(b.lat * r) -
    Math.sin(a.lat * r) * Math.cos(b.lat * r) * Math.cos((b.lon - a.lon) * r);
  return (((Math.atan2(y, x) / r) % 360) + 360) % 360;
}
