"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { label } from "@/lib/format";
import {
  compassTicks,
  inFrame,
  nearestTurn,
  offsetFrom,
  roofline,
  rooftops,
  toScreen,
  screenTop,
} from "@/lib/sky";
import { sunGlow } from "@/lib/skyColour";
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
// rooftops opposite, the planes in view and a cue where the next one appears.
export default function Sky({ view, now, phase, sun, featuredHex, next }: Props) {
  const roofs = useMemo(() => rooftops(), []);
  // A new window object arrives with every snapshot; the same window, the same object, so the sky is not redone.
  const snapshotWindow = view?.window;
  const w = useMemo(
    () => snapshotWindow,
    [snapshotWindow?.azimuth_deg, snapshotWindow?.fov_deg, snapshotWindow?.min_elevation_deg],
  );
  const lat = view?.center.lat;
  const lon = view?.center.lon;
  const visible = useMemo(() => view?.planes.filter((p) => p.in_view) ?? [], [view]);
  const trails = useTrails(visible, w);
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
    const moon = inFrame(w, moonOff, m.alt) ? { ...m, ...toScreen(w, moonOff, m.alt) } : null;
    const sunOff = offsetFrom(w, sun.az);
    const sunY = sun.alt > w.min_elevation_deg ? toScreen(w, sunOff, sun.alt).y : roofline;
    return { stars, moon, glow: sunGlow(sun.alt, sunOff, w.fov_deg, sunY), limit: magnitudeLimit(sun.alt) };
  }, [w, now, lat, lon, sun]);
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
          <Craft key={p.hex} plane={p} w={w} size={size} dark={dark} featured={p.hex === featuredHex} />
        ))}
      {w && next?.coming && <Cue plane={next} w={w} />}
      <svg className="roofs" viewBox="0 0 1000 1000" preserveAspectRatio="none">
        <path d={roofs} />
      </svg>
      {w && (
        <div className="compass">
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
}

// Craft is one plane in view: its navigation lights after dark, a silhouette by day, pointing the way it moves.
// It is placed with a transform and glides there by transition, which the compositor runs: no layout per frame.
function Craft({ plane, w, size, dark, featured }: CraftProps) {
  const pos = toScreen(w, plane.offset_deg, plane.elevation_deg);
  const angle = useRef<number | null>(null);
  angle.current = nearestTurn(angle.current ?? 0, screenAngle(plane, w));
  const x = (pos.x * size.width).toFixed(1);
  const y = (pos.y * size.height).toFixed(1);
  return (
    <div
      className={`craft${featured ? " is-featured" : ""}${pos.x > 0.8 ? " label-left" : ""}`}
      style={{ transform: `translate3d(${x}px, ${y}px, 0)` }}
    >
      <div className="craft-body" style={{ transform: `rotate(${angle.current}deg)` }}>
        {dark ? (
          <>
            {/* Seen from below, the left (red) wing of a plane flying right is the one farther away: higher up. */}
            <i className="light port" />
            <i className="light starboard" />
            <i className="light strobe" style={{ animationDelay: `${strobeDelay(plane.hex)}s` }} />
          </>
        ) : (
          <svg className="silhouette" viewBox="0 0 24 24">
            <path d="M22.5 12c0 .7-.6 1.1-1.4 1.1h-6l-4.8 8.4H8.2l2.6-8.4H5.4l-2 2.6H1.5L2.5 12l-1-3.7h1.9l2 2.6h5.4L8.2 2.5h2.1l4.8 8.4h6c.8 0 1.4.4 1.4 1.1z" />
          </svg>
        )}
      </div>
      <span className="craft-label">{label(plane)}</span>
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
