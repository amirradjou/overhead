"""Passes and facts from overhead's history.

The history writer keeps every position event as gzipped JSON lines under
positions/dt=YYYY-MM-DD/hour=HH/. This package reads them with DuckDB, stitches positions into passes (one row per
aircraft per pass round home) and, in M4, turns passes into facts for the screen.
"""

from importlib import resources
from pathlib import Path

import duckdb
import pandas as pd

# The columns of a position event (internal/events: Position). Declared rather than inferred: fields the writer
# omits when false or empty, such as military, would otherwise be missing whenever no row in the files has them.
POSITION_COLUMNS = {
    "run": "VARCHAR",
    "seq": "BIGINT",
    "published": "TIMESTAMPTZ",
    "hex": "VARCHAR",
    "callsign": "VARCHAR",
    "registration": "VARCHAR",
    "type_code": "VARCHAR",
    "category": "VARCHAR",
    "squawk": "VARCHAR",
    "emergency": "VARCHAR",
    "lat": "DOUBLE",
    "lon": "DOUBLE",
    "alt_ft": "BIGINT",
    "on_ground": "BOOLEAN",
    "speed_kt": "DOUBLE",
    "track_deg": "DOUBLE",
    "vert_rate_fpm": "BIGINT",
    "military": "BOOLEAN",
    "private": "BOOLEAN",
    "source": "VARCHAR",
    "source_type": "VARCHAR",
    "at": "TIMESTAMPTZ",
}


def connect(history: Path, home_lat: float, home_lon: float) -> duckdb.DuckDBPyConnection:
    """An in-memory DuckDB with a `positions` view over the history under `history`.

    Each event appears once even if a batch was written twice: rows are unique by run and sequence number.
    """
    con = duckdb.connect()
    con.execute("SET TimeZone = 'UTC'")
    con.execute("SET VARIABLE home_lat = ?", [home_lat])
    con.execute("SET VARIABLE home_lon = ?", [home_lon])
    # A view can't take a bound parameter, so the path goes in as a quoted SQL string.
    pattern = _literal(str(history / "positions" / "*" / "*" / "*.jsonl.gz"))
    columns = ", ".join(f"{_literal(name)}: {_literal(kind)}" for name, kind in POSITION_COLUMNS.items())
    con.execute(
        f"""
        CREATE VIEW positions AS
        SELECT * FROM read_json({pattern}, format = 'newline_delimited', hive_partitioning = true,
                                columns = {{{columns}}})
        QUALIFY row_number() OVER (PARTITION BY run, seq ORDER BY published) = 1
        """
    )
    return con


def _literal(s: str) -> str:
    return "'" + s.replace("'", "''") + "'"


def passes_sql() -> str:
    """The pass-stitching query (passes.sql); it reads the positions view that connect makes."""
    return resources.files(__package__).joinpath("passes.sql").read_text()


def passes(con: duckdb.DuckDBPyConnection) -> pd.DataFrame:
    """One row per pass round home."""
    return con.sql(passes_sql()).df()
