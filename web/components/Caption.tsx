import {
  aircraftType,
  altitude,
  arrival,
  flightName,
  fromTo,
  landing,
  operator,
  photoCredit,
  speed,
  trend,
} from "@/lib/format";
import type { Status } from "@/lib/useLiveView";
import type { Plane } from "@/lib/types";

interface Props {
  plane?: Plane;
  alsoInView: number;
  next?: Plane;
  today?: number;
  status: Status;
}

const trendWord = { climbing: "climbing", descending: "descending", level: "" } as const;

// The words over the rooftops: the plane in view, or what comes next, and one line of the day's tally.
export default function Caption({ plane, alsoInView, next, today, status }: Props) {
  return (
    <section className="caption" aria-live="polite">
      {/* Keyed on the plane, so a new one fades in rather than overwriting the old one mid-sentence. */}
      <div className="caption-body" key={plane?.hex ?? next?.hex ?? status}>
        {plane ? <InView plane={plane} /> : <Waiting next={next} status={status} />}
      </div>
      <p className="caption-foot">
        {plane && alsoInView > 0 && (
          <span>{alsoInView === 1 ? "One more" : `${alsoInView} more`} in view. </span>
        )}
        {plane && next && (
          <span>
            Next, {flightName(next)} {arrival(next)}.{" "}
          </span>
        )}
        {today !== undefined && today > 0 && (
          <span>{today === 1 ? "One aircraft" : `${today} aircraft`} passed by today.</span>
        )}
      </p>
    </section>
  );
}

function InView({ plane }: { plane: Plane }) {
  const photo = plane.info?.photo;
  const route = fromTo(plane);
  const t = trend(plane);
  const facts = [
    aircraftType(plane),
    altitude(plane) && `${altitude(plane)} ${t ? trendWord[t] : ""}`.trim(),
    speed(plane),
  ];
  return (
    <div className="in-view">
      {photo && (
        <figure className="photo">
          <img src={photo.large || photo.thumbnail} alt="" />
          <figcaption>{photoCredit(plane)}</figcaption>
        </figure>
      )}
      <div>
        {plane.notable && <p className="notable">{plane.notable}</p>}
        <h1 className="flight">{flightName(plane)}</h1>
        <p className="route">{route || operator(plane) || "Route unknown"}</p>
        <p className="facts">
          {facts.filter(Boolean).map((f) => (
            <span key={f}>{f}</span>
          ))}
        </p>
        {landing(plane) && <p className="landing">{landing(plane)}</p>}
      </div>
    </div>
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
  const route = fromTo(next);
  return (
    <>
      <p className="notable">Coming up</p>
      <h1 className="flight">{flightName(next)}</h1>
      <p className="route">{route ? `${route}, ${arrival(next)}` : arrival(next)}</p>
      <p className="facts">
        {[aircraftType(next), altitude(next)].filter(Boolean).map((f) => (
          <span key={f}>{f}</span>
        ))}
      </p>
    </>
  );
}
