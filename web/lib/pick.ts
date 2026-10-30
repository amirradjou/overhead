// Which planes the screen talks about.
import { rank } from "./airport";
import type { Plane } from "./types";

/**
 * The planes the window shows, the one to feature first: a landing or take-off at the followed airport, then a pass
 * very close to the window, then the highest in the sky (the higher, the easier to spot).
 */
export function inView(planes: Plane[]): Plane[] {
  return planes
    .filter((p) => p.in_view)
    .sort((a, b) => rank(a) - rank(b) || b.elevation_deg - a.elevation_deg);
}

/** The planes about to come into view, soonest first. */
export function comingNext(planes: Plane[]): Plane[] {
  return planes
    .filter((p) => !p.in_view && p.coming)
    .sort((a, b) => (a.coming?.in_s ?? Infinity) - (b.coming?.in_s ?? Infinity));
}

/** One item from a list, taking turns: the item changes every `every` ms of the clock. */
export function rotating<T>(items: T[], now: Date | null, every: number): T | undefined {
  if (!items.length || !now) return undefined;
  return items[Math.floor(now.getTime() / every) % items.length];
}
