// The colour of the sky as the sun goes down and comes up: hand-picked colours at a few sun heights, blended in
// between in linear light, so the sky changes as smoothly as the real one instead of in four steps. The day sky is
// held well below the real one's brightness: a TV left on all day should not be a light box.

type RGB = [number, number, number];

export interface SkyColours {
  /** At the top of the screen, the middle, and down at the roofline. */
  top: string;
  mid: string;
  low: string;
  /** The rooftops: the darkest thing on screen, lifted a little by day so they still read as roofs. */
  roof: string;
}

// Sun heights in degrees, from deep night to full day.
const keys: { sun: number; top: string; mid: string; low: string; roof: string }[] = [
  { sun: -18, top: "#03060f", mid: "#070d20", low: "#101b36", roof: "#020409" },
  { sun: -12, top: "#040a1a", mid: "#0a1634", low: "#1a2a4f", roof: "#03060c" },
  { sun: -6, top: "#0b1534", mid: "#29305e", low: "#6a4f74", roof: "#04070e" },
  { sun: -2, top: "#132250", mid: "#4a4c7c", low: "#c27f6c", roof: "#060a12" },
  { sun: 2, top: "#1d3866", mid: "#64789e", low: "#dca47e", roof: "#0a1018" },
  { sun: 8, top: "#264a76", mid: "#6688a8", low: "#b9b1a0", roof: "#121a24" },
  { sun: 20, top: "#284b6e", mid: "#4b7193", low: "#7593a8", roof: "#18222d" },
];

const toLinear = (c: number) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4);
const toSrgb = (c: number) => (c <= 0.0031308 ? c * 12.92 : 1.055 * c ** (1 / 2.4) - 0.055);

function parse(hex: string): RGB {
  const n = parseInt(hex.slice(1), 16);
  return [((n >> 16) & 255) / 255, ((n >> 8) & 255) / 255, (n & 255) / 255];
}

function format(c: RGB): string {
  return (
    "#" +
    c
      .map((v) =>
        Math.round(Math.min(1, Math.max(0, v)) * 255)
          .toString(16)
          .padStart(2, "0"),
      )
      .join("")
  );
}

/** a and b blended t of the way, in linear light, as light really adds. */
export function mix(a: string, b: string, t: number): string {
  const x = parse(a).map(toLinear);
  const y = parse(b).map(toLinear);
  return format(x.map((v, i) => toSrgb(v + ((y[i] ?? 0) - v) * t)) as RGB);
}

/** WCAG relative luminance of a colour, 0 (black) to 1 (white). */
export function luminance(hex: string): number {
  const [r, g, b] = parse(hex).map(toLinear) as RGB;
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

/** The sky's colours with the sun at sunDeg above (or below) the horizon. */
export function skyColours(sunDeg: number): SkyColours {
  const first = keys[0]!;
  const last = keys[keys.length - 1]!;
  if (sunDeg <= first.sun) return { top: first.top, mid: first.mid, low: first.low, roof: first.roof };
  if (sunDeg >= last.sun) return { top: last.top, mid: last.mid, low: last.low, roof: last.roof };
  const i = keys.findIndex((k) => k.sun > sunDeg);
  const a = keys[i - 1]!;
  const b = keys[i]!;
  const t = (sunDeg - a.sun) / (b.sun - a.sun);
  return {
    top: mix(a.top, b.top, t),
    mid: mix(a.mid, b.mid, t),
    low: mix(a.low, b.low, t),
    roof: mix(a.roof, b.roof, t),
  };
}

export interface Glow {
  /** Centre on screen, as fractions of its width and height; may lie off screen when the sun is beside the window. */
  x: number;
  y: number;
  colour: string;
  opacity: number;
}

/**
 * The light around the sun: warm and strongest just after sunset and before sunrise, a faint pale haze by day,
 * nothing at night. offsetDeg is the sun's direction right of the window's centre; it fades as the sun moves
 * round behind the house. y is where the sun is on screen, or the roofline when it is lower than the roofs.
 */
export function sunGlow(sunDeg: number, offsetDeg: number, fovDeg: number, y: number): Glow | null {
  if (sunDeg < -10) return null;
  let opacity: number;
  if (sunDeg < -1) opacity = ((sunDeg + 10) / 9) * 0.5;
  else if (sunDeg < 8) opacity = 0.5 - ((sunDeg + 1) / 9) * 0.3;
  else opacity = Math.max(0.12, 0.2 - (sunDeg - 8) / 50);
  const beyond = Math.abs(offsetDeg) - fovDeg / 2; // degrees outside the window's edge
  if (beyond > 0) opacity *= Math.max(0, 1 - beyond / 70);
  if (opacity < 0.01) return null;
  const colour =
    sunDeg < 2 ? "#ff9a62" : sunDeg < 10 ? mix("#ffb47a", "#fff0d8", (sunDeg - 2) / 8) : "#fff0d8";
  return { x: 0.5 + offsetDeg / fovDeg, y, colour, opacity: Number(opacity.toFixed(3)) };
}
