// The window drawn on screen: across is direction (the window's slice of sky, left to right), up is the angle above
// the horizon, from the lowest angle visible over the rooftops to the top of the screen.
import type { Window } from "./types";

/** The earth's radius with standard refraction, in metres, as internal/geo. */
export const refractedEarthM = 6371008.8 * (4 / 3);

/** The highest angle drawn for a window without a top edge; anything above it sits at the top of the screen. */
export const topDeg = 70;

/** The angle at the top of the screen: the window's top edge, or topDeg when it has none (or a higher one). */
export function screenTop(w: Window): number {
  const edge = w.max_elevation_deg;
  return edge !== undefined && edge > w.min_elevation_deg && edge < topDeg ? edge : topDeg;
}

/** Where the rooftops end and the sky begins, as a fraction of the screen's height from the top. */
export const roofline = 0.72;

/**
 * Whether the window looks down past the horizon, as from high up with nothing opposite: then the bottom of the
 * screen is water or land below the horizon, not rooftops.
 */
export function looksDown(w: Window): boolean {
  return w.min_elevation_deg < 0;
}

/** The horizon's height on screen, as a fraction from the top, for a window that looks down past it. */
export function horizonY(w: Window): number | undefined {
  return looksDown(w) ? toScreen(w, 0, 0).y : undefined;
}

/** Screen position, each 0 to 1, of something seen offsetDeg right of the window's centre and elevationDeg up. */
export function toScreen(w: Window, offsetDeg: number, elevationDeg: number): { x: number; y: number } {
  const x = 0.5 + offsetDeg / w.fov_deg;
  const up = (elevationDeg - w.min_elevation_deg) / (screenTop(w) - w.min_elevation_deg);
  const y = roofline * (1 - Math.min(1, Math.max(0, up)));
  return { x: Math.min(1, Math.max(0, x)), y };
}

/**
 * The angle to turn to, as close to prev as it can be: next plus or minus whole turns. A plane flying right to left
 * points at about ±180°, and a heading that wobbles across that line must turn a few degrees, not spin round.
 */
export function nearestTurn(prev: number, next: number): number {
  return prev + ((((next - prev) % 360) + 540) % 360) - 180;
}

/** How far right of the window's centre something at azimuth azDeg is, in degrees from -180 to 180. */
export function offsetFrom(w: Window, azDeg: number): number {
  return ((((azDeg - w.azimuth_deg) % 360) + 540) % 360) - 180;
}

/** Whether something offsetDeg right of the window's centre and elevationDeg up is in the frame: over the roofs, below
 * the top edge and between the sides. */
export function inFrame(w: Window, offsetDeg: number, elevationDeg: number): boolean {
  return (
    Math.abs(offsetDeg) <= w.fov_deg / 2 &&
    elevationDeg >= w.min_elevation_deg &&
    elevationDeg <= screenTop(w)
  );
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

/**
 * How far to turn the overview disc so the window's direction is at the top: the window's azimuth, or none for a
 * window that sees all round (north up, as a map would be).
 */
export function overviewTurn(w: Window): number {
  return w.fov_deg >= 360 ? 0 : w.azimuth_deg;
}

/** A point at a compass angle (clockwise from the top) and a distance from the centre, in SVG's y-down space. */
export function polar(deg: number, d: number): { x: number; y: number } {
  const a = (deg * Math.PI) / 180;
  return { x: d * Math.sin(a), y: -d * Math.cos(a) };
}

/** A small seeded random generator (mulberry32), so the rooftops come out the same on every load. */
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
  const bleed = 40;
  const rand = seeded(seed);
  const top = roofline * 1000;
  // One closed outline, left to right along the roofs: a chimney is a detour up and down within it, never a new
  // sub-path, or the fill breaks apart.
  // The outline runs a little past the box on every side, so the drift against burn-in never shows an edge.
  const pts: [number, number][] = [
    [-bleed, 1000 + bleed],
    [-bleed, top + 40],
  ];
  let x = -bleed;
  while (x < 1000 + bleed) {
    const width = 25 + rand() * 70;
    const drop = rand() < 0.25 ? rand() * 70 : rand() * 18; // most buildings near the line, a few lower ones
    const y = top + drop;
    const x2 = Math.min(1000 + bleed, x + width);
    pts.push([x, y]);
    if (rand() < 0.15) {
      const cx = x + width * (0.2 + rand() * 0.5); // a chimney or a mast
      const h = 6 + rand() * 10;
      pts.push([cx, y], [cx, y - h], [cx + 4, y - h], [cx + 4, y]);
    }
    pts.push([x2, y]);
    x = x2;
  }
  pts.push([1000 + bleed, 1000 + bleed]);
  return "M" + pts.map(([px, py]) => `${px.toFixed(1)},${py.toFixed(1)}`).join(" L") + " Z";
}

/** The window's sill, flat along the roofline, for a window with nothing standing opposite (see looksDown). */
export function sill(): string {
  const bleed = 40;
  const top = roofline * 1000;
  return `M${-bleed},${1000 + bleed} L${-bleed},${top} L${1000 + bleed},${top} L${1000 + bleed},${1000 + bleed} Z`;
}
