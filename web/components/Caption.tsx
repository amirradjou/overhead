import { useMemo } from "react";
import { typeFactLines } from "@/lib/aircraft";
import { moonLine, sunLine } from "@/lib/almanac";
import {
  arrival,
  flightFacts,
  flightName,
  fromTo,
  landing,
  operator,
  photoCredit,
  webLink,
} from "@/lib/format";
import { rotating } from "@/lib/pick";
import { flightProgress, progressText } from "@/lib/progress";
import type { Status } from "@/lib/useLiveView";
import type { Plane, Window } from "@/lib/types";

interface Props {
  plane?: Plane;
  alsoInView: number;
  next?: Plane;
  facts: string[];
  now: Date | null;
  status: Status;
  home?: { lat: number; lon: number; window: Window };
}

// A line in the quiet place under the caption stays up this long before the next takes its place.
const lineEvery = 20_000;

// The words over the rooftops: the plane in view, or what comes next. Under them, one quiet line at a time: while a
// plane is in view, what comes next and a fact about the aircraft; between planes, a fact about the history, the
// next sunrise or sunset and the moon.
export default function Caption({ plane, alsoInView, next, facts, now, status, home }: Props) {
  const skyLines = useSkyLines(home, now);
  const lines = plane
    ? [following(alsoInView, next), ...typeFactLines(plane)].filter(Boolean)
    : [...facts, ...skyLines].filter(Boolean);
  const line = rotating(lines, now, lineEvery);
  return (
    <section className="caption" aria-live="polite">
      {/* Keyed on the plane, so a new one fades in rather than overwriting the old one mid-sentence. */}
      <div className="caption-body" key={plane?.hex ?? next?.hex ?? status}>
        {plane ? <InView plane={plane} /> : <Waiting next={next} status={status} />}
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
function following(alsoInView: number, next?: Plane): string {
  const more = alsoInView > 0 ? `${alsoInView === 1 ? "One more" : `${alsoInView} more`} in view.` : "";
  const then = next ? `Next, ${flightName(next)} ${arrival(next)}.` : "";
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

// Route is "From San Francisco to Toronto", and the line under it is the trip: origin at its left end, destination at
// its right, the part flown brighter, a small mark where the plane is now.
function Route({
  plane,
  done,
  fallback,
  after,
}: {
  plane: Plane;
  done?: number;
  fallback: string;
  after?: string;
}) {
  const route = fromTo(plane);
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

function Waiting({ next, status }: { next?: Plane; status: Status }) {
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
        <p className="route">Nothing is heading this way right now.</p>
      </>
    );
  }
  const progress = flightProgress(next);
  return (
    <>
      <p className="notable">Coming up</p>
      <h1 className="flight">{flightName(next)}</h1>
      <Route plane={next} done={progress?.done} fallback="" after={arrival(next)} />
      {flightFacts(next) && <p className="facts">{flightFacts(next)}</p>}
    </>
  );
}
