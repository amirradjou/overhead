// The sky's own news, for the quiet line between planes: when the sun next rises or sets, and the moon.
import { getMoonTimes } from "suncalc";
import { direction } from "./format";
import { inFrame, offsetFrom } from "./sky";
import { moonAt, moonPhaseName, sunTimes } from "./sun";
import type { Window } from "./types";

/** "18:10" in a time zone utcOffsetMin minutes east of UTC. */
export function hhmm(t: Date, utcOffsetMin: number): string {
  const local = new Date(t.getTime() + utcOffsetMin * 60_000);
  return `${String(local.getUTCHours()).padStart(2, "0")}:${String(local.getUTCMinutes()).padStart(2, "0")}`;
}

/** "Sunset at 18:10.", or the sunrise, whichever comes next. */
export function sunLine(
  lat: number,
  lon: number,
  now: Date,
  utcOffsetMin = -now.getTimezoneOffset(),
): string {
  const today = sunTimes(lat, lon, now, utcOffsetMin);
  if (today.sunrise && now < today.sunrise) return `Sunrise at ${hhmm(today.sunrise, utcOffsetMin)}.`;
  if (today.sunset && now < today.sunset) return `Sunset at ${hhmm(today.sunset, utcOffsetMin)}.`;
  const tomorrow = sunTimes(lat, lon, new Date(now.getTime() + 86_400_000), utcOffsetMin);
  return tomorrow.sunrise ? `Sunrise tomorrow at ${hhmm(tomorrow.sunrise, utcOffsetMin)}.` : "";
}

/** "The moon in your window is a waning gibbous, 76% lit.", or where it is, or when it rises. */
export function moonLine(
  lat: number,
  lon: number,
  now: Date,
  w: Window,
  utcOffsetMin = -now.getTimezoneOffset(),
): string {
  const m = moonAt(lat, lon, now);
  const name = moonPhaseName(m.phase);
  if (name === "new moon") return "New moon: the moon is too close to the sun to see.";
  const phase = `${name.endsWith("quarter") ? "at" : "a"} ${name}, ${Math.round(m.fraction * 100)}% lit`;
  if (m.alt > 0 && inFrame(w, offsetFrom(w, m.az), m.alt)) return `The moon in your window is ${phase}.`;
  if (m.alt > 0) return `The moon is up in the ${direction(m.az)}, ${phase}.`;
  const rise = getMoonTimes(now, lat, lon, utcOffsetMin).rise;
  if (rise && rise > now) return `The moon rises at ${hhmm(rise, utcOffsetMin)}, ${phase}.`;
  return `The moon is below the horizon, ${phase}.`;
}
