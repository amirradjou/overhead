// The view from a high window over Toronto's harbour, drawn from OpenStreetMap: the lake out to its horizon, the
// islands, Billy Bishop's runways, the piers and the buildings of the harbour front. Everything is placed with the
// geometry the planes use (direction across the window, angle above or below the horizon, the earth's curve with
// refraction), so a plane on its way down touches the drawn runway, as it does outside.
import harbourData from "./scenery/toronto-harbour.json";
import { greatCircleKm } from "./progress";
import { looksDown, refractedEarthM, roofline, screenTop, seeded } from "./sky";
import type { Window } from "./types";

export type LatLon = [number, number];

export interface Harbour {
  about: string;
  /** Heights above sea level, in metres, of the lake and the land round it. */
  lake_m: number;
  ground_m: number;
  /** The lake's islands, as closed rings; the mainland, closed well inland. */
  islands: LatLon[][];
  mainland: LatLon[];
  runways: { ref: string; width: number; line: LatLon[] }[];
  aprons: LatLon[][];
  piers: { kind: string; area: boolean; pts: LatLon[] }[];
  buildings: { h: number; ring: LatLon[] }[];
}

export const harbour = harbourData as unknown as Harbour;
const harbourCentre = { lat: 43.627, lon: -79.385 };

/** The scenery to draw for a window at home: the harbour, for a window high over it; null anywhere else. */
export function sceneryFor(home: { lat: number; lon: number }, w: Window): Harbour | null {
  return looksDown(w) && greatCircleKm(home, harbourCentre) < 4 ? harbour : null;
}

const rad = Math.PI / 180;

/** A point in metres from home: f straight out of the window, r to its right. */
export type Local = [number, number];

/** Nearer than this, in front of the window, nothing is drawn: it is under the window, out of sight. */
const nearestM = 60;

export interface Projector {
  /** A place, in metres out from and right of the window. */
  local(p: LatLon): Local;
  /** Where a point h metres above sea level appears: fractions of the screen's width and height (not clamped). */
  screen(p: Local, h: number): [number, number];
  /** The lake's horizon, as a fraction of the screen's height: from high up, a little below level. */
  horizonY: number;
}

export function projector(home: { lat: number; lon: number }, w: Window, lakeM: number): Projector {
  const az = w.azimuth_deg * rad;
  const kx = 111320 * Math.cos(home.lat * rad);
  const ky = 110540;
  const eye = w.home_elevation_m;
  const min = w.min_elevation_deg;
  const span = screenTop(w) - min;
  const y = (el: number) => roofline * (1 - (el - min) / span);
  // The farthest water seen lies where the earth's curve meets the line of sight: sqrt(2·drop/R) below level.
  const dip = -Math.sqrt((2 * Math.max(0, eye - lakeM)) / refractedEarthM) / rad;
  return {
    local([lat, lon]) {
      const x = (lon - home.lon) * kx;
      const n = (lat - home.lat) * ky;
      return [x * Math.sin(az) + n * Math.cos(az), x * Math.cos(az) - n * Math.sin(az)];
    },
    screen([f, r], h) {
      const d = Math.hypot(f, r);
      const off = Math.atan2(r, f) / rad;
      const el = Math.atan((h - eye) / d - d / (2 * refractedEarthM)) / rad;
      return [0.5 + off / w.fov_deg, y(el)];
    },
    horizonY: y(dip),
  };
}

/** Extra points along edges longer than step metres, so a straight edge outside bends as it does on screen. */
export function densify(pts: Local[], step = 40, closed = true): Local[] {
  const out: Local[] = [];
  const n = closed ? pts.length : pts.length - 1;
  for (let i = 0; i < n; i++) {
    const a = pts[i]!;
    const b = pts[(i + 1) % pts.length]!;
    out.push(a);
    const k = Math.floor(Math.hypot(b[0] - a[0], b[1] - a[1]) / step);
    for (let j = 1; j <= k; j++) {
      const t = j / (k + 1);
      out.push([a[0] + (b[0] - a[0]) * t, a[1] + (b[1] - a[1]) * t]);
    }
  }
  if (!closed && pts.length) out.push(pts[pts.length - 1]!);
  return out;
}

/** The part of a closed polygon in front of the window (f ≥ nearestM): Sutherland–Hodgman against one edge. */
export function clipFront(ring: Local[], fMin = nearestM): Local[] {
  const out: Local[] = [];
  for (let i = 0; i < ring.length; i++) {
    const a = ring[i]!;
    const b = ring[(i + 1) % ring.length]!;
    const ina = a[0] >= fMin;
    const inb = b[0] >= fMin;
    if (ina) out.push(a);
    if (ina !== inb) {
      const t = (fMin - a[0]) / (b[0] - a[0]);
      out.push([fMin, a[1] + (b[1] - a[1]) * t]);
    }
  }
  return out;
}

const k = (v: number) => (v * 1000).toFixed(1);

/** An SVG path, in a 1000 × 1000 box stretched over the screen, of a ring of places at h metres; "" if unseen. */
export function ringPath(p: Projector, ring: LatLon[], h: number): string {
  const front = clipFront(ring.map((q) => p.local(q)));
  if (front.length < 3) return "";
  const pts = densify(front).map((q) => p.screen(q, h));
  return "M" + pts.map(([x, y]) => `${k(x)},${k(y)}`).join("L") + "Z";
}

/** An SVG path of a line of places at h metres, broken where it passes under the window. */
export function linePath(p: Projector, line: LatLon[], h: number): string {
  const pts = densify(
    line.map((q) => p.local(q)),
    40,
    false,
  );
  let d = "";
  let pen = false;
  for (const q of pts) {
    if (q[0] < nearestM) {
      pen = false;
      continue;
    }
    const [x, y] = p.screen(q, h);
    d += `${pen ? "L" : "M"}${k(x)},${k(y)}`;
    pen = true;
  }
  return d;
}

/** A runway's paved strip, widened by `extra` metres each side (the grass round it), as a ring of local points. */
export function runwayStrip(p: Projector, line: LatLon[], width: number, extra = 0): Local[] {
  const a = p.local(line[0]!);
  const b = p.local(line[line.length - 1]!);
  const len = Math.hypot(b[0] - a[0], b[1] - a[1]);
  const half = width / 2 + extra;
  const nx = (-(b[1] - a[1]) / len) * half;
  const ny = ((b[0] - a[0]) / len) * half;
  const ux = ((b[0] - a[0]) / len) * extra;
  const uy = ((b[1] - a[1]) / len) * extra;
  return [
    [a[0] + nx - ux, a[1] + ny - uy],
    [b[0] + nx + ux, b[1] + ny + uy],
    [b[0] - nx + ux, b[1] - ny + uy],
    [a[0] - nx - ux, a[1] - ny - uy],
  ];
}

/** A ring of local points as a path at h metres, or "". */
export function localPath(p: Projector, ring: Local[], h: number): string {
  const front = clipFront(ring);
  if (front.length < 3) return "";
  return (
    "M" +
    densify(front, 25)
      .map((q) => p.screen(q, h))
      .map(([x, y]) => `${k(x)},${k(y)}`)
      .join("L") +
    "Z"
  );
}

/** Points every `every` metres along both long edges of a runway, h metres up: its edge lights. */
export function runwayLights(
  p: Projector,
  line: LatLon[],
  width: number,
  h: number,
  every = 60,
): [number, number][] {
  const [a, b, c, d] = runwayStrip(p, line, width);
  const out: [number, number][] = [];
  for (const [s, e] of [
    [a, b],
    [d, c],
  ] as [Local, Local][]) {
    const len = Math.hypot(e[0] - s[0], e[1] - s[1]);
    for (let m = 0; m <= len; m += every) {
      const t = m / len;
      const q: Local = [s[0] + (e[0] - s[0]) * t, s[1] + (e[1] - s[1]) * t];
      if (q[0] >= nearestM) out.push(p.screen(q, h));
    }
  }
  return out;
}

/** The convex hull of screen points (monotone chain), for a building's outline seen from above. */
export function hull(points: [number, number][]): [number, number][] {
  const pts = [...points].sort((a, b) => a[0] - b[0] || a[1] - b[1]);
  if (pts.length < 3) return pts;
  const cross = (o: [number, number], a: [number, number], b: [number, number]) =>
    (a[0] - o[0]) * (b[1] - o[1]) - (a[1] - o[1]) * (b[0] - o[0]);
  const lower: [number, number][] = [];
  for (const q of pts) {
    while (lower.length >= 2 && cross(lower[lower.length - 2]!, lower[lower.length - 1]!, q) <= 0)
      lower.pop();
    lower.push(q);
  }
  const upper: [number, number][] = [];
  for (const q of [...pts].reverse()) {
    while (upper.length >= 2 && cross(upper[upper.length - 2]!, upper[upper.length - 1]!, q) <= 0)
      upper.pop();
    upper.push(q);
  }
  return [...lower.slice(0, -1), ...upper.slice(0, -1)];
}

export interface Wall {
  path: string;
  /** How directly the wall faces the sun, 0 (away) to 1 (square on), for its shade. */
  sunlit: number;
  /** A line along each floor, when the wall is big enough on screen to show them; points for lit windows. */
  floors: string;
  windows: [number, number][];
}

export interface Block {
  /** The walls that face the window, then the roof. */
  walls: Wall[];
  roof: string;
  /** How far away it is, for drawing the far ones first. */
  distance: number;
}

const floorM = 3.2;

/**
 * The harbour front's buildings as seen from the window, farthest first: each wall that faces the window, shaded by
 * how squarely it faces the sun (sunDeg: the sun's direction right of the window's centre), and its roof.
 */
export function blocks(p: Projector, h: Harbour, sunDeg = 0, seed = 23): Block[] {
  const rand = seeded(seed);
  const sun: Local = [Math.cos(sunDeg * rad), Math.sin(sunDeg * rad)];
  const out: Block[] = [];
  const path = (pts: [number, number][]) => "M" + pts.map(([x, y]) => `${k(x)},${k(y)}`).join("L") + "Z";
  for (const b of h.buildings) {
    const ring = b.ring.slice(0, -1).map((q) => p.local(q));
    if (ring.length < 3 || ring.some((q) => q[0] < nearestM)) continue;
    const base = h.ground_m;
    const roofH = base + b.h;
    const top = ring.map((q) => p.screen(q, roofH));
    if (top.every(([x]) => x < -0.1 || x > 1.1) || top.every(([, y]) => y > 1.05)) continue;
    const cf = ring.reduce((s, q) => s + q[0], 0) / ring.length;
    const cr = ring.reduce((s, q) => s + q[1], 0) / ring.length;
    const walls: Wall[] = [];
    for (let i = 0; i < ring.length; i++) {
      const a = ring[i]!;
      const c = ring[(i + 1) % ring.length]!;
      const len = Math.hypot(c[0] - a[0], c[1] - a[1]);
      if (len < 2) continue;
      let n: Local = [(c[1] - a[1]) / len, -(c[0] - a[0]) / len];
      const mid: Local = [(a[0] + c[0]) / 2, (a[1] + c[1]) / 2];
      if (n[0] * (mid[0] - cf) + n[1] * (mid[1] - cr) < 0) n = [-n[0], -n[1]]; // outward
      if (n[0] * -mid[0] + n[1] * -mid[1] <= 0) continue; // faces away from the window
      const corners = [p.screen(a, base), p.screen(c, base), p.screen(c, roofH), p.screen(a, roofH)];
      const tall = Math.abs(corners[0]![1] - corners[3]![1]);
      let floors = "";
      const windows: [number, number][] = [];
      if (tall > 0.012) {
        for (let fh = base + floorM; fh < roofH - 1; fh += floorM) {
          const [x1, y1] = p.screen(a, fh);
          const [x2, y2] = p.screen(c, fh);
          floors += `M${k(x1)},${k(y1)}L${k(x2)},${k(y2)}`;
          // At night, some rooms along this floor are lit.
          const lit = Math.round(len / 9);
          for (let j = 0; j < lit; j++) {
            if (rand() < 0.55) continue;
            const t = (j + 0.5) / lit;
            const [wx, wy] = p.screen([a[0] + (c[0] - a[0]) * t, a[1] + (c[1] - a[1]) * t], fh - floorM / 2);
            windows.push([wx, wy]);
          }
        }
      }
      walls.push({
        path: path(corners),
        sunlit: Math.max(0, n[0] * sun[0] + n[1] * sun[1]),
        floors,
        windows,
      });
    }
    const distance = Math.hypot(cf, cr);
    out.push({ walls, roof: path(top), distance });
  }
  return out.sort((a, b) => b.distance - a.distance);
}
