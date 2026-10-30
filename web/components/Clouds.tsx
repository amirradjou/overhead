"use client";

import { memo, useMemo } from "react";
import { cloudscape } from "@/lib/clouds";
import { roofline, screenTop } from "@/lib/sky";
import { mix } from "@/lib/skyColour";
import type { Weather, Window } from "@/lib/types";

interface Props {
  weather: Weather;
  w: Window;
  sunAlt: number;
  /** Where the sky ends: the horizon (from high up) or the roofline, as a fraction of the screen's height. */
  horizonY: number;
}

const rad = Math.PI / 180;

// Clouds is the sky's weather: cumulus, altocumulus and cirrus as the reading reports them, or an overcast deck,
// with rain or snow over the whole view. The layer drifts a little with the wind, back and forth over a quarter of
// an hour, as a transform the compositor moves; it is drawn again only when the reading or the light changes.
function Clouds({ weather, w, sunAlt, horizonY }: Props) {
  const sunStep = Math.round(sunAlt);
  const top = screenTop(w);
  const scape = useMemo(() => {
    const span = top - w.min_elevation_deg;
    const at = (off: number, el: number): [number, number] => [
      0.5 + off / w.fov_deg,
      roofline * (1 - (el - w.min_elevation_deg) / span),
    ];
    return cloudscape(weather, sunStep, w.fov_deg, top, w.home_elevation_m, at);
  }, [weather, sunStep, w, top]);
  // Clouds move with the wind: blowing towards the window's left or right as seen from it.
  const towards = weather.wind_from_deg + 180;
  const across = Math.sin((towards - w.azimuth_deg) * rad);
  const drift = Math.max(-1, Math.min(1, across)) * Math.min(2.5, 0.4 + weather.wind_kmh / 15);
  const hy = horizonY * 1000;
  const rain = (weather.code >= 51 && weather.code <= 67) || (weather.code >= 80 && weather.code <= 82);
  const snow = (weather.code >= 71 && weather.code <= 77) || weather.code === 85 || weather.code === 86;
  const k = (v: number) => (v * 1000).toFixed(1);
  return (
    <div className="clouds" style={{ "--drift": `${drift.toFixed(2)}vw` } as React.CSSProperties}>
      <svg viewBox="0 0 1000 1000" preserveAspectRatio="none" aria-hidden="true">
        <defs>
          <radialGradient id="puff" fx="0.5" fy="0.3">
            {/* Even through the middle and fading at the rim, so overlapping puffs merge into one soft heap. */}
            <stop offset="0" stopColor={scape.lit} stopOpacity="0.82" />
            <stop offset="0.6" stopColor={scape.lit} stopOpacity="0.7" />
            <stop offset="0.86" stopColor={mix(scape.lit, scape.shade, 0.2)} stopOpacity="0.3" />
            <stop offset="1" stopColor={mix(scape.lit, scape.shade, 0.3)} stopOpacity="0" />
          </radialGradient>
          <radialGradient id="puff-low" fx="0.5" fy="0.4">
            <stop offset="0" stopColor={mix(scape.lit, scape.shade, 0.35)} stopOpacity="0.78" />
            <stop offset="0.6" stopColor={mix(scape.lit, scape.shade, 0.45)} stopOpacity="0.62" />
            <stop offset="1" stopColor={mix(scape.lit, scape.shade, 0.55)} stopOpacity="0" />
          </radialGradient>
          <radialGradient id="puff-base">
            <stop offset="0" stopColor={scape.shade} stopOpacity="0.7" />
            <stop offset="1" stopColor={scape.shade} stopOpacity="0" />
          </radialGradient>
          <radialGradient id="streak">
            <stop offset="0" stopColor={scape.lit} stopOpacity="0.32" />
            <stop offset="1" stopColor={scape.lit} stopOpacity="0" />
          </radialGradient>
          {/* A slight blur melts the puffs into one another. It is painted once per reading, then only moved. */}
          <filter id="soften" x="-5%" y="-5%" width="110%" height="110%">
            <feGaussianBlur stdDeviation="1.6 2.8" />
          </filter>
          <linearGradient id="deck" x1="0" y1="0" x2="0" y2={hy} gradientUnits="userSpaceOnUse">
            <stop offset="0" stopColor={scape.shade} />
            <stop offset="0.7" stopColor={mix(scape.shade, scape.lit, 0.35)} />
            <stop offset="1" stopColor={mix(scape.shade, scape.lit, 0.6)} />
          </linearGradient>
          {rain && (
            <pattern
              id="rain"
              width="14"
              height="40"
              patternUnits="userSpaceOnUse"
              patternTransform="rotate(12)"
            >
              <rect x="6" y="0" width="0.6" height="16" fill={scape.lit} opacity="0.35" />
            </pattern>
          )}
          {snow && (
            <pattern id="snow" width="22" height="26" patternUnits="userSpaceOnUse">
              <circle cx="4" cy="6" r="0.9" fill="#ffffff" opacity="0.55" />
              <circle cx="15" cy="18" r="0.7" fill="#ffffff" opacity="0.45" />
            </pattern>
          )}
        </defs>
        <g opacity={scape.opacity} filter="url(#soften)">
          {scape.streaks.map((s, i) => (
            <ellipse
              key={`s${i}`}
              cx={k(s.cx)}
              cy={k(s.cy)}
              rx={k(s.rx)}
              ry={k(s.ry)}
              fill="url(#streak)"
              transform={`rotate(${(s.tilt ?? 0).toFixed(1)} ${k(s.cx)} ${k(s.cy)})`}
            />
          ))}
          {scape.deck > 0 && (
            <rect x="-40" y="-20" width="1080" height={hy + 20} fill="url(#deck)" opacity={scape.deck} />
          )}
          {scape.puffs.map((p, i) => (
            <ellipse
              key={i}
              cx={k(p.cx)}
              cy={k(p.cy)}
              rx={k(p.rx)}
              ry={k(p.ry)}
              fill={p.base ? "url(#puff-base)" : p.low ? "url(#puff-low)" : "url(#puff)"}
            />
          ))}
        </g>
        {rain && <rect x="-40" y="0" width="1080" height="1000" fill="url(#rain)" opacity="0.6" />}
        {snow && <rect x="-40" y="0" width="1080" height="1000" fill="url(#snow)" />}
      </svg>
    </div>
  );
}

export default memo(Clouds);
