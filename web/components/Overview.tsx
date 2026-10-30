import type { View } from "@/lib/types";

// A small disc of everything around home, north up, with the window's slice of sky shaded: context for what the
// window shows, not a map to read.
export default function Overview({ view }: { view: View }) {
  const r = 50;
  const w = view.window;
  const wedge =
    w.fov_deg >= 360 ? null : sector(w.azimuth_deg - w.fov_deg / 2, w.azimuth_deg + w.fov_deg / 2, r);
  return (
    <svg className="overview" viewBox="-60 -60 120 120" aria-hidden="true">
      <circle className="overview-edge" r={r} />
      {wedge && <path className="overview-window" d={wedge} />}
      <text className="overview-north" y={-r - 3}>
        N
      </text>
      {view.planes
        .filter((p) => !p.on_ground)
        .map((p) => {
          const d = (Math.min(p.distance_km, view.radius_km) / view.radius_km) * r;
          const a = (p.bearing_deg * Math.PI) / 180;
          const cls = p.in_view ? "is-in-view" : p.coming ? "is-coming" : "";
          return (
            <circle
              key={p.hex}
              className={`overview-plane ${cls}`}
              cx={d * Math.sin(a)}
              cy={-d * Math.cos(a)}
              r={1.8}
            />
          );
        })}
      <circle className="overview-home" r={2.4} />
    </svg>
  );
}

function sector(fromDeg: number, toDeg: number, r: number): string {
  const pt = (deg: number) => {
    const a = (deg * Math.PI) / 180;
    return `${(r * Math.sin(a)).toFixed(2)} ${(-r * Math.cos(a)).toFixed(2)}`;
  };
  const large = toDeg - fromDeg > 180 ? 1 : 0;
  return `M0 0 L${pt(fromDeg)} A${r} ${r} 0 ${large} 1 ${pt(toDeg)} Z`;
}
