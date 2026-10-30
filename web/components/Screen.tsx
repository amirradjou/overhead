"use client";

import { useEffect, useState } from "react";
import Caption from "./Caption";
import Overview from "./Overview";
import Sky from "./Sky";
import { comingNext, inView } from "@/lib/pick";
import { skyPhase, sunElevation } from "@/lib/sun";
import { useLiveView } from "@/lib/useLiveView";

const clockFormat = new Intl.DateTimeFormat("en-CA", {
  hour: "2-digit",
  minute: "2-digit",
  hourCycle: "h23",
});

export default function Screen() {
  const { view, status } = useLiveView();
  const now = useNow(20_000);
  const drift = useDrift();

  const phase = view && now ? skyPhase(sunElevation(view.center.lat, view.center.lon, now)) : "night";
  const planes = view?.planes ?? [];
  const visible = inView(planes);
  const next = comingNext(planes)[0];
  const sources = view?.sources.length ? view.sources.join(" and ") : "community ADS-B feeds";

  return (
    <main className={`screen sky-${phase}${status === "live" ? "" : " is-stale"}`}>
      <Sky view={view} phase={phase} featuredHex={visible[0]?.hex} next={next} />
      {/* Everything that would otherwise sit still drifts a few pixels now and then, so nothing burns into a TV. */}
      <div className="overlay" style={{ transform: `translate(${drift.x}px, ${drift.y}px)` }}>
        <p className="clock">
          {now && clockFormat.format(now)}
          {status !== "live" && <span className="clock-status">reconnecting</span>}
        </p>
        {view && <Overview view={view} />}
        <Caption
          plane={visible[0]}
          alsoInView={visible.length - 1}
          next={next}
          today={view?.today}
          facts={view?.facts ?? []}
          now={now}
          status={status}
        />
        <p className="credits">
          Positions from {sources}; aircraft and routes from adsbdb; photos from planespotters.net.
        </p>
      </div>
    </main>
  );
}

// useNow is the time, refreshed every interval; null until the page runs in a browser, so the static export holds
// no stale time.
function useNow(interval: number): Date | null {
  const [now, setNow] = useState<Date | null>(null);
  useEffect(() => {
    setNow(new Date());
    const id = window.setInterval(() => setNow(new Date()), interval);
    return () => window.clearInterval(id);
  }, [interval]);
  return now;
}

// useDrift moves the overlay to a new spot within a few pixels every few minutes; the CSS eases it there slowly.
function useDrift(): { x: number; y: number } {
  const [drift, setDrift] = useState({ x: 0, y: 0 });
  useEffect(() => {
    const id = window.setInterval(() => {
      setDrift({ x: Math.round((Math.random() - 0.5) * 24), y: Math.round((Math.random() - 0.5) * 16) });
    }, 180_000);
    return () => window.clearInterval(id);
  }, []);
  return drift;
}
