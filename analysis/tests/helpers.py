"""Builders for test histories, written the way the history writer writes them."""

import gzip
import json
import math
from datetime import UTC, datetime
from pathlib import Path

HOME = (43.6777, -79.6248)  # Toronto Pearson
RUN = "20261030T180000Z-abc123"
T0 = datetime(2026, 10, 30, 18, 0, tzinfo=UTC)


def write_batch(root: Path, events: list[dict]) -> None:
    """One gzipped JSON-lines file, named and placed like the history writer's."""
    first = events[0]
    minute = datetime.fromisoformat(first["published"])
    path = root / "positions" / f"dt={minute:%Y-%m-%d}" / f"hour={minute:%H}"
    path.mkdir(parents=True, exist_ok=True)
    name = f"{minute:%Y%m%dT%H%MZ}_{first['run']}_{first['seq']:012d}.jsonl.gz"
    with gzip.open(path / name, "wt") as f:
        for e in events:
            f.write(json.dumps(e) + "\n")


def event(seq: int, hex_: str, at: datetime, lat: float, alt: int | None = 3000, lon: float = HOME[1], **extra) -> dict:
    e = {
        "run": RUN,
        "seq": seq,
        "published": at.isoformat(),
        "hex": hex_,
        "lat": lat,
        "lon": lon,
        "source": "adsb.fi",
        "at": at.isoformat(),
        **extra,
    }
    if alt is not None:
        e["alt_ft"] = alt
    return e


def offset(bearing_deg: float, km: float) -> tuple[float, float]:
    """The point km from home along bearing_deg (flat-Earth approximation, fine within 60 km)."""
    lat = HOME[0] + km * math.cos(math.radians(bearing_deg)) / 111.2
    lon = HOME[1] + km * math.sin(math.radians(bearing_deg)) / (111.2 * math.cos(math.radians(HOME[0])))
    return lat, lon
