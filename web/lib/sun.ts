// Where the sun is, so the sky on screen is the colour of the sky outside. NOAA's simplified solar position: good to
// a fraction of a degree, which is far finer than a sky gradient needs.

const rad = Math.PI / 180;

/** The sun's elevation above the horizon, in degrees, at a place and time. */
export function sunElevation(lat: number, lon: number, when: Date): number {
  // Whole days since 1 January (UTC); the time of day enters once, through `hours`.
  const day = Math.floor((when.getTime() - Date.UTC(when.getUTCFullYear(), 0, 1)) / 86_400_000);
  const hours = when.getUTCHours() + when.getUTCMinutes() / 60 + when.getUTCSeconds() / 3600;
  const g = ((2 * Math.PI) / 365) * (day + (hours - 12) / 24); // fractional year, radians

  const equationOfTime =
    229.18 *
    (0.000075 +
      0.001868 * Math.cos(g) -
      0.032077 * Math.sin(g) -
      0.014615 * Math.cos(2 * g) -
      0.040849 * Math.sin(2 * g));
  const declination =
    0.006918 -
    0.399912 * Math.cos(g) +
    0.070257 * Math.sin(g) -
    0.006758 * Math.cos(2 * g) +
    0.000907 * Math.sin(2 * g) -
    0.002697 * Math.cos(3 * g) +
    0.00148 * Math.sin(3 * g);

  const solarMinutes = hours * 60 + equationOfTime + 4 * lon;
  const hourAngle = (solarMinutes / 4 - 180) * rad;
  const cosZenith =
    Math.sin(lat * rad) * Math.sin(declination) +
    Math.cos(lat * rad) * Math.cos(declination) * Math.cos(hourAngle);
  return 90 - Math.acos(Math.min(1, Math.max(-1, cosZenith))) / rad;
}

export type SkyPhase = "night" | "twilight" | "golden" | "day";

/** The look of the sky: night below civil twilight, golden hour while the sun is low. */
export function skyPhase(sunDeg: number): SkyPhase {
  if (sunDeg < -6) return "night";
  if (sunDeg < 0) return "twilight";
  if (sunDeg < 8) return "golden";
  return "day";
}
