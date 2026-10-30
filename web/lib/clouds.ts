// The clouds outside, from the weather at home: each layer (low, mid, high) is drawn in perspective from the window,
// large overhead and shrinking into a dense band at the horizon, as a photograph out of the window shows them. Their
// colour follows the sun: white by day, warm at golden hour, pink from below at dusk, and at night a dull grey lit
// from beneath by the city. Overcast is a deck over the whole sky.
import { mix } from "./skyColour";
import { seeded } from "./sky";
import type { Weather } from "./types";

/** A soft round puff, or a long thin streak, in fractions of the screen (rx of its width, ry of its height). */
export interface Puff {
  cx: number;
  cy: number;
  rx: number;
  ry: number;
  /** The flat grey underside of a cumulus rather than its lit top. */
  base?: boolean;
  /** A puff low in the heap, in its own shade. */
  low?: boolean;
  /** A streak's lean, in degrees. */
  tilt?: number;
}

export interface Cloudscape {
  puffs: Puff[];
  streaks: Puff[];
  /** 0 to 1: how much of the sky an overcast deck covers. */
  deck: number;
  lit: string;
  shade: string;
  /** How strongly the clouds show: dimmer at night, when only the city lights them. */
  opacity: number;
}

/** Where a direction (degrees right of the window's centre) and angle above the horizon fall on screen. */
export type ToScreen = (offsetDeg: number, elevationDeg: number) => [number, number];

const rad = Math.PI / 180;

/** The clouds' colours with the sun at sunDeg: the lit top and the shaded underside. */
export function cloudColours(sunDeg: number): { lit: string; shade: string; opacity: number } {
  const keys = [
    { sun: -14, lit: "#3d3940", shade: "#1a1d26", opacity: 0.55 }, // night: the city's glow from below
    { sun: -6, lit: "#8a6f80", shade: "#2f2f45", opacity: 0.7 },
    { sun: -1, lit: "#f1a98c", shade: "#5c5470", opacity: 0.85 }, // dusk: lit pink from below the horizon
    { sun: 4, lit: "#ffd8bd", shade: "#a08088", opacity: 0.9 },
    { sun: 12, lit: "#dfe4e9", shade: "#9aa4b0", opacity: 0.92 }, // day, held below white like the sky
  ];
  const first = keys[0]!;
  const last = keys[keys.length - 1]!;
  if (sunDeg <= first.sun) return first;
  if (sunDeg >= last.sun) return last;
  const i = keys.findIndex((c) => c.sun > sunDeg);
  const a = keys[i - 1]!;
  const b = keys[i]!;
  const t = (sunDeg - a.sun) / (b.sun - a.sun);
  return {
    lit: mix(a.lit, b.lit, t),
    shade: mix(a.shade, b.shade, t),
    opacity: a.opacity + (b.opacity - a.opacity) * t,
  };
}

/**
 * The cloudscape for the weather, seen from a window eyeM above sea level that spans fovDeg, up to topDeg above the
 * horizon. The same weather and seed always give the same clouds.
 */
export function cloudscape(
  weather: Weather,
  sunDeg: number,
  fovDeg: number,
  topDeg: number,
  eyeM: number,
  at: ToScreen,
  seed = 11,
): Cloudscape {
  const rand = seeded(seed);
  const colours = cloudColours(sunDeg);
  const puffs: Puff[] = [];
  const streaks: Puff[] = [];
  const half = fovDeg / 2 + 8;
  const deck = Math.min(1, Math.max(0, (Math.max(weather.cloud_low, weather.cloud_mid) - 80) / 15));

  // A cumulus of size metres, d metres away and heightM up, at offset degrees: a flat shaded base under a heap of
  // overlapping puffs, the lower ones in shade. Seen low, near the horizon, a cloud is seen edge-on: flatter.
  const cumulus = (offset: number, d: number, heightM: number, size: number) => {
    const el = Math.atan((heightM - eyeM) / d) / rad;
    if (el > topDeg + 5 || el < 0.15) return;
    const wDeg = ((size / d) * 180) / Math.PI;
    const flat = Math.min(1, 0.32 + el / 18);
    const [bx, by] = at(offset, el);
    const [ex] = at(offset + wDeg * 0.58, el);
    const [, ey] = at(offset, el + wDeg * 0.07 * flat + 0.03);
    puffs.push({ cx: bx, cy: by, rx: Math.abs(ex - bx), ry: Math.abs(by - ey), base: true });
    const n = 5 + Math.floor(rand() * 5);
    for (let i = 0; i < n; i++) {
      const u = (rand() - 0.5) * 0.95; // across the cloud
      const r = wDeg * (0.13 + rand() * 0.15) * (1 - Math.abs(u) * 0.9);
      const heap = (1 - Math.abs(u) * 1.6) * r * 0.55; // the middle piles higher
      const lift = (r * (0.25 + rand() * 0.5) + Math.max(0, heap)) * flat;
      const [cx, cy] = at(offset + u * wDeg, el + lift);
      const [rx0] = at(offset + u * wDeg + r * 1.1, el + lift);
      const [, ry0] = at(offset + u * wDeg, el + lift + r * flat);
      puffs.push({ cx, cy, rx: Math.abs(rx0 - cx), ry: Math.abs(cy - ry0), low: lift < r * 0.5 * flat });
    }
  };

  if (deck < 1) {
    // Low cumulus within 14 km, one cell per square of side `cell` metres, filled as often as the cover says.
    const low = weather.cloud_low / 100;
    if (low > 0.03) {
      const cell = 1700;
      for (let f = 600; f < 14000; f += cell) {
        for (let r = -f * Math.tan(half * rad); r < f * Math.tan(half * rad); r += cell) {
          if (rand() > low * 0.9) continue;
          const x = f + (rand() - 0.5) * cell;
          const y = r + (rand() - 0.5) * cell;
          cumulus(Math.atan2(y, x) / rad, Math.hypot(x, y), 1500 + rand() * 500, 700 + rand() * 1300);
        }
      }
      // Beyond, a band of small far clouds down to the horizon, thicker the more cover there is.
      const far = Math.round(26 + 70 * low);
      for (let i = 0; i < far; i++) {
        const d = 14000 + rand() * rand() * 60000;
        cumulus((rand() - 0.5) * 2 * half, d, 1500 + rand() * 500, 900 + rand() * 1600);
      }
    }
    // Mid-level altocumulus: smaller, fainter, more of them, in patches.
    const mid = weather.cloud_mid / 100;
    if (mid > 0.05) {
      const patches = Math.round(2 + mid * 6);
      for (let p = 0; p < patches; p++) {
        const po = (rand() - 0.5) * 2 * half;
        const pd = 5000 + rand() * 30000;
        for (let i = 0; i < 10; i++) {
          cumulus(po + (rand() - 0.5) * 12, pd * (0.85 + rand() * 0.3), 4000, 300 + rand() * 500);
        }
      }
    }
  }
  // High cirrus: a few long feathery streaks, leaning the same way, faint.
  const high = weather.cloud_high / 100;
  if (high > 0.2) {
    const n = Math.round(2 + high * 6);
    const lean = (rand() - 0.5) * 10;
    for (let i = 0; i < n; i++) {
      const off = (rand() - 0.5) * 2 * half;
      const el = 14 + rand() * Math.max(4, topDeg - 14);
      const [cx, cy] = at(off, el);
      const [ex] = at(off + 6 + rand() * 14, el);
      streaks.push({
        cx,
        cy,
        rx: Math.abs(ex - cx),
        ry: 0.003 + rand() * 0.005,
        tilt: lean + (rand() - 0.5) * 4,
      });
    }
  }
  return { puffs, streaks, deck, ...colours };
}
