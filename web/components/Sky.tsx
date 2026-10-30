"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { label } from "@/lib/format";
import { compassTicks, nearestTurn, roofline, rooftops, stars, toScreen, topDeg } from "@/lib/sky";
import type { SkyPhase } from "@/lib/sun";
import type { Plane, View, Window } from "@/lib/types";

interface Props {
  view: View | null;
  phase: SkyPhase;
  featuredHex?: string;
  next?: Plane;
}

const trailPoints = 15; // 30 s of positions at one snapshot every 2 s

// The window: sky, stars after dark, the rooftops opposite, the planes in view and a cue where the next one appears.
export default function Sky({ view, phase, featuredHex, next }: Props) {
  const roofs = useMemo(() => rooftops(), []);
  const starField = useMemo(() => stars(160), []);
  const w = view?.window;
  const visible = useMemo(() => view?.planes.filter((p) => p.in_view) ?? [], [view]);
  const trails = useTrails(visible, w);
  const size = useScreenSize();
  const dark = phase === "night" || phase === "twilight";

  return (
    <div className="sky" aria-hidden="true">
      {dark && (
        // No viewBox: positions in percent, radii in pixels, so a star is round on any screen shape.
        <svg className="stars">
          {starField.map((s, i) => (
            <circle key={i} cx={`${s.x * 100}%`} cy={`${s.y * 100}%`} r={s.r} opacity={s.a} />
          ))}
        </svg>
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
      className={`craft${featured ? " is-featured" : ""}`}
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
  const x = c.side === "left" ? 0.015 : c.side === "right" ? 0.985 : Math.min(0.95, Math.max(0.05, at.x));
  const y = c.side === "rooftops" ? roofline - 0.04 : c.side === "above" ? 0.04 : Math.max(0.06, at.y);
  return (
    <div className={`cue cue-${c.side}`} style={{ left: `${x * 100}%`, top: `${y * 100}%` }}>
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
  const dy = ((-p.motion.elevation_deg / (topDeg - w.min_elevation_deg)) * roofline * 9) / 16; // in widths, 16:9
  return (Math.atan2(dy, dx) * 180) / Math.PI;
}

// strobeDelay spreads the strobes so the planes don't flash in step.
function strobeDelay(hex: string): number {
  let h = 0;
  for (const ch of hex) h = (h * 31 + ch.charCodeAt(0)) % 997;
  return -(h % 160) / 100;
}
