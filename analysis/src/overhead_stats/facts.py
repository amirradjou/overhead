"""Facts for the screen to show between planes (docs/SPEC.md: Stats).

Each fact is one plain sentence, and only appears once the history can back it: "the rarest type this month" means
little after an hour of data, so it waits for a few days.
"""

from dataclasses import dataclass
from datetime import date, datetime

import duckdb

from overhead_stats import passes_sql


@dataclass(frozen=True)
class Window:
    """The view the screen draws (internal/live: Window)."""

    azimuth_deg: float = 0
    fov_deg: float = 360
    min_elevation_deg: float = 5
    home_elevation_m: float = 0
    max_elevation_deg: float = 90


# Must match internal/live: a plane farther than this isn't counted as in view.
VISIBLE_KM = 40.0


def facts(con: duckdb.DuckDBPyConnection, now: datetime, tz: str, window: Window) -> list[str]:
    """Sentences about the history in `con` (see connect), as of `now`, with days and hours in `tz`."""
    _prepare(con, tz, window)
    today: date = _scalar(con, "SELECT (?::TIMESTAMPTZ AT TIME ZONE getvariable('tz'))::DATE", [now])
    first_seen: datetime | None = _scalar(con, "SELECT min(first_seen) FROM passes")
    if first_seen is None:
        return []
    # How much history there is, not how many dates it touches: an evening that crosses midnight is not two days.
    days = (now - first_seen).total_seconds() / 86_400
    out: list[str] = []

    crossed = _scalar(con, "SELECT count(DISTINCT hex) FROM positions_seen WHERE in_view AND local_day = ?", [today])
    if crossed:
        out.append(f"{_count(crossed, 'plane')} crossed your window today.")

    busiest = con.execute(
        """
        SELECT local_hour, count(*) AS n FROM passes WHERE local_day = ?
        GROUP BY local_hour HAVING count(*) > 0 ORDER BY n DESC, local_hour LIMIT 1
        """,
        [today],
    ).fetchone()
    hours_today = _scalar(con, "SELECT count(DISTINCT local_hour) FROM passes WHERE local_day = ?", [today])
    if busiest and (hours_today or 0) >= 3:
        hour, n = busiest
        out.append(
            f"The busiest hour today was {hour:02d}:00 to {(hour + 1) % 24:02d}:00, with {_count(n, 'aircraft')}."
        )

    if days >= 2:  # "first time" needs a past to compare with
        new = _scalar(
            con,
            """
            SELECT count(*) FROM (SELECT hex, min(local_day) AS first FROM passes GROUP BY hex) WHERE first = ?
            """,
            [today],
        )
        if new:
            out.append(f"{_count(new, 'aircraft')} passed by for the first time today.")

    month_start = today.replace(day=1)
    if days >= 3:  # a month's rarest and regulars need more than a weekend
        rarest = con.execute(
            """
            SELECT type_code, count(*) AS n FROM passes
            WHERE local_day >= ? AND type_code IS NOT NULL
            GROUP BY type_code ORDER BY n, max(last_seen) DESC LIMIT 1
            """,
            [month_start],
        ).fetchone()
        if rarest:
            code, n = rarest
            seen = "once" if n == 1 else f"{n} times"
            out.append(f"The rarest type this month is the {code}, seen {seen}.")

        regular = con.execute(
            """
            SELECT registration, count(*) AS n FROM passes
            WHERE local_day >= ? AND registration IS NOT NULL AND commercial
            GROUP BY registration ORDER BY n DESC, registration LIMIT 1
            """,
            [month_start],
        ).fetchone()
        if regular and regular[1] >= 3:
            out.append(f"{regular[0]} is this month's regular: {regular[1]} visits.")
    return out


def _prepare(con: duckdb.DuckDBPyConnection, tz: str, w: Window) -> None:
    """Builds the passes table and a positions_seen view (each position placed relative to the window)."""
    con.execute("SET VARIABLE tz = ?", [tz])
    for name, value in vars(w).items():
        con.execute(f"SET VARIABLE {name} = ?", [value])
    con.execute(
        f"""
        CREATE OR REPLACE TEMP TABLE passes AS
        SELECT *, (first_seen AT TIME ZONE getvariable('tz'))::DATE AS local_day,
                  hour(first_seen AT TIME ZONE getvariable('tz')) AS local_hour,
                  -- An airline flight, as the live API's public mode has it (internal/live: Commercial): only these
                  -- may be named by registration on a screen others can see (privacy rule, docs/SPEC.md).
                  regexp_full_match(coalesce(callsign, ''), '[A-Z]{{3}}[0-9][0-9A-Z]{{0,3}}')  -- {{}}: an f-string
                      AND NOT military AND NOT private AS commercial
        FROM ({passes_sql()})
        -- Passes that were in the air at some point: aircraft parked or taxiing at an airport are not "overhead".
        WHERE lowest_alt_ft IS NOT NULL
        """
    )
    # The window's geometry, as internal/live works it out: bearing and distance from home, the angle above the
    # horizon allowing for the Earth's curve (four-thirds radius for refraction), and the offset from the window's
    # centre.
    con.execute(
        f"""
        CREATE OR REPLACE TEMP VIEW positions_seen AS
        WITH placed AS (
            SELECT hex, "at",
                2 * 6371.0088 * asin(sqrt(
                    pow(sin(radians(lat - getvariable('home_lat')) / 2), 2)
                    + cos(radians(getvariable('home_lat'))) * cos(radians(lat))
                      * pow(sin(radians(lon - getvariable('home_lon')) / 2), 2))) AS d,
                degrees(atan2(
                    sin(radians(lon - getvariable('home_lon'))) * cos(radians(lat)),
                    cos(radians(getvariable('home_lat'))) * sin(radians(lat))
                    - sin(radians(getvariable('home_lat'))) * cos(radians(lat))
                      * cos(radians(lon - getvariable('home_lon'))))) AS bearing,
                alt_ft * 0.3048 / 1000 - getvariable('home_elevation_m') / 1000 AS h
            FROM positions
            WHERE alt_ft IS NOT NULL AND NOT coalesce(on_ground, false)
        )
        , seen AS (
            SELECT *, degrees(atan2(h - d * d / (2 * 6371.0088 * 4 / 3), greatest(d, 1e-6))) AS elevation FROM placed
        )
        SELECT hex, ("at" AT TIME ZONE getvariable('tz'))::DATE AS local_day,
            elevation >= getvariable('min_elevation_deg')
            -- An unset or impossible top edge means none, as internal/live: Window.top.
            AND (elevation <= getvariable('max_elevation_deg')
                 OR getvariable('max_elevation_deg') <= getvariable('min_elevation_deg')
                 OR getvariable('max_elevation_deg') > 90)
            AND (getvariable('fov_deg') >= 360
                 OR abs(((bearing - getvariable('azimuth_deg') + 540) % 360) - 180) <= getvariable('fov_deg') / 2)
            AND d <= {VISIBLE_KM} AS in_view
        FROM seen
        """
    )


def _scalar(con: duckdb.DuckDBPyConnection, sql: str, params: list | None = None):
    row = con.execute(sql, params or []).fetchone()
    return row[0] if row else None


def _count(n: int, noun: str) -> str:
    """'One plane', '12 planes'; aircraft is its own plural."""
    if n == 1:
        return f"One {noun}"
    return f"{n} {noun if noun == 'aircraft' else noun + 's'}"
