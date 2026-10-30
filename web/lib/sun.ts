// The sun and the moon over home, from suncalc (BSD-2-Clause; about 0.08° for the sun, a fraction of the moon's own
// width for the moon): where they are, the moon's phase, and when the sun rises and sets.
import { getMoonIllumination, getMoonPosition, getPosition, getTimes } from "suncalc";

/** The sun's elevation above the horizon, in degrees, at a place and time. */
export function sunElevation(lat: number, lon: number, when: Date): number {
  return getPosition(when, lat, lon).altitude;
}

/** Where the sun is: degrees above the horizon and clockwise from north. */
export function sunPosition(lat: number, lon: number, when: Date): { alt: number; az: number } {
  const p = getPosition(when, lat, lon);
  return { alt: p.altitude, az: p.azimuth };
}

export type SkyPhase = "night" | "twilight" | "golden" | "day";

/** The look of the sky: night below civil twilight, golden hour while the sun is low. */
export function skyPhase(sunDeg: number): SkyPhase {
  if (sunDeg < -6) return "night";
  if (sunDeg < 0) return "twilight";
  if (sunDeg < 8) return "golden";
  return "day";
}

export interface Moon {
  alt: number;
  az: number;
  /** Lit fraction of the disc, 0 (new) to 1 (full). */
  fraction: number;
  /** Through the month: 0 new, 0.25 first quarter, 0.5 full, 0.75 last quarter. */
  phase: number;
  /** Where the bright edge faces, degrees anticlockwise from straight up, as seen from home. */
  limbDeg: number;
}

/** The moon from home at a time. */
export function moonAt(lat: number, lon: number, when: Date): Moon {
  const pos = getMoonPosition(when, lat, lon);
  const ill = getMoonIllumination(when);
  return {
    alt: pos.altitude,
    az: pos.azimuth,
    fraction: ill.fraction,
    phase: ill.phase,
    limbDeg: ill.angle - pos.parallacticAngle,
  };
}

/** "waxing crescent", "full moon", …: the phase as people name it; the named quarters last about a day each. */
export function moonPhaseName(phase: number): string {
  const p = ((phase % 1) + 1) % 1;
  const near = (x: number) => Math.abs(p - x) < 0.034 || Math.abs(p - x - 1) < 0.034;
  if (near(0)) return "new moon";
  if (near(0.25)) return "first quarter";
  if (near(0.5)) return "full moon";
  if (near(0.75)) return "last quarter";
  if (p < 0.25) return "waxing crescent";
  if (p < 0.5) return "waxing gibbous";
  if (p < 0.75) return "waning gibbous";
  return "waning crescent";
}

/**
 * The lit part of a moon of radius r centred on 0,0, with its bright edge at the top (SVG's y points down): the
 * half-circle limb, back along the terminator, a half-ellipse whose width follows the phase.
 */
export function moonPath(fraction: number, r: number): string {
  const k = Math.min(1, Math.max(0, fraction));
  const e = Math.abs(r * (1 - 2 * k)); // the terminator's half-width across the disc
  const sweep = k > 0.5 ? 1 : 0; // a gibbous moon's terminator bulges away from the limb, a crescent's towards it
  return `M${-r} 0A${r} ${r} 0 0 1 ${r} 0A${r} ${e.toFixed(3)} 0 0 ${sweep} ${-r} 0Z`;
}

/**
 * Sunrise and sunset at home on the day that contains `when` in a time zone utcOffsetMin minutes east of UTC (the
 * browser's own by default: the TV is at home). Either is null where the sun does not rise or set that day.
 */
export function sunTimes(
  lat: number,
  lon: number,
  when: Date,
  utcOffsetMin = -when.getTimezoneOffset(),
): { sunrise: Date | null; sunset: Date | null } {
  const t = getTimes(when, lat, lon, 0, utcOffsetMin);
  return { sunrise: t.sunrise, sunset: t.sunset };
}
