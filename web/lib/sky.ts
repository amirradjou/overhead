// The window drawn on screen: across is direction (the window's slice of sky, left to right), up is the angle above
// the horizon, from the lowest angle visible over the rooftops to the top of the screen.
import type { Window } from "./types";

/** The highest angle drawn; anything above it sits at the top edge, as it would at the top of a window frame. */
export const topDeg = 70;

/** Where the rooftops end and the sky begins, as a fraction of the screen's height from the top. */
export const roofline = 0.72;

/** Screen position, each 0 to 1, of something seen offsetDeg right of the window's centre and elevationDeg up. */
export function toScreen(w: Window, offsetDeg: number, elevationDeg: number): { x: number; y: number } {
  const x = 0.5 + offsetDeg / w.fov_deg;
  const up = (elevationDeg - w.min_elevation_deg) / (topDeg - w.min_elevation_deg);
  const y = roofline * (1 - Math.min(1, Math.max(0, up)));
  return { x: Math.min(1, Math.max(0, x)), y };
}

/** The compass points across the window, with their screen position. */
export function compassTicks(w: Window): { label: string; x: number }[] {
  const names = ["N", "NE", "E", "SE", "S", "SW", "W", "NW"];
  const ticks: { label: string; x: number }[] = [];
  for (let i = 0; i < 8; i++) {
    let off = ((i * 45 - w.azimuth_deg + 540) % 360) - 180; // -180..180 from the window's centre
    if (Math.abs(off) > w.fov_deg / 2) continue;
    if (w.fov_deg >= 360 && off === -180) off = 180; // all round: show the point behind once, on the right
    ticks.push({ label: names[i] ?? "", x: 0.5 + off / w.fov_deg });
  }
  return ticks;
}

/** A small seeded random generator (mulberry32), so stars and rooftops come out the same on every load. */
export function seeded(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

/**
 * The rooftops opposite, as an SVG path in a 1000×1000 box: blocks of varying width whose tops reach the roofline at
 * most, never above it, because the roofline is the lowest angle the window sees over them.
 */
export function rooftops(seed = 7): string {
  const rand = seeded(seed);
  const top = roofline * 1000;
  let x = 0;
  let d = `M0 1000 L0 ${top + 40}`;
  while (x < 1000) {
    const width = 25 + rand() * 70;
    const drop = rand() < 0.25 ? rand() * 70 : rand() * 18; // most buildings near the line, a few lower ones
    const y = top + drop;
    d += ` L${x.toFixed(1)} ${y.toFixed(1)} L${Math.min(1000, x + width).toFixed(1)} ${y.toFixed(1)}`;
    if (rand() < 0.15) {
      const cx = x + width * (0.2 + rand() * 0.5); // a chimney or a mast
      d += ` M${cx.toFixed(1)} ${y.toFixed(1)} L${cx.toFixed(1)} ${(y - 6 - rand() * 10).toFixed(1)} L${(cx + 4).toFixed(1)} ${(y - 6).toFixed(1)} L${(cx + 4).toFixed(1)} ${y.toFixed(1)} M${Math.min(1000, x + width).toFixed(1)} ${y.toFixed(1)}`;
    }
    x += width;
  }
  return `${d} L1000 1000 Z`;
}

/** Stars for a night sky: positions above the roofline and a brightness each. */
export function stars(count: number, seed = 11): { x: number; y: number; a: number; r: number }[] {
  const rand = seeded(seed);
  return Array.from({ length: count }, () => ({
    x: rand(),
    y: rand() * roofline * 0.95,
    a: 0.25 + rand() * 0.6,
    r: rand() < 0.08 ? 1.6 : 0.9,
  }));
}
