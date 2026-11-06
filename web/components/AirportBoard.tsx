import { airportDay, type BoardRow } from "@/lib/airport";
import type { FollowedAirport } from "@/lib/types";

// The followed airport's board, like the one in its hall but only for what is moving now: landing, taking off,
// taxiing, with where each flight comes from or goes and when. It sits on the sky, under the small overview.
export default function AirportBoard({ airport, rows }: { airport: FollowedAirport; rows: BoardRow[] }) {
  const day = airportDay(airport).replace(/^Runway \S+ in use\.\s*/, "");
  return (
    <section className="board">
      <p className="board-title">
        {airport.name}
        {airport.runway_in_use && <span className="board-runway">runway {airport.runway_in_use}</span>}
      </p>
      {rows.length ? (
        <table>
          <tbody>
            {rows.map((r) => (
              <tr key={r.hex}>
                <td className="board-arrow">{r.arrow}</td>
                <td className="board-flight">{r.flight}</td>
                <td className="board-place">{r.place}</td>
                <td className="board-status">{r.status}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : (
        <p className="board-empty">Nothing moving right now</p>
      )}
      {day && <p className="board-day">{day}</p>}
    </section>
  );
}
