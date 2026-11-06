"use client";

import { Fragment, useEffect, useMemo, useState } from "react";
import AirportBoard from "./AirportBoard";
import Caption from "./Caption";
import Overview from "./Overview";
import Sky from "./Sky";
import { board, shown } from "@/lib/airport";
import { sceneryFor } from "@/lib/scenery";
import { sourceHomes } from "@/lib/format";
import { comingNext, inView } from "@/lib/pick";
import { skyColours } from "@/lib/skyColour";
import { skyPhase, sunPosition } from "@/lib/sun";
import type { View } from "@/lib/types";
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
  useSafeArea();

  const lat = view?.center.lat;
  const lon = view?.center.lon;
  const sun = useMemo(
    () => (lat !== undefined && lon !== undefined && now ? sunPosition(lat, lon, now) : null),
    [lat, lon, now],
  );
  const phase = sun ? skyPhase(sun.alt) : "night";
  const colours = skyColours(sun?.alt ?? -30);
  const planes = shown(view);
  const visible = inView(planes);
  const next = comingNext(planes)[0];
  // The harbour drawn from OpenStreetMap is credited as its licence asks.
  const mapped = !!view && !!sceneryFor(view.center, view.window);

  return (
    <main
      className={`screen sky-${phase}${status === "live" || status === "replay" ? "" : " is-stale"}${view?.public ? " is-public" : ""}`}
      style={
        {
          background: `linear-gradient(to bottom, ${colours.top} 0%, ${colours.mid} 45%, ${colours.low} var(--roofline))`,
          "--roof": colours.roof,
        } as React.CSSProperties
      }
    >
      {/* The whole scene, rooftops and words alike, drifts a few pixels now and then, so nothing burns into a TV. */}
      <div className="scene" style={{ transform: `translate(${drift.x}px, ${drift.y}px)` }}>
        <Sky view={view} now={now} phase={phase} sun={sun} featuredHex={visible[0]?.hex} next={next} />
        <div className="overlay">
          <p className="clock">
            {now && clockFormat.format(now)}
            {status === "replay" && <span className="clock-status">a recording</span>}
            {status !== "live" && status !== "replay" && <span className="clock-status">reconnecting</span>}
          </p>
          {view && <Overview view={view} />}
          {view?.airport && <AirportBoard airport={view.airport} rows={board(view)} />}
          <Caption
            plane={visible[0]}
            alsoInView={visible.length - 1}
            next={next}
            facts={view?.facts ?? []}
            now={now}
            status={status}
            home={view ? { lat: view.center.lat, lon: view.center.lon, window: view.window } : undefined}
            airport={view?.airport}
            weather={view?.weather}
          />
          <Credits view={view} mapped={mapped} replay={status === "replay"} />
        </div>
      </div>
    </main>
  );
}

// Credits names every source as its terms ask (docs/setup.md, section 7). On the public instance they begin with a
// line for someone who has never seen the window.
function Credits({ view, mapped, replay }: { view: View | null; mapped: boolean; replay: boolean }) {
  const sources = view?.sources ?? [];
  const photos = !view?.public || view.planes.some((p) => p.info?.photo);
  return (
    <p className="credits">
      {view?.public && (
        <span className="about">
          You are at a real window{mapped && " high over Toronto's harbour"}.{" "}
          {replay
            ? "These planes are a recording, as the live feed is away for now, drawn where you would have seen them."
            : "The planes are live, drawn where you would see them."}{" "}
          <a href="https://github.com/amirradjou/overhead" rel="noopener">
            How it works
          </a>
        </span>
      )}
      {view?.public && sources.includes("adsb.lol") ? (
        <span>
          Contains information from <Site name="adsb.lol" />, made available under the <Site name="ODbL" />
        </span>
      ) : (
        <span>
          Positions from{" "}
          {sources.length
            ? sources.map((name, i) => (
                <Fragment key={name}>
                  {i > 0 && (i === sources.length - 1 ? " and " : ", ")}
                  <Site name={name} />
                </Fragment>
              ))
            : "community ADS-B feeds"}
        </span>
      )}
      <span>
        Aircraft from <Site name="adsbdb" label="adsbdb.com" />
        {photos && (
          <>
            , photos from <Site name="planespotters.net" />
          </>
        )}
      </span>
      {view?.weather && (
        <span>
          Weather data by <Site name="open-meteo.com" label="Open-Meteo.com" /> (<Site name="CC BY 4.0" />
          ), clouds drawn from it
        </span>
      )}
      {mapped && (
        <span>
          Map © <Site name="OpenStreetMap" /> contributors (<Site name="ODbL" />)
        </span>
      )}
    </p>
  );
}

// Site names a source by its address, linked to its home page, as the sources' terms ask.
function Site({ name, label }: { name: string; label?: string }) {
  const home = sourceHomes[name];
  return home ? (
    <a href={home} rel="noopener">
      {label ?? name}
    </a>
  ) : (
    <>{label ?? name}</>
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

// useSafeArea lets a TV that crops more than most widen the margins every word keeps to: `?safe=8` keeps text 8% of
// the screen in from each edge.
function useSafeArea() {
  useEffect(() => {
    const safe = Number(new URLSearchParams(window.location.search).get("safe"));
    if (!safe || safe < 0 || safe > 20) return;
    document.documentElement.style.setProperty("--safe-x", `${safe}vw`);
    document.documentElement.style.setProperty("--safe-y", `${safe}vh`);
  }, []);
}

// useDrift moves the scene to a new spot within a few pixels every few minutes; the CSS eases it there slowly.
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
