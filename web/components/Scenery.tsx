"use client";

import { memo, useMemo } from "react";
import {
  blocks,
  linePath,
  localPath,
  projector,
  ringPath,
  runwayLights,
  runwayStrip,
  type Harbour,
  type LatLon,
  type Projector,
} from "@/lib/scenery";
import { seeded } from "@/lib/sky";
import { mix } from "@/lib/skyColour";
import type { Window } from "@/lib/types";

// Colours of the view below the horizon at a few sun heights, blended in between like the sky's: the lake hazy at
// its horizon and deep blue close in, the islands' trees with their autumn colour, the airfield's grass and runway,
// the harbour front's concrete. Held below the photograph's brightness, as the sky is, for a TV that is on all day.
interface Palette {
  lakeFar: string;
  lakeNear: string;
  trees: string;
  autumn: string;
  grass: string;
  runway: string;
  apron: string;
  pier: string;
  wall: string;
  wallLit: string;
  roof: string;
  land: string;
}

const keys: { sun: number; c: Palette }[] = [
  {
    sun: -12,
    c: {
      lakeFar: "#141c2c",
      lakeNear: "#060a13",
      trees: "#06090b",
      autumn: "#0d0d0a",
      grass: "#0a0e0e",
      runway: "#1a1d22",
      apron: "#1d2026",
      pier: "#22252c",
      wall: "#0b0f16",
      wallLit: "#0e131b",
      roof: "#141922",
      land: "#080b10",
    },
  },
  {
    sun: -4,
    c: {
      lakeFar: "#4b4c69",
      lakeNear: "#141b2e",
      trees: "#11160f",
      autumn: "#2a2414",
      grass: "#1b2219",
      runway: "#3a3d45",
      apron: "#40434b",
      pier: "#4b4d55",
      wall: "#22262f",
      wallLit: "#2c303a",
      roof: "#30343d",
      land: "#191c22",
    },
  },
  {
    sun: 3,
    c: {
      lakeFar: "#a68f8f",
      lakeNear: "#253650",
      trees: "#242b17",
      autumn: "#5e4a22",
      grass: "#4a5235",
      runway: "#6e6e72",
      apron: "#7a797b",
      pier: "#8c8682",
      wall: "#5c5a5e",
      wallLit: "#9b8576",
      roof: "#837f7f",
      land: "#3a3a36",
    },
  },
  {
    sun: 12,
    c: {
      lakeFar: "#7f93a6",
      lakeNear: "#223a57",
      trees: "#2f3a22",
      autumn: "#7b6a33",
      grass: "#5f6c4c",
      runway: "#7c8288",
      apron: "#8b9096",
      pier: "#a5a7a5",
      wall: "#5d646d",
      wallLit: "#b3b6b8",
      roof: "#a3a8ad",
      land: "#47504a",
    },
  },
];

function palette(sunDeg: number): Palette {
  const first = keys[0]!;
  const last = keys[keys.length - 1]!;
  if (sunDeg <= first.sun) return first.c;
  if (sunDeg >= last.sun) return last.c;
  const i = keys.findIndex((k) => k.sun > sunDeg);
  const a = keys[i - 1]!;
  const b = keys[i]!;
  const t = (sunDeg - a.sun) / (b.sun - a.sun);
  const out = { ...a.c };
  for (const key of Object.keys(out) as (keyof Palette)[]) out[key] = mix(a.c[key], b.c[key], t);
  return out;
}

// The tall ship moored at the foot of Spadina, seen in every photo from the window: where it lies, which way.
const tallShip = { at: [43.63655, -79.39185] as LatLon, headingDeg: 82, lengthM: 44, mastM: 30 };

interface Props {
  scenery: Harbour;
  home: { lat: number; lon: number };
  w: Window;
  sunAlt: number;
  /** The sun or the moon low over the water, for its glitter: direction right of the window's centre, height. */
  glitter: { offsetDeg: number; alt: number; moon: boolean; strength: number } | null;
  /** 0 to 1: how hazy the horizon is. */
  haze: number;
  /** The sun's direction right of the window's centre, for the light and shade on the buildings. */
  sunOffset: number;
}

// Scenery is the view below the horizon. It is redrawn only when the window, the light or the weather changes,
// never as the planes move.
function Scenery({ scenery, home, w, sunAlt, glitter, haze, sunOffset }: Props) {
  // The light changes in steps too small to see; a whole degree of sun is the smallest step worth a redraw.
  const sunStep = Math.round(sunAlt);
  // Walls are shaded by the sun's direction, to the nearest 10°: finer steps would redraw for nothing.
  const sunDir = Math.round(sunOffset / 10) * 10;
  const shapes = useMemo(
    () => draw(scenery, projector(home, w, scenery.lake_m), sunDir),
    [scenery, home.lat, home.lon, w, sunDir],
  );
  const c = useMemo(() => palette(sunStep), [sunStep]);
  const night = sunStep < -5;
  const glints = useMemo(
    () => (glitter ? glitterPath(shapes.p, w, glitter) : null),
    [
      shapes,
      w,
      glitter?.offsetDeg && Math.round(glitter.offsetDeg),
      glitter && Math.round(glitter.alt),
      glitter?.moon,
    ],
  );
  const hy = shapes.p.horizonY * 1000;
  return (
    <svg className="scenery" viewBox="0 0 1000 1000" preserveAspectRatio="none" aria-hidden="true">
      <defs>
        <linearGradient id="lake" x1="0" y1={hy} x2="0" y2="1000" gradientUnits="userSpaceOnUse">
          <stop offset="0" stopColor={c.lakeFar} />
          <stop offset="0.12" stopColor={mix(c.lakeFar, c.lakeNear, 0.55)} />
          <stop offset="1" stopColor={c.lakeNear} />
        </linearGradient>
        <linearGradient id="haze" x1="0" y1={hy - 70} x2="0" y2={hy + 60} gradientUnits="userSpaceOnUse">
          <stop offset="0" stopColor={c.lakeFar} stopOpacity="0" />
          <stop offset="0.55" stopColor={mix(c.lakeFar, "#ffffff", night ? 0.05 : 0.3)} stopOpacity={haze} />
          <stop offset="1" stopColor={c.lakeFar} stopOpacity="0" />
        </linearGradient>
        <linearGradient id="scrim" x1="0" y1="640" x2="0" y2="900" gradientUnits="userSpaceOnUse">
          <stop offset="0" stopColor="#03060c" stopOpacity="0" />
          <stop offset="1" stopColor="#03060c" stopOpacity="0.84" />
        </linearGradient>
        <pattern id="autumn" width="23" height="11" patternUnits="userSpaceOnUse">
          {/* Yellow, rust and red crowns among the green, scattered so no row or column shows. */}
          <circle cx="3" cy="2" r="1.3" fill={c.autumn} />
          <circle cx="11" cy="7.5" r="1" fill={mix(c.autumn, "#9a3b1e", night ? 0 : 0.4)} />
          <circle cx="17.5" cy="3.5" r="1.4" fill={c.autumn} />
          <circle cx="7" cy="9" r="0.8" fill={mix(c.autumn, "#c08a2a", night ? 0 : 0.5)} />
          <circle cx="20.5" cy="9.5" r="0.9" fill={mix(c.autumn, "#9a3b1e", night ? 0 : 0.25)} />
        </pattern>
        <clipPath id="island-land">
          <path d={shapes.islandsGround} />
        </clipPath>
      </defs>
      <rect x="-20" y={hy} width="1040" height={1020 - hy} fill="url(#lake)" />
      {glints && (
        <path
          d={glints}
          fill={glitter!.moon ? "#e8eef6" : sunAlt < 8 ? "#ffd59a" : "#fff5e0"}
          opacity={glitter!.strength}
        />
      )}
      <path d={shapes.mainland} fill={c.land} />
      {/* The islands: their trees' tops, then the ground, in one colour, so the canopy stands a little proud. */}
      <path d={shapes.islandsCanopy} fill={c.trees} />
      <path d={shapes.islandsGround} fill={c.trees} />
      <path d={shapes.islandsCanopy} fill="url(#autumn)" opacity="0.4" />
      <g clipPath="url(#island-land)">
        <path d={shapes.airfield} fill={c.grass} />
      </g>
      <path d={shapes.aprons} fill={c.apron} />
      <path d={shapes.runways} fill={c.runway} />
      <path
        d={shapes.pierLines}
        fill="none"
        stroke={c.pier}
        strokeWidth="1.4"
        vectorEffect="non-scaling-stroke"
      />
      <path d={shapes.pierAreas} fill={c.pier} />
      {shapes.blocks.map((b, i) => (
        <g key={i}>
          {b.walls.map((wl, j) => (
            <g key={j}>
              <path d={wl.path} fill={night ? c.wall : mix(c.wall, c.wallLit, wl.sunlit)} />
              {wl.floors && (
                <path
                  d={wl.floors}
                  fill="none"
                  stroke={night ? "#000000" : mix(c.wall, "#0d1218", 0.55)}
                  strokeWidth="1"
                  opacity="0.5"
                  vectorEffect="non-scaling-stroke"
                />
              )}
            </g>
          ))}
          <path d={b.roof} fill={c.roof} />
        </g>
      ))}
      <path d={shapes.ship.hull} fill={night ? "#0b0d12" : "#2a2422"} />
      <path
        d={shapes.ship.rigging}
        fill="none"
        stroke={night ? "#1c2029" : "#3b332e"}
        strokeWidth="1.2"
        vectorEffect="non-scaling-stroke"
      />
      {night && (
        <g className="lights">
          {shapes.windows.map(([x, y], i) => (
            <rect
              key={`w${i}`}
              x={x * 1000}
              y={y * 1000}
              width="1.6"
              height="1.2"
              fill="#ffd486"
              opacity="0.6"
            />
          ))}
          {shapes.shoreLamps.map(([x, y], i) => {
            // Its reflection: a streak on the water below it, longer the closer the lamp (perspective).
            const below = Math.max(0, y - shapes.p.horizonY);
            return (
              <g key={`s${i}`}>
                <circle cx={x * 1000} cy={y * 1000} r="0.9" fill="#ffcf8a" />
                <rect
                  x={x * 1000 - 0.4}
                  y={y * 1000 + 1.5}
                  width="0.8"
                  height={(1 + below * 60).toFixed(1)}
                  fill="#ffcf8a"
                  opacity="0.13"
                />
              </g>
            );
          })}
          {shapes.runwayLights.map(([x, y], i) => (
            <circle key={`r${i}`} cx={x * 1000} cy={y * 1000} r="0.75" fill="#fff1c7" />
          ))}
        </g>
      )}
      <rect x="-20" y={hy - 70} width="1040" height="130" fill="url(#haze)" />
      <rect x="-20" y="640" width="1040" height="400" fill="url(#scrim)" />
    </svg>
  );
}

export default memo(Scenery);

interface Shapes {
  p: Projector;
  mainland: string;
  islandsGround: string;
  islandsCanopy: string;
  airfield: string;
  runways: string;
  aprons: string;
  pierLines: string;
  pierAreas: string;
  blocks: ReturnType<typeof blocks>;
  ship: { hull: string; rigging: string };
  windows: [number, number][];
  shoreLamps: [number, number][];
  runwayLights: [number, number][];
}

// draw projects the whole harbour once for a window: every path the scenery needs, and the lights' places.
function draw(h: Harbour, p: Projector, sunDeg: number): Shapes {
  const ground = h.ground_m;
  const join = (paths: string[]) => paths.filter(Boolean).join("");
  const blocksSeen = blocks(p, h, sunDeg);
  // Lit windows at night: along the floors of the walls that face the window.
  const windows = blocksSeen.flatMap((b) => b.walls.flatMap((wl) => wl.windows));
  // The shore's lamps: one every 70 m along the mainland's edge (the ring without its inland closing points).
  const shoreLamps: [number, number][] = [];
  const shore = h.mainland.slice(0, -3).map((q) => p.local(q));
  let carried = 0;
  for (let i = 1; i < shore.length; i++) {
    const a = shore[i - 1]!;
    const b = shore[i]!;
    const len = Math.hypot(b[0] - a[0], b[1] - a[1]);
    for (let m = 70 - carried; m < len; m += 70) {
      const t = m / len;
      const q: [number, number] = [a[0] + (b[0] - a[0]) * t, a[1] + (b[1] - a[1]) * t];
      if (q[0] > 150 && Math.abs(q[1]) < q[0] * 1.6) shoreLamps.push(p.screen(q, ground + 5));
    }
    carried = (carried + len) % 70;
  }
  return {
    p,
    mainland: ringPath(p, h.mainland, ground),
    islandsGround: join(h.islands.map((r) => ringPath(p, r, ground))),
    islandsCanopy: join(h.islands.map((r) => ringPath(p, r, ground + 16))),
    airfield: join(h.runways.map((r) => localPath(p, runwayStrip(p, r.line, r.width, 90), ground))),
    runways: join(h.runways.map((r) => localPath(p, runwayStrip(p, r.line, r.width), ground + 0.5))),
    aprons: join(h.aprons.map((r) => ringPath(p, r, ground + 0.3))),
    pierLines: join(h.piers.filter((q) => !q.area).map((q) => linePath(p, q.pts, ground + 1))),
    pierAreas: join(h.piers.filter((q) => q.area).map((q) => ringPath(p, q.pts, ground + 1))),
    blocks: blocksSeen,
    ship: ship(p, ground),
    windows,
    shoreLamps,
    runwayLights: h.runways
      .filter((r) => r.ref === "08/26")
      .flatMap((r) => runwayLights(p, r.line, r.width + 6, ground + 1)),
  };
}

// ship is the tall ship: a dark hull along the quay and three masts with their yards.
function ship(p: Projector, ground: number): { hull: string; rigging: string } {
  const c = p.local(tallShip.at);
  // Unit vectors north and east in the window's frame, from two points a little way off.
  const unit = (q: LatLon) => {
    const v = p.local(q);
    const len = Math.hypot(v[0] - c[0], v[1] - c[1]);
    return [(v[0] - c[0]) / len, (v[1] - c[1]) / len] as const;
  };
  const n = unit([tallShip.at[0] + 0.001, tallShip.at[1]]);
  const e = unit([tallShip.at[0], tallShip.at[1] + 0.001]);
  const hd = (tallShip.headingDeg * Math.PI) / 180;
  const along = [
    n[0] * Math.cos(hd) + e[0] * Math.sin(hd),
    n[1] * Math.cos(hd) + e[1] * Math.sin(hd),
  ] as const;
  const across = [along[1], -along[0]] as const;
  const at = (a: number, s: number): [number, number] => [
    c[0] + along[0] * a + across[0] * s,
    c[1] + along[1] * a + across[1] * s,
  ];
  const L = tallShip.lengthM / 2;
  const hull = localPath(
    p,
    [at(-L, -4), at(L * 0.8, -4), at(L * 1.25, 0), at(L * 0.8, 4), at(-L, 4)],
    ground + 2,
  );
  const k = (v: number) => (v * 1000).toFixed(1);
  let rigging = "";
  for (const [i, a] of [-0.55, 0, 0.55].entries()) {
    const base = at(a * L, 0);
    const [bx, by] = p.screen(base, ground + 3);
    const [tx, ty] = p.screen(base, ground + tallShip.mastM - i * 2);
    rigging += `M${k(bx)},${k(by)}L${k(tx)},${k(ty)}`;
    for (const yard of [0.45, 0.65, 0.85]) {
      const hgt = ground + 3 + (tallShip.mastM - 3) * yard;
      const [lx, ly] = p.screen(at(a * L, -6 + yard * 3), hgt);
      const [rx, ry] = p.screen(at(a * L, 6 - yard * 3), hgt);
      rigging += `M${k(lx)},${k(ly)}L${k(rx)},${k(ry)}`;
    }
  }
  return { hull, rigging };
}

// glitterPath is the sun's (or the moon's) path of light on the water: short dashes under it, from the horizon
// towards the window, wider and sparser as they come closer.
function glitterPath(p: Projector, w: Window, g: NonNullable<Props["glitter"]>): string {
  const x0 = 0.5 + g.offsetDeg / w.fov_deg;
  if (x0 < -0.1 || x0 > 1.1) return "";
  const rand = seeded(5);
  const spread = 0.008 + Math.max(0, 25 - g.alt) / 1400; // a low sun lays a wider path
  let d = "";
  for (let i = 0; i < 140; i++) {
    const t = rand() ** 1.9; // most glints far off, where the path is narrow and bright
    const y = p.horizonY + 0.002 + (0.96 - p.horizonY) * t;
    const half = spread + t * 0.06;
    // Glints cluster on the line under the light and thin out to either side.
    const x = x0 + (rand() + rand() + rand() - 1.5) * half * 1.4;
    const len = (0.002 + t * 0.022) * (0.3 + rand());
    const thick = 0.0008 + t * 0.0022;
    const [a, b, c2, c3] = [(x - len / 2) * 1000, (x + len / 2) * 1000, y * 1000, (y + thick) * 1000];
    d += `M${a.toFixed(1)},${c2.toFixed(1)}L${b.toFixed(1)},${c2.toFixed(1)}L${b.toFixed(1)},${c3.toFixed(1)}L${a.toFixed(1)},${c3.toFixed(1)}Z`;
  }
  return d;
}
