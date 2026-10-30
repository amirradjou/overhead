"use client";

import { useRef } from "react";
import { airportStatus, endName, feet, heightFromWindow, lookShort } from "@/lib/airport";
import { aircraftType, flightName, operator, photoCredit, speed, webLink } from "@/lib/format";
import { flightProgress, km } from "@/lib/progress";
import { profile, type ProfilePoint } from "@/lib/profile";
import type { FollowedAirport, Plane, Window } from "@/lib/types";

const clock = new Intl.DateTimeFormat("en-CA", { hour: "2-digit", minute: "2-digit", hourCycle: "h23" });
const thousands = new Intl.NumberFormat("en-CA");

interface Props {
  plane: Plane;
  airport?: FollowedAirport;
  w: Window;
  now: Date | null;
}

// FlightCard is the plane in view, told in full and laid out to be read from the sofa: what it is doing now, who
// flies it and on what, its trip from airport to airport with the time it lands, and on the right its instruments:
// for a flight at the followed airport, the side view of its approach or climb against the window's height, then
// its height, speed, climb and distance. Old TV browsers ignore flex gaps, so margins space everything.
export default function FlightCard({ plane, airport, w, now }: Props) {
  const photo = plane.info?.photo;
  const route = plane.info?.route;
  const reg = plane.info?.registration || plane.registration;
  const title = route?.airline ? flightName(plane) : reg || flightName(plane);
  const who = route?.airline || plane.info?.owner || "";
  const sub = [aircraftType(plane), reg !== title ? reg : "", who && who !== title.split(" ")[0] ? who : ""]
    .filter(Boolean)
    .join(" · ");
  const status = plane.notable || airportStatus(plane, airport);
  const progress = plane.on_ground ? null : flightProgress(plane);
  const landsIn = plane.airport?.op === "arriving" ? plane.airport.in_min : progress?.minutesLeft;
  const lands = landsIn !== undefined && now ? clock.format(new Date(now.getTime() + landsIn * 60_000)) : "";

  // The flight's side view keeps the points it has passed through while it is on screen.
  const trail = useRef<ProfilePoint[]>([]);
  const at = plane.airport;
  if (at) {
    const pt = { km: at.distance_km, m: (at.height_ft ?? 0) * 0.3048 };
    const last = trail.current[trail.current.length - 1];
    if (!last || Math.abs(last.km - pt.km) > 0.05 || Math.abs(last.m - pt.m) > 4) {
      trail.current.push(pt);
      if (trail.current.length > 45) trail.current.shift();
    }
  }
  const side =
    at && airport
      ? profile(at, trail.current.slice(0, -1), w.home_elevation_m - airport.elevation_ft * 0.3048)
      : null;

  return (
    <div className="card">
      {photo && (
        // Linked to the photo's page on planespotters.net, as their terms ask.
        <a className="photo card-photo" href={webLink(photo.link)} rel="noopener">
          <figure>
            <img src={photo.large || photo.thumbnail} alt="" />
            <figcaption>{photoCredit(plane)}</figcaption>
          </figure>
        </a>
      )}
      <div className="card-main">
        {status && (
          <p className="status">
            <span className="status-dot" />
            {status}
          </p>
        )}
        <h1 className="flight">{title}</h1>
        {sub && <p className="card-sub">{sub}</p>}
        {route && progress ? (
          <>
            <div className="trip-row">
              <p className="trip-end">
                <span className="trip-code">{progress.fromCode}</span>
                <span className="trip-city">{endName(route.origin, airport)}</span>
              </p>
              <div className="trip-line">
                <span className="trip-line-flown" style={{ width: `${(progress.done * 100).toFixed(1)}%` }} />
                <svg
                  className="trip-plane"
                  viewBox="0 0 24 24"
                  style={{ left: `${(progress.done * 100).toFixed(1)}%` }}
                >
                  <path d="M22.5 12c0 .7-.6 1.1-1.4 1.1h-6l-4.8 8.4H8.2l2.6-8.4H5.4l-2 2.6H1.5L2.5 12l-1-3.7h1.9l2 2.6h5.4L8.2 2.5h2.1l4.8 8.4h6c.8 0 1.4.4 1.4 1.1z" />
                </svg>
              </div>
              <p className="trip-end is-to">
                <span className="trip-code">{progress.toCode}</span>
                <span className="trip-city">{endName(route.destination, airport)}</span>
              </p>
            </div>
            <p className="trip-note">
              {km(progress.flownKm)} flown · {km(progress.toGoKm)} to go
              {lands && ` · lands about ${lands}`}
            </p>
          </>
        ) : (
          <p className="route">{operator(plane) || withoutRoute(plane, airport)}</p>
        )}
        <p className="trip-note">
          {plane.distance_km < 10 ? plane.distance_km.toFixed(1) : Math.round(plane.distance_km)} km away ·
          look {lookShort(plane)}
        </p>
      </div>
      <div className="card-panel">
        {side && at && (
          <div className="side">
            <svg viewBox="0 0 1000 300" preserveAspectRatio="none">
              <line
                className="side-ground"
                x1="0"
                y1="299"
                x2="1000"
                y2="299"
                vectorEffect="non-scaling-stroke"
              />
              <line
                className="side-runway"
                x1={side.runwayX * 1000 - (side.departing ? 0 : 70)}
                y1="297"
                x2={side.runwayX * 1000 + (side.departing ? 70 : 0)}
                y2="297"
                vectorEffect="non-scaling-stroke"
              />
              <line
                className="side-glide"
                x1={side.glide[0][0] * 1000}
                y1={side.glide[0][1] * 300}
                x2={side.glide[1][0] * 1000}
                y2={side.glide[1][1] * 300}
                vectorEffect="non-scaling-stroke"
              />
              {side.windowY !== null && (
                <line
                  className="side-window"
                  x1="0"
                  y1={side.windowY * 300}
                  x2="1000"
                  y2={side.windowY * 300}
                  vectorEffect="non-scaling-stroke"
                />
              )}
              {side.trail.length > 1 && (
                <polyline
                  className="side-trail"
                  points={[...side.trail, side.plane]
                    .map(([x, y]) => `${(x * 1000).toFixed(0)},${(y * 300).toFixed(0)}`)
                    .join(" ")}
                  vectorEffect="non-scaling-stroke"
                />
              )}
            </svg>
            <span
              className="side-plane"
              style={{
                left: `${(side.plane[0] * 100).toFixed(1)}%`,
                top: `${(side.plane[1] * 100).toFixed(1)}%`,
              }}
            />
            {side.windowY !== null && (
              <span
                className={`side-label is-window${side.departing ? " is-right" : ""}`}
                style={{ top: `${(side.windowY * 100).toFixed(1)}%` }}
              >
                your window
              </span>
            )}
            <span className={`side-label is-runway${side.departing ? "" : " is-right"}`}>
              {at.runway || airport?.runway_in_use
                ? `runway ${at.runway || airport?.runway_in_use}`
                : airport?.iata}
            </span>
          </div>
        )}
        {side?.path && <p className="side-note">{side.path}</p>}
        <div className="tiles">
          {tiles(plane, w).map((t) => (
            <p key={t.label} className="tile">
              <span className="tile-label">{t.label}</span>
              <span className="tile-value">{t.value}</span>
              {t.note && <span className="tile-note">{t.note}</span>}
            </p>
          ))}
        </div>
      </div>
    </div>
  );
}

// tiles are the plane's instruments: height (against the window's), speed, and climb or descent.
function tiles(p: Plane, w: Window): { label: string; value: string; note: string }[] {
  const out = [
    {
      label: "Height",
      value: feet(p) || "–",
      note: heightFromWindow(p, w).replace("your window", "you"),
    },
    {
      label: "Speed",
      value: speed(p) || (p.on_ground ? `${Math.round(p.speed_kt ?? 0)} kt` : "–"),
      note: p.speed_kt !== undefined && !p.on_ground ? `${Math.round(p.speed_kt)} knots` : "",
    },
  ];
  const v = p.vert_rate_fpm;
  if (!p.on_ground && v !== undefined) {
    const n = Math.round(Math.abs(v) / 50) * 50;
    const ms = Math.max(30, (p.speed_kt ?? 150) * 0.5144);
    const angle = (Math.atan2(Math.abs(v) * 0.00508, ms) * 180) / Math.PI;
    out.push(
      n < 300
        ? { label: "Climb", value: "level", note: "" }
        : {
            label: v < 0 ? "Descent" : "Climb",
            value: `${thousands.format(n)} ft/min`,
            note: `${angle.toFixed(1)}° ${v < 0 ? "down" : "up"}`,
          },
    );
  }
  return out;
}

// withoutRoute says what a plane with no known route is doing: landing at or leaving the followed airport, or
// passing close by.
function withoutRoute(p: Plane, ap?: FollowedAirport): string {
  const name = ap?.name ?? "the airport";
  if (p.airport?.op === "arriving") return `Landing at ${name}`;
  if (p.airport?.op === "departing") return `Leaving ${name}`;
  if (p.airport) return `On the ground at ${name}`;
  return p.near ? "Passing close by" : "Route unknown";
}
