import { bearingDeg, shown } from "@/lib/airport";
import { greatCircleKm } from "@/lib/progress";
import { overviewTurn, polar } from "@/lib/sky";
import type { View } from "@/lib/types";

// A small disc of everything around home, turned so the window's direction is at the top: what is around your
// window, with its slice of sky shaded, not a radar to read. A small N marks where north is.
export default function Overview({ view }: { view: View }) {
  const r = 50;
  const w = view.window;
  const turn = overviewTurn(w);
  const wedge = w.fov_deg >= 360 ? null : sector(-w.fov_deg / 2, w.fov_deg / 2, r);
  const north = polar(-turn, r + 7);
  // Following an airport, the disc shows the nearer sky: what lands and takes off there is within 20 km.
  const reach = view.focused ? Math.min(view.radius_km, 20) : view.radius_km;
  const ap = view.airport;
  const apAt = ap
    ? polar(bearingDeg(view.center, ap) - turn, (Math.min(greatCircleKm(view.center, ap), reach) / reach) * r)
    : null;
  return (
    <svg className="overview" viewBox="-60 -60 120 120" aria-hidden="true">
      <circle className="overview-edge" r={r} />
      {wedge && <path className="overview-window" d={wedge} />}
      <text className="overview-north" x={north.x} y={north.y}>
        N
      </text>
      {apAt && <rect className="overview-airport" x={apAt.x - 2} y={apAt.y - 2} width={4} height={4} />}
      {shown(view)
        .filter((p) => !p.on_ground)
        .map((p) => {
          const at = polar(p.bearing_deg - turn, (Math.min(p.distance_km, reach) / reach) * r);
          const cls = p.in_view ? "is-in-view" : p.coming ? "is-coming" : "";
          return <circle key={p.hex} className={`overview-plane ${cls}`} cx={at.x} cy={at.y} r={1.8} />;
        })}
      <circle className="overview-home" r={2.4} />
    </svg>
  );
}

function sector(fromDeg: number, toDeg: number, r: number): string {
  const pt = (deg: number) => {
    const p = polar(deg, r);
    return `${p.x.toFixed(2)} ${p.y.toFixed(2)}`;
  };
  const large = toDeg - fromDeg > 180 ? 1 : 0;
  return `M0 0 L${pt(fromDeg)} A${r} ${r} 0 ${large} 1 ${pt(toDeg)} Z`;
}
