"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import Clouds from "./Clouds";
import Scenery from "./Scenery";
import { shown } from "@/lib/airport";
import { attitude, draw, lookFor, sunVector } from "@/lib/aircraft3d";
import { label } from "@/lib/format";
import {
  compassTicks,
  horizonY,
  inFrame,
  looksDown,
  nearestTurn,
  offsetFrom,
  roofline,
  rooftops,
  sill,
  toScreen,
  screenTop,
} from "@/lib/sky";
import { sunGlow } from "@/lib/skyColour";
import { projector, sceneryFor } from "@/lib/scenery";
import { magnitudeLimit, starColour, starsAbove } from "@/lib/stars";
import { moonAt, moonPath, type SkyPhase } from "@/lib/sun";
import type { Plane, View, Window } from "@/lib/types";

interface Props {
  view: View | null;
  now: Date | null;
  phase: SkyPhase;
  sun: { alt: number; az: number } | null;
  featuredHex?: string;
  next?: Plane;
}

const trailPoints = 15; // 30 s of positions at one snapshot every 2 s

// The window: the sky as it is outside (the light round the sun, the real stars and the moon where they are), the
// rooftops opposite (or, from high up, the water below the horizon), the planes in view and a cue where the next one
// appears.
export default function Sky({ view, now, phase, sun, featuredHex, next }: Props) {
  // A new window object arrives with every snapshot; the same window, the same object, so the sky is not redone.
  const snapshotWindow = view?.window;
  const w = useMemo(
    () => snapshotWindow,
    [snapshotWindow?.azimuth_deg, snapshotWindow?.fov_deg, snapshotWindow?.min_elevation_deg],
  );
  const high = w ? looksDown(w) : false;
  const roofs = useMemo(() => (high ? sill() : rooftops()), [high]);
  const lat = view?.center.lat;
  const lon = view?.center.lon;
  // From high over Toronto's harbour, the view below the horizon is drawn from the map; elsewhere, a plain band.
  const home = useMemo(() => (lat === undefined || lon === undefined ? undefined : { lat, lon }), [lat, lon]);
  const scenery = useMemo(() => (home && w ? sceneryFor(home, w) : null), [home, w]);
  const horizon = useMemo(
    () => (scenery && home && w ? projector(home, w, scenery.lake_m).horizonY : w ? horizonY(w) : undefined),
    [scenery, home, w],
  );
  const weather = view?.weather;
  const visible = useMemo(() => shown(view).filter((p) => p.in_view), [view]);
  const trails = useTrails(visible, w);
  const turns = useTurnRates(visible);
  const sunVec = useMemo(
    () => sunVector(sun ? { alt: Math.round(sun.alt), az: Math.round(sun.az / 5) * 5 } : null),
    [sun && Math.round(sun.alt), sun && Math.round(sun.az / 5)],
  );
  const size = useScreenSize();
  const dark = phase === "night" || phase === "twilight";

  // The heavens move slowly: worked out again only when the clock (every 20 s) or the window changes.
  const heavens = useMemo(() => {
    if (!w || !now || lat === undefined || lon === undefined || !sun) return null;
    const stars = starsAbove(lat, lon, now, magnitudeLimit(sun.alt)).flatMap((st) => {
      const off = offsetFrom(w, st.az);
      return inFrame(w, off, st.alt) ? [{ ...toScreen(w, off, st.alt), mag: st.mag, bv: st.bv }] : [];
    });
    const m = moonAt(lat, lon, now);
    const moonOff = offsetFrom(w, m.az);
    const moon = m.alt > 0 && inFrame(w, moonOff, m.alt) ? { ...m, ...toScreen(w, moonOff, m.alt) } : null;
    const sunOff = offsetFrom(w, sun.az);
    const sunY = sun.alt > w.min_elevation_deg ? toScreen(w, sunOff, sun.alt).y : roofline;
    // A path of light on the water under the sun, or under a bright moon at night, when either is low and ahead.
    const clear = 1 - Math.min(1, (weather?.cloud_low ?? 0) / 100) * 0.9;
    let glitter = null;
    if (sun.alt > -1 && sun.alt < 35 && Math.abs(sunOff) < w.fov_deg / 2 + 5) {
      glitter = {
        offsetDeg: sunOff,
        alt: sun.alt,
        moon: false,
        strength: (sun.alt < 15 ? 0.6 : 0.35) * clear,
      };
    } else if (
      sun.alt < -8 &&
      m.alt > 0 &&
      m.alt < 40 &&
      m.fraction > 0.3 &&
      Math.abs(moonOff) < w.fov_deg / 2
    ) {
      glitter = { offsetDeg: moonOff, alt: m.alt, moon: true, strength: 0.45 * m.fraction * clear };
    }
    return {
      stars,
      moon,
      glow: sunGlow(sun.alt, sunOff, w.fov_deg, sunY),
      sunOffset: sunOff,
      limit: magnitudeLimit(sun.alt),
      glitter: glitter && glitter.strength > 0.05 ? glitter : null,
    };
  }, [w, now, lat, lon, sun, weather?.cloud_low]);
  // The horizon hazes over as the air thickens: a little always, as in any photo across the lake.
  const haze = weather ? Math.min(0.75, Math.max(0.2, 0.25 + (1 - weather.visibility_m / 30000) * 0.5)) : 0.3;
  const scale = size ? size.height / 1080 : 1;

  return (
    <div className="sky" aria-hidden="true">
      {heavens?.glow && (
        <div
          className="sun-glow"
          style={{
            left: `${heavens.glow.x * 100}%`,
            top: `${heavens.glow.y * 100}%`,
            opacity: heavens.glow.opacity,
            background: `radial-gradient(closest-side, ${heavens.glow.colour}, transparent)`,
          }}
        />
      )}
      {heavens && heavens.stars.length > 0 && (
        // No viewBox: positions in percent, radii in pixels, so a star is round on any screen shape.
        <svg className="stars">
          {heavens.stars.map((st, i) => (
            <circle
              key={i}
              cx={`${(st.x * 100).toFixed(2)}%`}
              cy={`${(st.y * 100).toFixed(2)}%`}
              r={(Math.min(2.8, Math.max(1.05, 2.4 - 0.3 * st.mag)) * scale).toFixed(2)}
              fill={starColour(st.bv)}
              opacity={Math.min(1, Math.max(0.3, 0.45 + (heavens.limit - st.mag) * 0.18)).toFixed(2)}
            />
          ))}
        </svg>
      )}
      {heavens?.moon && <MoonDisc moon={heavens.moon} dark={dark} />}
      {w && weather && sun && (
        <Clouds weather={weather} w={w} sunAlt={sun.alt} horizonY={horizon ?? roofline} />
      )}
      {/* Below the horizon, under the planes: from high up they are seen against the water or the city. */}
      {scenery && home && w && sun ? (
        <Scenery
          scenery={scenery}
          home={home}
          w={w}
          sunAlt={sun.alt}
          glitter={heavens?.glitter ?? null}
          sunOffset={heavens?.sunOffset ?? 0}
          haze={haze}
        />
      ) : (
        horizon !== undefined && <div className="below-horizon" style={{ top: `${horizon * 100}%` }} />
      )}
      {w && (
        <svg className="trails" viewBox="0 0 1000 1000" preserveAspectRatio="none">
          {/* Up to where the plane was at the last snapshot: it glides on from there, so the trail follows it. */}
          {[...trails].map(([hex, pts]) => (
            <polyline
              key={hex}
              className={hex === featuredHex ? "trail is-featured" : "trail"}
              points={pts
                .slice(0, -1)
                .map((p) => `${(p.x * 1000).toFixed(1)},${(p.y * 1000).toFixed(1)}`)
                .join(" ")}
              vectorEffect="non-scaling-stroke"
            />
          ))}
        </svg>
      )}
      {w &&
        size &&
        visible.map((p) => (
          <Craft
            key={p.hex}
            plane={p}
            w={w}
            size={size}
            dark={dark}
            featured={p.hex === featuredHex}
            sun={sunVec}
            turn={turns.get(p.hex) ?? 0}
          />
        ))}
      {w && next?.coming && <Cue plane={next} w={w} />}
      {!scenery && (
        <svg className="roofs" viewBox="0 0 1000 1000" preserveAspectRatio="none">
          <path d={roofs} />
        </svg>
      )}
      {w && (
        // Compass points stand on the horizon: the roofline, or from high up the line where sky meets water.
        <div
          className="compass"
          style={horizon === undefined ? undefined : { top: `calc(${horizon * 100}% - 1.7rem)` }}
        >
          {compassTicks(w).map((t) => (
            <span key={t.label} style={{ left: `${t.x * 100}%` }}>
              {t.label}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}

// MoonDisc is the moon where it stands in the window, lit on the side that faces the sun, as seen from home. It is
// drawn larger than life (the real one is half a degree, a few pixels here), as the eye remembers it.
function MoonDisc({
  moon,
  dark,
}: {
  moon: { x: number; y: number; fraction: number; limbDeg: number };
  dark: boolean;
}) {
  return (
    <div
      className={`moon${dark ? " is-night" : ""}`}
      style={{ left: `${moon.x * 100}%`, top: `${moon.y * 100}%` }}
    >
      <svg viewBox="-1 -1 2 2">
        <circle className="moon-dark" r={1} />
        <path
          className="moon-lit"
          d={moonPath(moon.fraction, 1)}
          transform={`rotate(${(-moon.limbDeg).toFixed(1)})`}
        />
      </svg>
    </div>
  );
}

// useTrails remembers where each plane in view has been on screen, for a short fading trail.
function useTrails(visible: Plane[], w: Window | undefined) {
  const [trails, setTrails] = useState(new Map<string, { x: number; y: number }[]>());
  useEffect(() => {
    if (!w) return;
    setTrails((prev) => {
      const next = new Map<string, { x: number; y: number }[]>();
      for (const p of visible) {
        const pts = [...(prev.get(p.hex) ?? []), toScreen(w, p.offset_deg, p.elevation_deg)];
        next.set(p.hex, pts.slice(-trailPoints));
      }
      return next; // planes gone from view drop their trails
    });
  }, [visible, w]);
  return trails;
}

// useScreenSize is the screen's size in pixels, for placing planes with transforms; null until the page runs.
function useScreenSize(): { width: number; height: number } | null {
  const [size, setSize] = useState<{ width: number; height: number } | null>(null);
  useEffect(() => {
    const measure = () => setSize({ width: window.innerWidth, height: window.innerHeight });
    measure();
    window.addEventListener("resize", measure);
    return () => window.removeEventListener("resize", measure);
  }, []);
  return size;
}

interface CraftProps {
  plane: Plane;
  w: Window;
  size: { width: number; height: number };
  dark: boolean;
  featured: boolean;
  /** The sun's direction, east-north-up, or null at night. */
  sun: [number, number, number] | null;
  /** How fast its track is turning, degrees a second: for its bank. */
  turn: number;
}

// Craft is one plane in view: its navigation lights after dark, a silhouette by day, pointing the way it moves.
// It is placed with a transform and glides there by transition, which the compositor runs: no layout per frame.
function Craft({ plane, w, size, dark, featured, sun, turn }: CraftProps) {
  const pos = toScreen(w, plane.offset_deg, plane.elevation_deg);
  const look = useMemo(
    () => lookFor(plane),
    // Its kind and colours change only with its type or operator.
    [plane.type_code, plane.info?.type_code, plane.category, plane.callsign, plane.info?.route?.airline_icao],
  );
  // Its true size from the window, made larger so it reads from the sofa: far off a small model, close in more of
  // the sky, as close planes are.
  const scale = size.height / 1080;
  const slantM = (plane.distance_km * 1000) / Math.max(0.25, Math.cos((plane.elevation_deg * Math.PI) / 180));
  const trueDeg = ((look.lengthM / Math.max(200, slantM)) * 180) / Math.PI;
  const lengthPx =
    Math.min(260, Math.max(featured ? 100 : 64, trueDeg * (size.width / w.fov_deg) * 3)) * scale;
  const att = attitude(plane, turn);
  // Drawn again only when the view of it changes enough to see.
  const key = [
    Math.round(att.headingDeg / 2),
    Math.round(att.pitchDeg),
    Math.round(att.rollDeg / 3),
    Math.round(plane.bearing_deg),
    Math.round(plane.elevation_deg),
    Math.round(lengthPx / 3),
    dark,
    sun?.map((v) => v.toFixed(1)).join(),
  ].join("|");
  const drawn = useMemo(
    () =>
      draw(
        look,
        att,
        { bearingDeg: plane.bearing_deg, elevationDeg: plane.elevation_deg },
        sun,
        lengthPx,
        dark,
      ),
    [look, key],
  );
  const box = Math.ceil(lengthPx * 1.7);
  const x = (pos.x * size.width).toFixed(1);
  const y = (pos.y * size.height).toFixed(1);
  const at = ([lx, ly]: [number, number]) => ({ transform: `translate(${lx}px, ${ly}px)` });
  // Landing lights burn below 10,000 ft and on the runway; they glare when the nose points at the window.
  const lit = (plane.alt_ft ?? 99999) < 10000 || (plane.on_ground && (plane.speed_kt ?? 0) > 30);
  const glare = lit ? drawn.glare * (dark ? 1 : 0.55) : 0;
  const labelLeft = pos.x > 0.8;
  const labelAt = Math.round(lengthPx * 0.42 + 10 * scale);
  return (
    <div
      className={`craft${featured ? " is-featured" : ""}${labelLeft ? " label-left" : ""}`}
      style={{ transform: `translate3d(${x}px, ${y}px, 0)` }}
    >
      <svg
        className="model"
        width={box}
        height={box}
        viewBox={`${-box / 2} ${-box / 2} ${box} ${box}`}
        style={{ left: -box / 2, top: -box / 2 }}
      >
        {drawn.faces.map((f, i) => (
          <polygon key={i} points={f.points} fill={f.fill} opacity={f.opacity} />
        ))}
      </svg>
      {dark && (
        <>
          <i className="light port" style={at(drawn.lights.port)} />
          <i className="light starboard" style={at(drawn.lights.starboard)} />
          <i className="light tail" style={at(drawn.lights.tail)} />
          <i
            className="light beacon"
            style={{ ...at(drawn.lights.beacon), animationDelay: `${strobeDelay(plane.hex)}s` }}
          />
          <i
            className="light strobe"
            style={{ ...at(drawn.lights.port), animationDelay: `${strobeDelay(plane.hex)}s` }}
          />
          <i
            className="light strobe"
            style={{ ...at(drawn.lights.starboard), animationDelay: `${strobeDelay(plane.hex)}s` }}
          />
        </>
      )}
      {glare > 0.08 && (
        <i
          className="glare"
          style={{ ...at(drawn.lights.landing), opacity: Math.min(1, glare).toFixed(2) }}
        />
      )}
      <span
        className="craft-label"
        style={
          labelLeft
            ? { right: labelAt, top: -labelAt * 0.6 - 18 * scale }
            : { left: labelAt, top: -labelAt * 0.6 - 18 * scale }
        }
      >
        {label(plane)}
      </span>
    </div>
  );
}

// Cue marks where the next plane will come into view, at the edge it comes from.
function Cue({ plane, w }: { plane: Plane; w: Window }) {
  const c = plane.coming;
  if (!c) return null;
  const at = toScreen(w, c.offset_deg, c.elevation_deg);
  // At the edge it comes from, inside the TV-safe area so an overscanning TV cannot crop it.
  const left =
    c.side === "left"
      ? "var(--safe-x)"
      : c.side === "right"
        ? "calc(100% - var(--safe-x))"
        : `${Math.min(0.85, Math.max(0.15, at.x)) * 100}%`;
  const top =
    c.side === "rooftops"
      ? `${(roofline - 0.04) * 100}%`
      : c.side === "above"
        ? "calc(var(--safe-y) + 1rem)"
        : `${Math.max(0.1, at.y) * 100}%`; // below the safe top margin; CSS max() needs Chromium 79
  return (
    <div className={`cue cue-${c.side}`} style={{ left, top }}>
      <span className="cue-mark" />
      <span className="cue-text">
        {label(plane)} {c.in_s < 45 ? "any moment" : `in ${Math.round(c.in_s / 60)} min`}
      </span>
    </div>
  );
}

// screenAngle turns a plane's motion across the window into a direction on screen, degrees clockwise from right.
function screenAngle(p: Plane, w: Window): number {
  if (!p.motion) return 0;
  const dx = p.motion.offset_deg / w.fov_deg; // screen widths
  const dy = ((-p.motion.elevation_deg / (screenTop(w) - w.min_elevation_deg)) * roofline * 9) / 16; // in widths, 16:9
  return (Math.atan2(dy, dx) * 180) / Math.PI;
}

// strobeDelay spreads the strobes so the planes don't flash in step.
function strobeDelay(hex: string): number {
  let h = 0;
  for (const ch of hex) h = (h * 31 + ch.charCodeAt(0)) % 997;
  return -(h % 160) / 100;
}

// useTurnRates follows each plane's track from one report to the next: how fast it turns, smoothed, for its bank.
function useTurnRates(planes: Plane[]): Map<string, number> {
  const seen = useRef(new Map<string, { track: number; at: number; rate: number }>());
  return useMemo(() => {
    const out = new Map<string, number>();
    for (const p of planes) {
      if (p.track_deg === undefined) continue;
      const at = Date.parse(p.at);
      const prev = seen.current.get(p.hex);
      let rate = prev?.rate ?? 0;
      if (prev && at - prev.at > 900 && at - prev.at < 30_000) {
        const turned = ((((p.track_deg - prev.track) % 360) + 540) % 360) - 180;
        rate = Math.max(-6, Math.min(6, rate * 0.6 + (turned / ((at - prev.at) / 1000)) * 0.4));
      }
      if (!prev || at > prev.at) seen.current.set(p.hex, { track: p.track_deg, at, rate });
      out.set(p.hex, rate);
    }
    return out;
  }, [planes]);
}
