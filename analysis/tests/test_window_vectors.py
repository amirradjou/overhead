"""The facts' SQL must agree with the live API about what the window shows: both read the same vectors
(internal/live/testdata/window_vectors.json)."""

import json
from datetime import UTC, datetime
from pathlib import Path

import pytest
from helpers import write_batch

from overhead_stats import connect
from overhead_stats.facts import Window, _prepare

VECTORS = json.loads((Path(__file__).parents[2] / "internal/live/testdata/window_vectors.json").read_text())
AT = datetime(2026, 10, 30, 18, 0, tzinfo=UTC)


def test_vectors_loaded() -> None:
    assert len(VECTORS["cases"]) >= 10


@pytest.mark.parametrize("case", VECTORS["cases"], ids=lambda c: c["name"])
def test_in_view_matches_the_live_api(tmp_path: Path, case: dict) -> None:
    event = {
        "run": "20261030T180000Z-abc123",
        "seq": 1,
        "published": AT.isoformat(),
        "hex": "abc123",
        "lat": case["lat"],
        "lon": case["lon"],
        "alt_ft": case["alt_ft"],
        "source": "test",
        "at": AT.isoformat(),
    }
    write_batch(tmp_path, [event])
    con = connect(tmp_path, VECTORS["home"]["lat"], VECTORS["home"]["lon"])
    _prepare(con, "America/Toronto", Window(**case["window"]))
    (in_view,) = con.execute("SELECT in_view FROM positions_seen").fetchone()
    assert in_view == case["in_view"]
