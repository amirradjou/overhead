// Words and numbers for the screen. Plain language for a living room: km/h rather than knots, "south-west" rather
// than 225°; altitude stays in feet, the unit pilots and every flight app use.
import type { Airport, Plane } from "./types";

const thousands = new Intl.NumberFormat("en-CA");

/** "9,700 ft", "On the ground", or "" when unknown. */
export function altitude(p: Plane): string {
  if (p.on_ground) return "On the ground";
  if (p.alt_ft === undefined) return "";
  return `${thousands.format(Math.round(p.alt_ft / 25) * 25)} ft`;
}

/** Climbing, descending or level, from the vertical rate; level within ±300 ft/min. */
export function trend(p: Plane): "climbing" | "descending" | "level" | undefined {
  if (p.on_ground || p.vert_rate_fpm === undefined) return undefined;
  if (p.vert_rate_fpm > 300) return "climbing";
  if (p.vert_rate_fpm < -300) return "descending";
  return "level";
}

export const trendArrow = { climbing: "↑", descending: "↓", level: "" } as const;

/**
 * What the plane is and what it is doing, as one sentence: "Boeing 737 MAX 8, descending through 9,800 ft at
 * 520 km/h". Written out rather than set as separate pieces, so it reads right even where the pieces' spacing is
 * lost (old TV browsers ignore flex gaps: "MAX 8" and "9,800 ft" ran together as "89,800 ft").
 */
export function flightFacts(p: Plane): string {
  const type = aircraftType(p);
  if (p.on_ground) return type ? `${type}, on the ground` : "On the ground";
  const alt = altitude(p);
  const kmh = speed(p);
  const t = trend(p);
  let doing = "";
  if (alt && (t === "climbing" || t === "descending")) doing = `${t} through ${alt}${kmh && ` at ${kmh}`}`;
  else if (alt && t === "level") doing = `level at ${alt}${kmh && `, ${kmh}`}`;
  else if (alt) doing = `at ${alt}${kmh && `, ${kmh}`}`;
  else doing = kmh;
  if (!type) return doing.charAt(0).toUpperCase() + doing.slice(1);
  if (!doing) return type;
  return doing.startsWith("at ") ? `${type} ${doing}` : `${type}, ${doing}`;
}

/** Ground speed in km/h. */
export function speed(p: Plane): string {
  if (p.speed_kt === undefined || p.on_ground) return "";
  return `${thousands.format(Math.round((p.speed_kt * 1.852) / 5) * 5)} km/h`;
}

const compass = ["north", "north-east", "east", "south-east", "south", "south-west", "west", "north-west"];

/** The direction from home, as a word. */
export function direction(bearingDeg: number): string {
  return compass[Math.round((((bearingDeg % 360) + 360) % 360) / 45) % 8] ?? "north";
}

/** "3.2 km south-west", or "overhead" within half a kilometre. */
export function whereFromHome(p: Plane): string {
  if (p.distance_km < 0.5) return "overhead";
  const km = p.distance_km < 10 ? p.distance_km.toFixed(1) : Math.round(p.distance_km).toString();
  return `${km} km ${direction(p.bearing_deg)}`;
}

/**
 * When the plane will land, if it is heading for a nearby destination and coming down: "Lands in about 4 min",
 * "Landing now". Planes climbing away from their destination get nothing.
 */
export function landing(p: Plane): string {
  if (p.eta_min === undefined || p.on_ground) return "";
  const t = trend(p);
  if (t === "climbing") return "";
  if (t !== "descending" && (p.alt_ft ?? Infinity) > 10000) return "";
  const min = Math.round(p.eta_min);
  return min < 1 ? "Landing now" : `Lands in about ${min} min`;
}

/** The airline or owner: "Porter Airlines". */
export function operator(p: Plane): string {
  return p.info?.route?.airline || p.info?.owner || "";
}

/** "Embraer ERJ 190-400", falling back to the type code. */
export function aircraftType(p: Plane): string {
  const name = p.info?.type_name;
  if (!name) return p.type_code ?? "";
  const maker = p.info?.manufacturer?.replace(
    /\s+(S\.?A\.?S|S\.?A\.?|Inc\.?|Ltd\.?|Company|Corporation)$/i,
    "",
  );
  return maker && !name.toLowerCase().startsWith(maker.toLowerCase()) ? `${maker} ${name}` : name;
}

/** An airport as the screen names it: city, or the airport's name, and its IATA code. */
export function place(a: Airport): { code: string; name: string } {
  return { code: a.iata || a.icao || "", name: a.city || a.name || "" };
}

/** The callsign, or the registration, or the address: something to call it by. */
export function label(p: Plane): string {
  return p.callsign || p.registration || p.info?.registration || p.hex.toUpperCase();
}

/** "Photo by Frederick Tremblay, planespotters.net": the credit planespotters' terms ask for. */
export function photoCredit(p: Plane): string {
  const ph = p.info?.photo;
  return ph ? `Photo by ${ph.photographer}, planespotters.net` : "";
}

/**
 * The flight as people say it: "Porter Airlines 672" when the airline and flight number are known, else the
 * callsign or registration.
 */
export function flightName(p: Plane): string {
  const airline = p.info?.route?.airline;
  const number = p.callsign?.match(/^[A-Z]{3}(\d[0-9A-Z]*)$/)?.[1];
  return airline && number ? `${airline} ${number}` : label(p);
}

/** "From San Francisco to Toronto", or "" when the route is unknown. */
export function fromTo(p: Plane): string {
  const route = p.info?.route;
  if (!route) return "";
  const from = place(route.origin).name || place(route.origin).code;
  const to = place(route.destination).name || place(route.destination).code;
  return from && to ? `From ${from} to ${to}` : "";
}

const sides = {
  left: "from the left",
  right: "from the right",
  rooftops: "over the rooftops",
  above: "over the house",
  distance: "far off, straight ahead",
} as const;

/** "over the rooftops in about 2 min": where and when a plane comes into view. */
export function arrival(p: Plane): string {
  if (!p.coming) return "";
  const s = Math.round(p.coming.in_s);
  const when = s < 45 ? "any moment now" : `in about ${Math.round(s / 60)} min`;
  return `${sides[p.coming.side]} ${when}`;
}
