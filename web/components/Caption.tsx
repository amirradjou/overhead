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
import type { Status } from "@/lib/useLiveView";
import type { Plane } from "@/lib/types";

interface Props {
  plane?: Plane;
  alsoInView: number;
  next?: Plane;
  facts: string[];
  now: Date | null;
  status: Status;
}

// A fact stays up this long before the next takes its place.
const factEvery = 20_000;

// The words over the rooftops: the plane in view, or what comes next; between planes, a fact about the history.
export default function Caption({ plane, alsoInView, next, facts, now, status }: Props) {
  const fact = facts.length && now ? facts[Math.floor(now.getTime() / factEvery) % facts.length] : undefined;
  return (
    <section className="caption" aria-live="polite">
      {/* Keyed on the plane, so a new one fades in rather than overwriting the old one mid-sentence. */}
      <div className="caption-body" key={plane?.hex ?? next?.hex ?? status}>
        {plane ? <InView plane={plane} /> : <Waiting next={next} status={status} />}
      </div>
      <p className="caption-foot">
        {plane ? (
          <>
            {alsoInView > 0 && <span>{alsoInView === 1 ? "One more" : `${alsoInView} more`} in view. </span>}
            {next && (
              <span>
                Next, {flightName(next)} {arrival(next)}.
              </span>
            )}
          </>
        ) : (
          // The server's facts include the day's count of planes across the window: the one "today" number shown.
          fact && (
            <span key={fact} className="fact">
              {fact}
            </span>
          )
        )}
      </p>
    </section>
  );
}

function InView({ plane }: { plane: Plane }) {
  const photo = plane.info?.photo;
  const route = fromTo(plane);
  const facts = flightFacts(plane);
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
        <p className="route">{route || operator(plane) || "Route unknown"}</p>
        {facts && <p className="facts">{facts}</p>}
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
      {flightFacts(next) && <p className="facts">{flightFacts(next)}</p>}
    </>
  );
}
