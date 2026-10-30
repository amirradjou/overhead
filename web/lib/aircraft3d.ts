// Planes in three dimensions, as seen from the window. Each is a small model of its kind (a high-wing turboprop like
// the Dash 8, a narrowbody jet, a jet with engines at the tail, a widebody, a light aircraft, a helicopter), built to
// its type's length and span, turned to its heading, pitched to its climb or descent and banked into its turn, then
// drawn as it is seen along the line of sight from the window: from above when it is below the window, from beneath
// when it is above, nose-on when it comes straight at you. Faces are shaded by the real sun.
import { typeFacts } from "./aircraft";
import { mix } from "./skyColour";
import type { Plane } from "./types";

export type Kind = "turboprop" | "jet" | "rearjet" | "widebody" | "light" | "heli";

/** What a plane looks like: its kind, size, and the colours of its fuselage, belly and tail fin. */
export interface Look {
  kind: Kind;
  lengthM: number;
  spanM: number;
  highWing: boolean;
  fourEngines: boolean;
  body: string;
  belly: string;
  fin: string;
}

const kinds: [Kind, RegExp][] = [
  ["turboprop", /^(DH8[A-D]|AT[47][2-6]|DHC6|SF34|D328|F50|AT4[3-6])$/],
  [
    "heli",
    /^(R22|R44|R66|B06|B407|B412|B429|B505|EC[1-7]\d|AS[35]\d|A1[0-6]9|AW\d+|H[1-9]\d\d?|S76|H60|BK17|MD[56]\d\d?)$/,
  ],
  [
    "rearjet",
    /^(CRJ[1-9X]|E1[34]5|GLF\d|GLEX|GL[5-8]T|CL[36]\d|LJ\d\d|C25[A-C]|C5[0-6][0-9A-Z]?|C68A|C700|C750|FA[0-9]X|F2TH|F900|H25B|E55P|E50P|PC24|HDJT)$/,
  ],
  ["widebody", /^(A30[0-9B]|A310|A3[3-5][0-9]|A38[0-9]|B7[4678][0-9A-Z]|MD11|IL96)$/],
  ["light", /^(C1[0-9]\d|C2[0-9]\d|P28[A-Z]|PA\d\d|SR2[02]|DA[24]\d|BE\d\d|M20[A-Z]|C77R|DV20|TBM\d|PC12)$/],
];

// The usual size of each kind when the type's own is not known, metres: length and span.
const sizes: Record<Kind, [number, number]> = {
  turboprop: [32.8, 28.4],
  jet: [37.6, 35.8],
  rearjet: [26, 21],
  widebody: [63.7, 60.3],
  light: [8.3, 11],
  heli: [11.7, 10],
};

// Tail fins in the airlines' colours, by the operator's ICAO code: what you see first of a livery from far off.
const fins: Record<string, string> = {
  POE: "#173a63", // Porter's navy
  PTR: "#173a63",
  ACA: "#1d1d1f", // Air Canada's black tail
  JZA: "#1d1d1f",
  ROU: "#1d1d1f",
  WJA: "#0a9c97", // WestJet teal
  SWG: "#f37021", // Sunwing orange
  FLE: "#2e8b3c", // Flair green
  UAL: "#123462",
  AAL: "#9aa5ad",
  DAL: "#9b1b30",
  RPA: "#1f3a6b",
  SKW: "#32477a",
  JBU: "#13265e",
  BAW: "#c8102e",
  DLH: "#05164d",
  AFR: "#002157",
  KLM: "#00a1de",
  UAE: "#d71921",
  QTR: "#5c0632",
  TAP: "#c8102e",
  AMX: "#0b2343",
};

/** How the plane looks, from its type and operator. */
export function lookFor(p: Plane): Look {
  const code = p.type_code ?? p.info?.type_code ?? "";
  let kind: Kind | undefined = kinds.find(([, re]) => re.test(code))?.[0];
  if (!kind) {
    kind =
      p.category === "A7"
        ? "heli"
        : p.category === "A1"
          ? "light"
          : p.category === "A2"
            ? "rearjet"
            : p.category === "A5"
              ? "widebody"
              : "jet";
  }
  const facts = typeFacts[code];
  const [len, span] = sizes[kind];
  const lengthM = facts?.lengthM ?? len;
  const spanM = facts?.wingspanM ?? span * (lengthM / len);
  const op = p.info?.route?.airline_icao || p.callsign?.slice(0, 3) || "";
  const airline = /^[A-Z]{3}\d/.test(p.callsign ?? "");
  return {
    kind,
    lengthM,
    spanM,
    highWing: kind === "turboprop" || (kind === "light" && /^C[12]\d\d/.test(code)),
    fourEngines: /^(A38\d|A34\d|B74|IL96)/.test(code),
    body: kind === "heli" && !airline ? "#b33a2e" : op === "AAL" ? "#c7ccd1" : "#eef1f4",
    belly: kind === "heli" ? "#3a3f47" : "#aab2bb",
    fin: fins[op] ?? (airline ? "#3d5a80" : kind === "light" || kind === "heli" ? "#c63d2f" : "#7a8894"),
  };
}

/** The plane's attitude: heading (degrees clockwise from north), pitch (nose up) and roll (right wing down). */
export interface Attitude {
  headingDeg: number;
  pitchDeg: number;
  rollDeg: number;
}

const g = 9.81;

/**
 * The attitude from what the plane reports: its track, its climb or descent against its speed (plus the few degrees
 * of nose-up a wing flies at), and the bank a turn at this rate needs. turnDegPerS comes from successive tracks.
 */
export function attitude(p: Plane, turnDegPerS = 0): Attitude {
  const headingDeg = p.track_deg ?? 0;
  if (p.on_ground) return { headingDeg, pitchDeg: 0, rollDeg: 0 };
  const v = Math.max(30, (p.speed_kt ?? 150) * 0.5144);
  const climb = ((p.vert_rate_fpm ?? 0) * 0.3048) / 60;
  const pathDeg = (Math.atan2(climb, v) * 180) / Math.PI;
  const bank = (Math.atan((v * turnDegPerS * Math.PI) / 180 / g) * 180) / Math.PI;
  return { headingDeg, pitchDeg: pathDeg + 2.5, rollDeg: Math.max(-35, Math.min(35, bank)) };
}

type V3 = [number, number, number]; // body frame: x forward, y right, z up, metres

type Part = "body" | "belly" | "glass" | "wing" | "fin" | "tail" | "engine" | "prop" | "rotor";

interface Face {
  pts: V3[];
  part: Part;
}

/** Where the lights are on the airframe. */
interface Lights {
  port: V3;
  starboard: V3;
  tail: V3;
  beacon: V3;
  landing: V3;
}

// ring is a cross-section: n points round (x, 0, zc) with radius r, starting at the top.
function ring(x: number, r: number, zc: number, n = 8, yScale = 1): V3[] {
  return Array.from({ length: n }, (_, i) => {
    const a = (i / n) * 2 * Math.PI;
    return [x, Math.sin(a) * r * yScale, zc + Math.cos(a) * r] as V3;
  });
}

// tube is a body of revolution through sections (x, radius, centre height): its side faces, with the lower ones
// marked as belly and, if asked, the windscreen between two sections.
function tube(sections: [number, number, number][], n: number, faces: Face[], glassAt = -1, yScale = 1) {
  const rings = sections.map(([x, r, z]) => ring(x, r, z, n, yScale));
  for (let s = 0; s < rings.length - 1; s++) {
    const a = rings[s]!;
    const b = rings[s + 1]!;
    for (let i = 0; i < n; i++) {
      const j = (i + 1) % n;
      const up = Math.cos(((i + 0.5) / n) * 2 * Math.PI); // 1 at the top, -1 underneath
      const part: Part = s === glassAt && up > 0.3 ? "glass" : up < -0.35 ? "belly" : "body";
      faces.push({ pts: [a[i]!, a[j]!, b[j]!, b[i]!], part });
    }
  }
  faces.push({ pts: [...rings[0]!].reverse(), part: "body" });
  faces.push({ pts: rings[rings.length - 1]!, part: "body" });
}

/** The airframe as faces in the body frame, and where its lights are. Built once per kind and size. */
export function model(look: Look): { faces: Face[]; lights: Lights } {
  const L = look.lengthM;
  const b = look.spanM / 2;
  const faces: Face[] = [];
  if (look.kind === "heli") {
    const r = L * 0.11;
    tube(
      [
        [-0.1 * L, r * 0.5, 0.1 * r],
        [0.05 * L, r, 0],
        [0.22 * L, r * 0.95, -0.1 * r],
        [0.3 * L, r * 0.35, -0.3 * r],
      ],
      8,
      faces,
      2,
    );
    tube(
      [
        [-0.5 * L, r * 0.12, 0.45 * r],
        [-0.1 * L, r * 0.22, 0.3 * r],
      ],
      6,
      faces,
    );
    faces.push({
      pts: [
        [-0.5 * L, 0, 0.4 * r],
        [-0.44 * L, 0, 0.4 * r],
        [-0.47 * L, 0, 1.6 * r],
        [-0.52 * L, 0, 1.5 * r],
      ],
      part: "fin",
    });
    const rotor: V3[] = Array.from({ length: 16 }, (_, i) => {
      const a = (i / 16) * 2 * Math.PI;
      return [0.08 * L + Math.cos(a) * b, Math.sin(a) * b, r * 1.35] as V3;
    });
    faces.push({ pts: rotor, part: "rotor" });
    for (const side of [-1, 1]) {
      faces.push({
        pts: [
          [-0.12 * L, side * r * 0.9, -1.35 * r],
          [0.25 * L, side * r * 0.9, -1.35 * r],
          [0.25 * L, side * r * 0.9, -1.45 * r],
          [-0.12 * L, side * r * 0.9, -1.45 * r],
        ],
        part: "engine",
      });
    }
    return {
      faces,
      lights: {
        port: [0, -r, 0],
        starboard: [0, r, 0],
        tail: [-0.5 * L, 0, 0.5 * r],
        beacon: [0.05 * L, 0, r * 1.1],
        landing: [0.25 * L, 0, -r],
      },
    };
  }
  const r = L * (look.kind === "light" ? 0.065 : look.kind === "widebody" ? 0.047 : 0.054);
  tube(
    [
      [-0.5 * L, r * 0.14, 0.62 * r],
      [-0.4 * L, r * 0.5, 0.4 * r],
      [-0.24 * L, r, 0.05 * r],
      [0.3 * L, r, 0],
      [0.41 * L, r * 0.88, -0.05 * r],
      [0.46 * L, r * 0.6, -0.15 * r],
      [0.5 * L, r * 0.1, -0.25 * r],
    ],
    8,
    faces,
    4,
  );
  const jetty = look.kind === "jet" || look.kind === "widebody" || look.kind === "rearjet";
  // Wings: a flat panel each side, swept on jets, straight on props, with a little dihedral.
  const rootChord = L * (jetty ? 0.17 : look.kind === "light" ? 0.17 : 0.09);
  const tipChord = rootChord * (jetty ? 0.33 : 0.62);
  const sweep = jetty ? b * (look.kind === "widebody" ? 0.55 : 0.45) : b * 0.04;
  const rootLE = L * (look.kind === "rearjet" ? 0.0 : look.kind === "light" ? 0.12 : jetty ? 0.1 : 0.06);
  const wz = look.highWing ? r * 0.95 : -r * 0.6;
  const dihedral = b * (look.highWing ? 0.02 : 0.09);
  for (const s of [-1, 1]) {
    faces.push({
      pts: [
        [rootLE, s * r * 0.8, wz],
        [rootLE - sweep, s * b, wz + dihedral],
        [rootLE - sweep - tipChord, s * b, wz + dihedral],
        [rootLE - rootChord, s * r * 0.8, wz],
      ],
      part: "wing",
    });
  }
  // The fin, and the tailplane: on top of the fin (a T-tail) on turboprops and rear-engined jets.
  const finH = L * (look.kind === "light" ? 0.16 : 0.17);
  const finRoot = L * 0.19;
  faces.push({
    pts: [
      [-0.5 * L + finRoot * 0.95, 0, r * 0.45],
      [-0.5 * L + finRoot * 0.95 - finRoot * 0.55, 0, r * 0.45 + finH],
      [-0.5 * L - 0.01 * L, 0, r * 0.45 + finH],
      [-0.5 * L + 0.01 * L, 0, r * 0.5],
    ],
    part: "fin",
  });
  const tTail = look.kind === "turboprop" || look.kind === "rearjet";
  const ht = look.spanM * (tTail ? 0.15 : 0.18);
  const htChord = L * 0.09;
  const htX = tTail ? -0.5 * L + 0.02 * L : -0.5 * L + 0.14 * L;
  const htZ = tTail ? r * 0.45 + finH : r * 0.55;
  for (const s of [-1, 1]) {
    faces.push({
      pts: [
        [htX + htChord, s * r * 0.2, htZ],
        [htX + htChord - ht * 0.5, s * ht, htZ],
        [htX - ht * 0.5 + htChord * 0.35, s * ht, htZ],
        [htX, s * r * 0.2, htZ],
      ],
      part: "tail",
    });
  }
  // Engines.
  const nacelle = (x: number, y: number, z: number, len: number, rad: number) =>
    tube(
      [
        [x - len / 2, rad * 0.8, z],
        [x + len / 2, rad, z],
      ],
      6,
      faces,
    );
  if (look.kind === "rearjet") {
    for (const s of [-1, 1]) nacelle(-0.3 * L, s * r * 1.5, r * 0.35, 0.15 * L, r * 0.42);
  } else if (look.kind === "turboprop" || look.kind === "light") {
    const props = look.kind === "light" ? [0] : [-0.3 * b, 0.3 * b];
    for (const y of props) {
      const x = look.kind === "light" ? 0.5 * L : rootLE + 0.04 * L;
      if (look.kind !== "light") nacelle(rootLE - 0.05 * L, y, wz - r * 0.35, 0.22 * L, r * 0.38);
      const pr = look.kind === "light" ? look.spanM * 0.08 : look.spanM * 0.075;
      faces.push({
        pts: Array.from({ length: 12 }, (_, i) => {
          const a = (i / 12) * 2 * Math.PI;
          return [x + 0.01 * L, y + Math.cos(a) * pr, wz - r * 0.3 + Math.sin(a) * pr] as V3;
        }),
        part: "prop",
      });
    }
  } else {
    const at = look.fourEngines ? [0.3, 0.62] : [0.33];
    for (const f of at) {
      for (const s of [-1, 1]) {
        const x = rootLE - sweep * f + 0.04 * L;
        nacelle(x, s * b * f, wz - r * (look.kind === "widebody" ? 0.75 : 0.7), 0.12 * L, r * 0.52);
      }
    }
  }
  return {
    faces,
    lights: {
      port: [rootLE - sweep - tipChord * 0.5, -b, wz + dihedral],
      starboard: [rootLE - sweep - tipChord * 0.5, b, wz + dihedral],
      tail: [-0.5 * L, 0, 0.6 * r],
      beacon: [0.05 * L, 0, r * 1.02],
      landing: [0.38 * L, 0, -0.7 * r],
    },
  };
}

type E3 = [number, number, number]; // east, north, up

const rad = Math.PI / 180;

/** Body frame to east-north-up: roll about the nose, pitch about the wing, then the heading. */
export function toENU(v: V3, a: Attitude): E3 {
  const [cr, sr] = [Math.cos(a.rollDeg * rad), Math.sin(a.rollDeg * rad)];
  const [cp, sp] = [Math.cos(a.pitchDeg * rad), Math.sin(a.pitchDeg * rad)];
  const [ch, sh] = [Math.cos(a.headingDeg * rad), Math.sin(a.headingDeg * rad)];
  const [x0, y0, z0] = v;
  const y1 = y0 * cr + z0 * sr; // right wing down for a right bank
  const z1 = -y0 * sr + z0 * cr;
  const x2 = x0 * cp - z1 * sp; // nose up
  const z2 = x0 * sp + z1 * cp;
  return [x2 * sh + y1 * ch, x2 * ch - y1 * sh, z2];
}

/** How the window sees the plane: the line of sight's direction (bearing, elevation). */
export interface Sight {
  bearingDeg: number;
  elevationDeg: number;
}

/** The screen's right and up, and the line of sight, for looking along `s`: east-north-up unit vectors. */
export function viewBasis(s: Sight): { right: E3; up: E3; along: E3 } {
  const [ce, se] = [Math.cos(s.elevationDeg * rad), Math.sin(s.elevationDeg * rad)];
  const [cb, sb] = [Math.cos(s.bearingDeg * rad), Math.sin(s.bearingDeg * rad)];
  const along: E3 = [ce * sb, ce * cb, se];
  const right: E3 = [cb, -sb, 0]; // along × up, normalised: horizontal, to the right of the line of sight
  const up: E3 = [-se * sb, -se * cb, ce]; // right × along
  return { right, up, along };
}

const dot = (a: E3, b: E3) => a[0] * b[0] + a[1] * b[1] + a[2] * b[2];

export interface Drawn {
  /** Faces as SVG polygons, in pixels from the plane's centre, farthest first. */
  faces: { points: string; fill: string; opacity?: number }[];
  /** The lights' places in pixels from the centre, and how squarely the landing lights point at the window. */
  lights: {
    port: [number, number];
    starboard: [number, number];
    tail: [number, number];
    beacon: [number, number];
    landing: [number, number];
  };
  glare: number;
}

/** The sun's direction, east-north-up, from its height and bearing; null below the horizon. */
export function sunVector(sun: { alt: number; az: number } | null): E3 | null {
  if (!sun || sun.alt < -2) return null;
  const [ca, sa] = [Math.cos(sun.alt * rad), Math.sin(sun.alt * rad)];
  return [ca * Math.sin(sun.az * rad), ca * Math.cos(sun.az * rad), sa];
}

/**
 * The plane drawn as the window sees it, lengthPx long if seen broadside: each face projected along the line of
 * sight, sorted far to near, and shaded by the sun (or, at night, left nearly black for its lights to show).
 */
export function draw(
  look: Look,
  att: Attitude,
  sight: Sight,
  sun: E3 | null,
  lengthPx: number,
  night: boolean,
): Drawn {
  const m = model(look);
  const { right, up, along } = viewBasis(sight);
  const k = lengthPx / look.lengthM;
  const project = (v: V3): [number, number, number] => {
    const e = toENU(v, att);
    return [dot(e, right) * k, -dot(e, up) * k, dot(e, along)];
  };
  const shaded = m.faces.map((f) => {
    const pts = f.pts.map(project);
    const e = f.pts.map((v) => toENU(v, att));
    // The face's normal, turned to face the window: both sides of a wing are painted alike.
    const u: E3 = [e[1]![0] - e[0]![0], e[1]![1] - e[0]![1], e[1]![2] - e[0]![2]];
    const w: E3 = [e[2]![0] - e[0]![0], e[2]![1] - e[0]![1], e[2]![2] - e[0]![2]];
    let n: E3 = [u[1] * w[2] - u[2] * w[1], u[2] * w[0] - u[0] * w[2], u[0] * w[1] - u[1] * w[0]];
    const len = Math.hypot(...n) || 1;
    n = [n[0] / len, n[1] / len, n[2] / len];
    if (dot(n, along) > 0) n = [-n[0], -n[1], -n[2]];
    const base =
      f.part === "fin"
        ? look.fin
        : f.part === "belly" || f.part === "engine"
          ? look.belly
          : f.part === "glass"
            ? "#1d2833"
            : f.part === "wing" || f.part === "tail"
              ? mix(look.body, look.belly, 0.7)
              : look.body;
    let light = 0.42 + 0.22 * Math.max(0, n[2]); // the sky's light, from above
    if (sun) light += 0.5 * Math.max(0, dot(n, sun));
    if (night) light = 0.07 + 0.05 * Math.max(0, -n[2]); // only the city's glow, from below
    const fill = mix("#0b1018", base, Math.min(1, light));
    const depth = pts.reduce((s, p) => s + p[2], 0) / pts.length;
    const points = pts.map(([x, y]) => `${x.toFixed(1)},${y.toFixed(1)}`).join(" ");
    const opacity = f.part === "prop" ? 0.18 : f.part === "rotor" ? 0.16 : undefined;
    return { points, fill, depth, opacity };
  });
  shaded.sort((a, b) => b.depth - a.depth);
  const at = (v: V3): [number, number] => {
    const [x, y] = project(v);
    return [Number(x.toFixed(1)), Number(y.toFixed(1))];
  };
  // Landing lights point along the nose: brightest when the plane comes straight at the window.
  const nose = toENU([1, 0, 0], att);
  return {
    faces: shaded.map(({ points, fill, opacity }) =>
      opacity ? { points, fill, opacity } : { points, fill },
    ),
    lights: {
      port: at(m.lights.port),
      starboard: at(m.lights.starboard),
      tail: at(m.lights.tail),
      beacon: at(m.lights.beacon),
      landing: at(m.lights.landing),
    },
    glare: Math.max(0, -dot(nose, along)) ** 3,
  };
}
