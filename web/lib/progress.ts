// How far along its trip a plane is: great-circle distances from where it took off and to where it is going, from
// the airports' coordinates in the route, and the time left at the speed it is flying now.
import { landing, place, trend } from "./format";
import type { Plane } from "./types";

const earthKm = 6371.0088; // mean radius
const rad = Math.PI / 180;

/** The great-circle distance between two points, in km (haversine). */
export function greatCircleKm(a: { lat: number; lon: number }, b: { lat: number; lon: number }): number {
  const dLat = (b.lat - a.lat) * rad;
  const dLon = (b.lon - a.lon) * rad;
  const h = Math.sin(dLat / 2) ** 2 + Math.cos(a.lat * rad) * Math.cos(b.lat * rad) * Math.sin(dLon / 2) ** 2;
  return 2 * earthKm * Math.asin(Math.min(1, Math.sqrt(h)));
}

export interface Progress {
  fromCode: string;
  toCode: string;
  fromName: string;
  toName: string;
  flownKm: number;
  toGoKm: number;
  /** Share of the trip done, 0 to 1. */
  done: number;
  /** Minutes to the destination at the current speed, when that makes sense. */
  minutesLeft?: number;
}

/** The plane's progress, or null when the route or an airport's position is unknown. */
export function flightProgress(p: Plane): Progress | null {
  const r = p.info?.route;
  const o = r?.origin;
  const d = r?.destination;
  if (!o || !d || o.lat === undefined || o.lon === undefined || d.lat === undefined || d.lon === undefined)
    return null;
  if ((o.lat === 0 && o.lon === 0) || (d.lat === 0 && d.lon === 0)) return null; // unknown, sent as zero
  const here = { lat: p.lat, lon: p.lon };
  const flownKm = greatCircleKm({ lat: o.lat, lon: o.lon }, here);
  const toGoKm = greatCircleKm(here, { lat: d.lat, lon: d.lon });
  if (flownKm + toGoKm < 1) return null;
  return {
    fromCode: place(o).code,
    toCode: place(d).code,
    fromName: place(o).name || place(o).code,
    toName: place(d).name || place(d).code,
    flownKm,
    toGoKm,
    done: flownKm / (flownKm + toGoKm),
    minutesLeft: minutesLeft(p, toGoKm),
  };
}

// The time left at the current ground speed: the server's own estimate near the destination, otherwise only for a
// plane that is not climbing (one climbing out is far slower than it will cruise) and fast enough to be flying.
function minutesLeft(p: Plane, toGoKm: number): number | undefined {
  if (p.eta_min !== undefined) return p.eta_min;
  if (p.on_ground || p.speed_kt === undefined || p.speed_kt < 150 || trend(p) === "climbing")
    return undefined;
  return (toGoKm / (p.speed_kt * 1.852)) * 60;
}

const thousands = new Intl.NumberFormat("en-CA");

/** "40 km", "380 km", "3,640 km": to the nearest km under 100, then to the nearest 10. */
export function km(n: number): string {
  return `${thousands.format(n < 100 ? Math.round(n) : Math.round(n / 10) * 10)} km`;
}

/** "4 min", "1 h 05 min", "4 h 10 min": to the minute under an hour, then to five minutes. */
export function duration(minutes: number): string {
  if (minutes < 59.5) return `${Math.max(1, Math.round(minutes))} min`;
  const total = Math.round(minutes / 5) * 5;
  const h = Math.floor(total / 60);
  const m = total % 60;
  return m ? `${h} h ${String(m).padStart(2, "0")} min` : `${h} h`;
}

/**
 * The line under the route, which already names both ends: "3,640 km flown, about 25 min to go". The time is left
 * out when the screen already says when the plane lands, or when it cannot be worked out; the distance is said.
 */
export function progressText(p: Plane, pr: Progress): string {
  const flown = `${km(pr.flownKm)} flown`;
  if (pr.minutesLeft !== undefined && !landing(p)) return `${flown}, about ${duration(pr.minutesLeft)} to go`;
  return `${flown}, ${km(pr.toGoKm)} to go`;
}
