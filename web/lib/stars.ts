// The real stars over home: the 904 stars to magnitude 4.5 in the Yale Bright Star Catalogue (NASA HEASARC's copy;
// scripts/bright-stars.mjs makes the file), turned to the sky at home at a time.
import catalogue from "./bright-stars.json";

const rad = Math.PI / 180;

/** [right ascension°, declination° (J2000), V magnitude, B−V colour] */
type CatalogueStar = [number, number, number, number];

export interface SkyStar {
  alt: number;
  az: number;
  mag: number;
  bv: number;
}

/** Local sidereal time in degrees: which right ascension is due south at home now. */
export function siderealDeg(when: Date, lonDeg: number): number {
  const days = when.getTime() / 86_400_000 + 2440587.5 - 2451545.0; // since J2000.0
  const gmst = 280.46061837 + 360.98564736629 * days;
  return (((gmst + lonDeg) % 360) + 360) % 360;
}

/** Altitude and azimuth (clockwise from north), in degrees, of a star at a latitude and sidereal time. */
export function horizontal(raDeg: number, decDeg: number, latDeg: number, lstDeg: number) {
  const h = (lstDeg - raDeg) * rad;
  const d = decDeg * rad;
  const lat = latDeg * rad;
  const alt = Math.asin(Math.sin(lat) * Math.sin(d) + Math.cos(lat) * Math.cos(d) * Math.cos(h));
  const az = Math.atan2(
    -Math.cos(d) * Math.sin(h),
    Math.sin(d) * Math.cos(lat) - Math.cos(d) * Math.cos(h) * Math.sin(lat),
  );
  return { alt: alt / rad, az: (((az / rad) % 360) + 360) % 360 };
}

/**
 * The faintest stars worth drawing with the sun this far below the horizon: about magnitude 4.5 in a city night,
 * only the brightest few in the blue of twilight, none by day.
 */
export function magnitudeLimit(sunAltDeg: number): number {
  if (sunAltDeg <= -12) return 4.5;
  if (sunAltDeg <= -6) return 1.5 + ((-6 - sunAltDeg) / 6) * 3;
  if (sunAltDeg <= -3) return ((-3 - sunAltDeg) / 3) * 1.5;
  return -Infinity;
}

/** The stars above the horizon at home, no fainter than limitMag. */
export function starsAbove(lat: number, lon: number, when: Date, limitMag: number): SkyStar[] {
  const lst = siderealDeg(when, lon);
  const out: SkyStar[] = [];
  for (const [ra, dec, mag, bv] of catalogue as CatalogueStar[]) {
    if (mag > limitMag) break; // brightest first
    const { alt, az } = horizontal(ra, dec, lat, lst);
    if (alt > 0) out.push({ alt, az, mag, bv });
  }
  return out;
}

/** A star's tint from its B−V colour: hot stars a little blue, cool ones a little orange. */
export function starColour(bv: number): string {
  if (bv < 0) return "#d6e1ff";
  if (bv < 0.4) return "#f4f6ff";
  if (bv < 0.8) return "#fff6e6";
  if (bv < 1.3) return "#ffe6c4";
  return "#ffd5ab";
}
