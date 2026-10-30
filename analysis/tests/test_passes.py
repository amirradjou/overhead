import gzip
import json
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest

from overhead_stats import connect, passes

HOME = (43.6777, -79.6248)  # Toronto Pearson
T0 = datetime(2026, 10, 30, 18, 0, tzinfo=UTC)


def write_batch(root: Path, run: str, events: list[dict]) -> None:
    """Writes events the way the history writer does: one gzipped JSON-lines file per minute."""
    first = events[0]
    minute = datetime.fromisoformat(first["published"])
    path = root / "positions" / f"dt={minute:%Y-%m-%d}" / f"hour={minute:%H}"
    path.mkdir(parents=True, exist_ok=True)
    name = f"{minute:%Y%m%dT%H%MZ}_{run}_{first['seq']:012d}.jsonl.gz"
    with gzip.open(path / name, "wt") as f:
        for e in events:
            f.write(json.dumps(e) + "\n")


def event(seq: int, hex_: str, at: datetime, lat: float, alt: int | None = 3000, **extra) -> dict:
    e = {
        "run": "20261030T180000Z-abc123",
        "seq": seq,
        "published": at.isoformat(),
        "hex": hex_,
        "lat": lat,
        "lon": HOME[1],
        "source": "adsb.fi",
        "at": at.isoformat(),
        **extra,
    }
    if alt is not None:
        e["alt_ft"] = alt
    return e


@pytest.fixture
def history(tmp_path: Path) -> Path:
    # ACA101 passes twice, 30 minutes apart; WJA9 once; a military C-17 once.
    first = [event(1, "aaaaaa", T0, 43.80, callsign="ACA101", type_code="A320", alt=5000)]
    first += [event(2, "aaaaaa", T0 + timedelta(minutes=1), 43.70, callsign="ACA101", type_code="A320", alt=3000)]
    first += [event(3, "bbbbbb", T0 + timedelta(minutes=1), 43.90, callsign="WJA9", type_code="B38M")]
    write_batch(tmp_path, "20261030T180000Z-abc123", first)
    second = [
        event(4, "aaaaaa", T0 + timedelta(minutes=31), 43.69, callsign="ACA102", alt=2500),
        event(5, "cccccc", T0 + timedelta(minutes=31), 43.75, callsign="RCH401", type_code="C17", military=True),
    ]
    write_batch(tmp_path, "20261030T180000Z-abc123", second)
    # The same first batch written again, as after a redelivery: it must not double anything.
    write_batch(tmp_path / "copy", "20261030T180000Z-abc123", first)
    (tmp_path / "positions" / "dt=2026-10-30" / "hour=18" / "dup.jsonl.gz").write_bytes(
        next((tmp_path / "copy").rglob("*.jsonl.gz")).read_bytes()
    )
    return tmp_path


def test_passes_split_on_a_ten_minute_gap(history: Path) -> None:
    df = passes(connect(history, *HOME))
    assert len(df) == 4
    aca = df[df.hex == "aaaaaa"].sort_values("pass")
    assert list(aca.callsign) == ["ACA101", "ACA102"]
    assert list(aca.positions) == [2, 1], "the duplicated batch must not double the first pass"
    first = aca.iloc[0]
    assert first.lowest_alt_ft == 3000
    assert first.type_code == "A320"
    assert first.closest_km == pytest.approx(2.5, abs=0.1)  # 43.70 is 2.5 km north of Pearson


def test_flags_and_missing_fields(history: Path) -> None:
    df = passes(connect(history, *HOME)).set_index("hex")
    assert bool(df.loc["cccccc", "military"]) is True
    assert bool(df.loc["bbbbbb", "military"]) is False
    assert bool(df.loc["bbbbbb", "private"]) is False
    assert bool(df.loc["aaaaaa"].iloc[0]["on_ground_at_some_point"]) is False
