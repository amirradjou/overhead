import type { Aircraft, FollowedAirport, Plane, View } from "./types";

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
