import { useMemo } from "react";
import FlightCard from "./FlightCard";
import { typeFactLines } from "@/lib/aircraft";
import { airportDay, airportStatus } from "@/lib/airport";
import { cloudLine, weatherLine } from "@/lib/weather";
import { moonLine, sunLine } from "@/lib/almanac";
import { arrival, flightFacts, flightName, fromTo, landing } from "@/lib/format";
import { rotating } from "@/lib/pick";
import { flightProgress } from "@/lib/progress";
import type { Status } from "@/lib/useLiveView";
import type { FollowedAirport, Plane, Weather, Window } from "@/lib/types";

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
  weather?: Weather;
}

// A line in the quiet place under the caption stays up this long before the next takes its place.
const lineEvery = 20_000;

// The words over the rooftops: the plane in view, or what comes next. Under them, one quiet line at a time: while a
// plane is in view, what comes next and a fact about the aircraft; between planes, a fact about the history, the
// next sunrise or sunset and the moon.
export default function Caption({
  plane,
  alsoInView,
  next,
  facts,
  now,
  status,
  home,
  airport,
  weather,
}: Props) {
  const skyLines = useSkyLines(home, now);
  const day = airport ? airportDay(airport) : "";
  // The weather, and the wind that decides the runway, between planes.
  const sky = weather ? [weatherLine(weather, airport), cloudLine(weather)] : [];
  const lines = plane
    ? [following(alsoInView, next, home?.window), ...typeFactLines(plane), day].filter(Boolean)
    : [day, ...sky, ...facts, ...skyLines].filter(Boolean);
  const line = rotating(lines, now, lineEvery);
  return (
    <section className="caption" aria-live="polite">
      {/* Keyed on the plane, so a new one fades in rather than overwriting the old one mid-sentence. */}
      <div className="caption-body" key={plane?.hex ?? next?.hex ?? status}>
        {plane && home ? (
          <FlightCard plane={plane} airport={airport} w={home.window} now={now} />
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
  if (status !== "live" && status !== "replay") {
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
