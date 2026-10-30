// The weather in words: what the sky is doing, and the wind, which decides which way planes land.
import type { FollowedAirport, Weather } from "./types";

/** The WMO weather code as a few words: "overcast", "light rain". */
export function sky(code: number): string {
  if (code === 0) return "clear";
  if (code === 1) return "mostly clear";
  if (code === 2) return "partly cloudy";
  if (code === 3) return "overcast";
  if (code === 45 || code === 48) return "foggy";
  if (code >= 51 && code <= 57) return "drizzling";
  if (code === 61 || code === 80) return "light rain";
  if (code === 63 || code === 81) return "raining";
  if (code === 65 || code === 82) return "heavy rain";
  if (code === 66 || code === 67) return "freezing rain";
  if (code === 71 || code === 85) return "light snow";
  if (code === 73 || code === 75 || code === 77 || code === 86) return "snowing";
  if (code >= 95) return "thunderstorms";
  return "";
}

const points = [
  "north",
  "north-north-east",
  "north-east",
  "east-north-east",
  "east",
  "east-south-east",
  "south-east",
  "south-south-east",
  "south",
  "south-south-west",
  "south-west",
  "west-south-west",
  "west",
  "west-north-west",
  "north-west",
  "north-north-west",
];

/** The cloud cover in words, finer than the WMO code's four steps: a broken deck is "mostly cloudy". */
export function cover(pct: number): string {
  if (pct < 10) return "clear";
  if (pct < 30) return "mostly clear";
  if (pct < 60) return "partly cloudy";
  if (pct < 90) return "mostly cloudy";
  return "overcast";
}

/** "the west-north-west": a bearing as one of 16 compass points. */
export function compass16(deg: number): string {
  return points[Math.round((((deg % 360) + 360) % 360) / 22.5) % 16] ?? "north";
}

/** The true heading flown along a runway, from its name: runway 26 is about 260° magnetic, 250° true here. */
function runwayHeading(name: string, declinationDeg = -10): number {
  return (((Number.parseInt(name, 10) * 10 + declinationDeg) % 360) + 360) % 360;
}

/**
 * The weather as the screen says it between planes: "14°C and overcast. Wind from the west-north-west at 18 km/h,
 * gusting 29, so planes land and take off on runway 26, into it." The runway is linked to the wind only when the
 * runway in use points within 60° of it.
 */
export function weatherLine(w: Weather, ap?: FollowedAirport): string {
  const parts: string[] = [];
  const s = w.code >= 1 && w.code <= 3 ? cover(w.cloud_cover) : sky(w.code);
  parts.push(`${Math.round(w.temp_c)}°C${s ? ` and ${s}` : ""}.`);
  if (w.wind_kmh < 5) {
    parts.push("Almost no wind.");
  } else {
    const gust = w.gust_kmh >= w.wind_kmh + 10 ? `, gusting ${Math.round(w.gust_kmh)}` : "";
    let into = "";
    const rw = ap?.runway_in_use;
    if (rw) {
      const diff = Math.abs(((runwayHeading(rw) - w.wind_from_deg + 540) % 360) - 180);
      if (diff <= 60) into = `, so planes land and take off on runway ${rw}, into it`;
    }
    parts.push(
      `Wind from the ${compass16(w.wind_from_deg)} at ${Math.round(w.wind_kmh)} km/h${gust}${into}.`,
    );
  }
  if (w.visibility_m < 8000)
    parts.push(`You can see about ${Math.max(1, Math.round(w.visibility_m / 1000))} km.`);
  return parts.join(" ");
}

/** A line about low cloud, when there is enough to swallow a climbing plane: "" otherwise. */
export function cloudLine(w: Weather): string {
  if (w.cloud_low >= 85) return "Low cloud: a plane climbing out disappears into it within a minute or two.";
  return "";
}
