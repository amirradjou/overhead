import { bearingDeg } from "./airport";
import { placeOnGround } from "./ground";
import { inFrame, nearestTurn } from "./sky";
import type { Aircraft, AtAirport, FollowedAirport, Plane, View } from "./types";

/**
 * A stretch of the public view, kept for the public page to play while the live feed is away
 * (scripts/record-replay.mjs). Each aircraft's details are kept once, not in every frame.
 */
export interface Recording {
  recorded_at: string;
  step_s: number;
  view: Omit<View, "at" | "planes" | "today" | "airport">;
  aircraft: Record<string, Aircraft>;
  frames: Frame[];
}

export interface Frame {
  at: string;
  today: number;
  airport?: FollowedAirport;
  planes: Plane[];
}

/** The recording as a live snapshot at `now`, `elapsedMs` into playing it: it loops, and its times are moved to now. */
export function frameAt(rec: Recording, elapsedMs: number, now: Date): View {
  const frame = rec.frames[Math.floor(elapsedMs / (rec.step_s * 1000)) % rec.frames.length]!;
  const shift = now.getTime() - Date.parse(frame.at);
  const moved = (at: string) => new Date(Date.parse(at) + shift).toISOString();
  return {
    ...rec.view,
    ...frame,
    at: now.toISOString(),
    weather: rec.view.weather && { ...rec.view.weather, at: now.toISOString() },
    planes: frame.planes.map((p) => ({ ...p, at: moved(p.at), info: rec.aircraft[p.hex] })),
  };
}

// A plane reports every 15 s, and live has to guess in between: it carries the plane on at its last speed, then
// corrects the guess when the next report arrives. A landing guessed that way sails past the runway at 100 knots.
// A recording knows what came next, so it can draw the true path through the reports instead.

/** The longest silence a plane's path is drawn across; beyond it the plane stays as recorded. */
const longestGapS = 45;

/** A plane on the ground has moved once it is this far from where it was, in metres: below this, its feed's noise. */
const movedM = 15;

interface Report {
  t: number; // the frame it first appears in
  plane: Plane;
  heading?: number;
}

/** Tangents for a curve through the points that never overshoots them: flat wherever the direction turns back. */
function tangents(ts: number[], vs: number[]): number[] {
  const slopes = ts.slice(1).map((t, i) => (vs[i + 1]! - vs[i]!) / (t - ts[i]!));
  return ts.map((_, i) => {
    const a = slopes[i - 1];
    const b = slopes[i];
    if (a === undefined) return b ?? 0;
    if (b === undefined) return a;
    return a * b <= 0 ? 0 : (2 * a * b) / (a + b);
  });
}

function hermite(u: number, h: number, v0: number, v1: number, m0: number, m1: number): number {
  const u2 = u * u;
  const u3 = u2 * u;
  return (
    (2 * u3 - 3 * u2 + 1) * v0 + (u3 - 2 * u2 + u) * h * m0 + (-2 * u3 + 3 * u2) * v1 + (u3 - u2) * h * m1
  );
}

const between = (a: number | undefined, b: number | undefined, u: number) =>
  a === undefined || b === undefined ? a : a + (b - a) * u;

function metresApart(a: Plane, b: Plane): number {
  const r = Math.PI / 180;
  return Math.hypot((a.lat - b.lat) * 110540, (a.lon - b.lon) * 111320 * Math.cos(a.lat * r));
}

/** Which way each report's plane points: on the ground, the way it is moving (feeds repeat a stale track there). */
function headings(run: Report[]): void {
  let last: number | undefined;
  run.forEach((r, k) => {
    const next = run[k + 1]?.plane;
    const rolling = r.plane.on_ground && next && metresApart(r.plane, next) > movedM;
    r.heading = rolling
      ? bearingDeg(r.plane, next)
      : ((r.plane.on_ground ? last : undefined) ?? r.plane.track_deg ?? last);
    last = r.heading;
  });
}

/** The airport numbers between two reports. */
function airportBetween(
  a: AtAirport | undefined,
  b: AtAirport | undefined,
  u: number,
): AtAirport | undefined {
  if (!a || !b) return a;
  return {
    ...a,
    distance_km: between(a.distance_km, b.distance_km, u)!,
    height_ft: between(a.height_ft, b.height_ft, u),
    in_min: between(a.in_min, b.in_min, u),
    ago_min: between(a.ago_min, b.ago_min, u),
  };
}

/**
 * The recording with each plane on its true path between reports, and on the ground where it is on the ground, so a
 * landing touches down on the drawn runway, rolls out and taxis to its gate.
 */
export function smoothed(rec: Recording): Recording {
  const w = rec.view.window;
  const placed = rec.frames.map((f) => ({ ...f, planes: placeOnGround(f.planes, w, f.airport) }));

  const reports = new Map<string, Report[]>();
  placed.forEach((f, t) => {
    for (const plane of f.planes) {
      const seen = reports.get(plane.hex) ?? [];
      if (seen[seen.length - 1]?.plane.at !== plane.at) seen.push({ t, plane });
      reports.set(plane.hex, seen);
    }
  });

  // Each plane's frames between two reports, as what to draw there.
  const drawn = new Map<string, Map<number, Plane>>();
  const longestGap = longestGapS / rec.step_s;
  for (const [hex, seen] of reports) {
    const at = new Map<number, Plane>();
    drawn.set(hex, at);
    // Unbroken runs of reports: a plane that goes silent for long is not drawn across the silence.
    const runs: Report[][] = [];
    seen.forEach((r, k) => {
      if (k === 0 || r.t - seen[k - 1]!.t > longestGap) runs.push([]);
      runs[runs.length - 1]!.push(r);
    });
    for (const run of runs) {
      headings(run);
      const ts = run.map((r) => r.t);
      const off = run.map((r) => r.plane.offset_deg);
      const el = run.map((r) => r.plane.elevation_deg);
      const mOff = tangents(ts, off);
      const mEl = tangents(ts, el);
      for (let k = 0; k + 1 < run.length; k++) {
        const a = run[k]!;
        const b = run[k + 1]!;
        const h = b.t - a.t;
        for (let t = a.t; t < b.t; t++) {
          const u = (t - a.t) / h;
          const own = placed[t]!.planes.find((p) => p.hex === hex);
          if (!own) continue;
          const offset_deg = hermite(u, h, off[k]!, off[k + 1]!, mOff[k]!, mOff[k + 1]!);
          const elevation_deg = hermite(u, h, el[k]!, el[k + 1]!, mEl[k]!, mEl[k + 1]!);
          const distance_km = between(a.plane.distance_km, b.plane.distance_km, u)!;
          const track =
            a.heading === undefined || b.heading === undefined
              ? a.heading
              : a.heading + (nearestTurn(a.heading, b.heading) - a.heading) * u;
          at.set(t, {
            ...own,
            offset_deg,
            elevation_deg,
            distance_km,
            in_view: distance_km <= 40 && inFrame(w, offset_deg, elevation_deg),
            track_deg: track === undefined ? undefined : ((track % 360) + 360) % 360,
            speed_kt: between(a.plane.speed_kt, b.plane.speed_kt, u),
            alt_ft: between(a.plane.alt_ft, b.plane.alt_ft, u),
            alt_msl_ft: between(a.plane.alt_msl_ft, b.plane.alt_msl_ft, u),
            vert_rate_fpm: between(a.plane.vert_rate_fpm, b.plane.vert_rate_fpm, u),
            airport: airportBetween(own.airport, b.plane.airport, u),
          });
        }
      }
    }
  }
  return {
    ...rec,
    frames: placed.map((f, t) => ({
      ...f,
      planes: f.planes.map((p) => drawn.get(p.hex)?.get(t) ?? p),
    })),
  };
}
