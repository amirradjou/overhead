// The side view of a flight at the followed airport: distance from the runway across, height above it up, the 3°
// path planes fly down to the runway, the height of the window, and where the plane has been. Arrivals come in from
// the left to the runway on the right; departures leave the runway on the left.
import type { AtAirport } from "./types";

/** A moment of the flight: km from the airport and metres above it. */
export interface ProfilePoint {
  km: number;
  m: number;
}

export interface Profile {
  departing: boolean;
  /** Fractions of the chart: x from its left, y from its top. */
  plane: [number, number];
  trail: [number, number][];
  /** The 3° path from the runway, to where it leaves the chart. */
  glide: [[number, number], [number, number]];
  /** The window's height, as a fraction from the top; null when it is above the chart. */
  windowY: number | null;
  runwayX: number;
  /** Against the 3° path, for an arrival within 12 km: "on the 3° path", "above it", "below it"; else "". */
  path: string;
  maxKm: number;
}

const glideMPerKm = Math.tan((3 * Math.PI) / 180) * 1000; // 52.4 m a kilometre

export function profile(at: AtAirport, trail: ProfilePoint[], windowAboveFieldM: number): Profile | null {
  if (at.op === "ground" && at.height_ft === undefined && at.distance_km > 3) return null;
  const departing = at.op === "departing";
  const m = (at.height_ft ?? 0) * 0.3048;
  const km = at.distance_km;
  const points = [...trail, { km, m }];
  const maxKm = Math.min(20, Math.max(4, ...points.map((p) => p.km * 1.15)));
  const maxM = Math.max(
    250,
    1.35 * Math.max(m, windowAboveFieldM, ...points.map((p) => p.m), glideMPerKm * km * 0.8),
  );
  const fx = (k: number) => (departing ? k / maxKm : 1 - k / maxKm);
  const fy = (h: number) => 1 - h / maxM;
  const runwayX = fx(0);
  // The 3° path leaves the chart at its far side or its top, whichever comes first.
  const endKm = Math.min(maxKm, maxM / glideMPerKm);
  let path = "";
  if (at.op === "arriving" && at.height_ft !== undefined && km <= 12 && km > 0.3) {
    const ratio = m / (glideMPerKm * km);
    path = ratio > 1.3 ? "above the 3° path" : ratio < 0.75 ? "below the 3° path" : "on the 3° path";
  }
  return {
    departing,
    plane: [fx(km), fy(m)],
    trail: trail.map((p) => [fx(p.km), fy(p.m)]),
    glide: [
      [runwayX, 1],
      [fx(endKm), fy(glideMPerKm * endKm)],
    ],
    windowY: windowAboveFieldM < maxM ? fy(windowAboveFieldM) : null,
    runwayX,
    path,
    maxKm,
  };
}
