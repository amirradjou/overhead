// A plane on the ground reports no height, so the live API cannot say where the window sees it. At the followed
// airport the height is known, the airport's own: that keeps a landing on screen through its roll-out and taxi, and
// a take-off from the moment it lines up, instead of the plane vanishing at touchdown.
import { inFrame, refractedEarthM } from "./sky";
import type { FollowedAirport, Plane, Window } from "./types";

const visibleKm = 40; // as internal/live

/** Degrees above (+) or below (-) the horizon of something at groundM above sea level, distanceKm from the window. */
export function groundElevationDeg(distanceKm: number, groundM: number, eyeM: number): number {
  const d = Math.max(1, distanceKm * 1000);
  return (Math.atan((groundM - eyeM) / d - d / (2 * refractedEarthM)) * 180) / Math.PI;
}

/** The planes, with those on the ground at the followed airport placed on it. */
export function placeOnGround(planes: Plane[], w: Window, airport?: FollowedAirport): Plane[] {
  if (!airport || !planes.some((p) => p.on_ground && p.airport)) return planes;
  const groundM = airport.elevation_ft * 0.3048;
  return planes.map((p) => {
    if (!p.on_ground || !p.airport) return p;
    const elevation_deg = groundElevationDeg(p.distance_km, groundM, w.home_elevation_m);
    return {
      ...p,
      elevation_deg,
      in_view: p.distance_km <= visibleKm && inFrame(w, p.offset_deg, elevation_deg),
    };
  });
}
