import { useMemo } from "react";
import { typeFactLines } from "@/lib/aircraft";
import {
  airportDay,
  airportStatus,
  feet,
  fromToHere,
  heightFromWindow,
  lookWhere,
  vertical,
} from "@/lib/airport";
import { moonLine, sunLine } from "@/lib/almanac";
import {
  aircraftType,
  arrival,
  flightFacts,
  flightName,
  fromTo,
  landing,
  operator,
  photoCredit,
  speed,
  webLink,
} from "@/lib/format";
import { rotating } from "@/lib/pick";
import { flightProgress, progressText } from "@/lib/progress";
import type { Status } from "@/lib/useLiveView";
import type { FollowedAirport, Plane, Window } from "@/lib/types";

interface Props {
  plane?: Plane;
  alsoInView: number;
  next?: Plane;
  facts: string[];
  now: Date | null;
  status: Status;
  home?: { lat: number; lon: number; window: Window };
  /** The airport the screen follows, if any: its flights get the full story. */
  airport?: FollowedAirport;
}

// A line in the quiet place under the caption stays up this long before the next takes its place.
const lineEvery = 20_000;

// The words over the rooftops: the plane in view, or what comes next. Under them, one quiet line at a time: while a
// plane is in view, what comes next and a fact about the aircraft; between planes, a fact about the history, the
// next sunrise or sunset and the moon.
export default function Caption({ plane, alsoInView, next, facts, now, status, home, airport }: Props) {
  const skyLines = useSkyLines(home, now);
  const day = airport ? airportDay(airport) : "";
  const lines = plane
    ? [following(alsoInView, next, home?.window), ...typeFactLines(plane), day].filter(Boolean)
    : [day, ...facts, ...skyLines].filter(Boolean);
  const full = plane && home && (plane.airport || plane.near);
  const line = rotating(lines, now, lineEvery);
  return (
    <section className="caption" aria-live="polite">
      {/* Keyed on the plane, so a new one fades in rather than overwriting the old one mid-sentence. */}
      <div className="caption-body" key={plane?.hex ?? next?.hex ?? status}>
        {full ? (
          <AirportFlight plane={plane} airport={airport} w={home.window} />
        ) : plane ? (
          <InView plane={plane} />
        ) : (
          <Waiting next={next} status={status} w={home?.window} airport={airport} />
        )}
      </div>
      <p className="caption-foot">
        {line && (
          <span key={line} className="fact">
            {line}
          </span>
        )}
      </p>
    </section>
  );
}

// "2 more in view. Next, Delta Air Lines 55 from the right in about 2 min."
function following(alsoInView: number, next?: Plane, w?: Window): string {
  const more = alsoInView > 0 ? `${alsoInView === 1 ? "One more" : `${alsoInView} more`} in view.` : "";
  const then = next ? `Next, ${flightName(next)} ${arrival(next, w)}.` : "";
  return [more, then].filter(Boolean).join(" ");
}

// useSkyLines is the sun's and moon's news for home, worked out again only as the clock moves.
function useSkyLines(home: Props["home"], now: Date | null): string[] {
  const lat = home?.lat;
  const lon = home?.lon;
  const w = home?.window;
  // Keyed on the window's settings: a new window object arrives with every snapshot.
  const windowKey = w ? `${w.azimuth_deg}/${w.fov_deg}/${w.min_elevation_deg}` : "";
  return useMemo(() => {
    if (lat === undefined || lon === undefined || !w || !now) return [];
    return [sunLine(lat, lon, now), moonLine(lat, lon, now, w)];
  }, [lat, lon, windowKey, now]);
}

function InView({ plane }: { plane: Plane }) {
  const photo = plane.info?.photo;
  const facts = flightFacts(plane);
  const progress = flightProgress(plane);
  return (
    <div className="in-view">
      {photo && (
        // Linked to the photo's page on planespotters.net, as their terms ask.
        <a className="photo" href={webLink(photo.link)} rel="noopener">
          <figure>
            <img src={photo.large || photo.thumbnail} alt="" />
            <figcaption>{photoCredit(plane)}</figcaption>
          </figure>
        </a>
      )}
      <div>
        {plane.notable && <p className="notable">{plane.notable}</p>}
        <h1 className="flight">{flightName(plane)}</h1>
        <Route plane={plane} done={progress?.done} fallback={operator(plane) || "Route unknown"} />
        {progress && <p className="progress">{progressText(plane, progress)}</p>}
        {facts && <p className="facts">{facts}</p>}
        {landing(plane) && <p className="landing">{landing(plane)}</p>}
      </div>
    </div>
  );
}

// AirportFlight is a flight at the followed airport, or a plane very close to the window, told in full: what it is
// doing there, the trip, its height against the window's, its speed, where to look, and the airframe. Each line is
// one sentence-like run of words, not a grid: old TV browsers ignore flex gaps.
function AirportFlight({ plane, airport, w }: { plane: Plane; airport?: FollowedAirport; w: Window }) {
  const photo = plane.info?.photo;
  const progress = plane.on_ground ? null : flightProgress(plane); // a trip bar for a plane at the gate says nothing
  const reg = plane.info?.registration || plane.registration;
  const type = aircraftType(plane);
  const motion = [speed(plane), vertical(plane)].filter(Boolean).join(", ");
  const d = plane.distance_km;
  const away = `${d < 10 ? d.toFixed(1) : Math.round(d)} km away`;
  // Two lines, so the caption stays under the sill: how it flies, then where it is and what it is.
  const stats: [string, string][][] = [
    [
      ["Height", [feet(plane), heightFromWindow(plane, w)].filter(Boolean).join(", ")],
      ["Speed", motion],
    ],
    [
      ["Look", `${lookWhere(plane)}, ${away}`],
      ["Aircraft", [type, reg].filter(Boolean).join(", ")],
    ],
  ];
  // An airline flight by its name; anything else by its registration, as it is painted on the plane.
  const title = plane.info?.route?.airline ? flightName(plane) : reg || flightName(plane);
  return (
    <div className="in-view is-full">
      {photo && (
        <a className="photo" href={webLink(photo.link)} rel="noopener">
          <figure>
            <img src={photo.large || photo.thumbnail} alt="" />
            <figcaption>{photoCredit(plane)}</figcaption>
          </figure>
        </a>
      )}
      <div>
        <p className="notable">{plane.notable || airportStatus(plane, airport)}</p>
        <h1 className="flight">{title}</h1>
        <Route
          plane={plane}
          text={fromToHere(plane, airport)}
          done={progress?.done}
          fallback={operator(plane) || "Route unknown"}
          after={progress ? progressText(plane, progress) : undefined}
        />
        {stats.map((row, i) => (
          <p key={i} className="stats">
            {row
              .filter(([, v]) => v)
              .map(([k, v]) => (
                <span key={k} className="stat">
                  <span className="stat-label">{k}</span> {v}
                </span>
              ))}
          </p>
        ))}
      </div>
    </div>
  );
}

// Route is "From San Francisco to Toronto", and the line under it is the trip: origin at its left end, destination at
// its right, the part flown brighter, a small mark where the plane is now.
function Route({
  plane,
  text,
  done,
  fallback,
  after,
}: {
  plane: Plane;
  /** The route in other words than fromTo's. */
  text?: string;
  done?: number;
  fallback: string;
  after?: string;
}) {
  const route = text || fromTo(plane);
  if (!route) return <p className="route">{[fallback, after].filter(Boolean).join(", ")}</p>;
  return (
    <p className="route">
      <span className={done === undefined ? undefined : "route-trip"}>
        {route}
        {done !== undefined && (
          <span className="trip">
            <span className="trip-flown" style={{ width: `${(done * 100).toFixed(1)}%` }} />
            <span className="trip-here" style={{ left: `${(done * 100).toFixed(1)}%` }} />
          </span>
        )}
      </span>
      {after && `, ${after}`}
    </p>
  );
}

function Waiting({
  next,
  status,
  w,
  airport,
}: {
  next?: Plane;
  status: Status;
  w?: Window;
  airport?: FollowedAirport;
}) {
  if (status !== "live") {
    return (
      <>
        <h1 className="flight is-quiet">Looking for planes</h1>
        <p className="route">Waiting for the live feed.</p>
      </>
    );
  }
  if (!next) {
    return (
      <>
        <h1 className="flight is-quiet">Clear skies</h1>
        <p className="route">
          {airport
            ? `Nothing is landing at or leaving ${airport.name} right now.`
            : "Nothing is heading this way right now."}
        </p>
      </>
    );
  }
  const progress = flightProgress(next);
  return (
    <>
      <p className="notable">
        {next.airport
          ? `Coming up: ${airportStatus(next, airport).replace(/^./, (c) => c.toLowerCase())}`
          : "Coming up"}
      </p>
      <h1 className="flight">{flightName(next)}</h1>
      <Route plane={next} done={progress?.done} fallback="" after={arrival(next, w)} />
      {flightFacts(next) && <p className="facts">{flightFacts(next)}</p>}
    </>
  );
}
