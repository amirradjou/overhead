from datetime import timedelta
from pathlib import Path

from helpers import HOME, T0, event, offset, write_batch

from overhead_stats import connect
from overhead_stats.facts import Window, facts

TZ = "America/Toronto"
SOUTH = Window(azimuth_deg=180, fov_deg=90, min_elevation_deg=5)


def history(root: Path, planes: list[tuple]) -> None:
    """planes: (hex, start, bearing, km, alt_ft, extra), each written as three positions a minute apart."""
    seq = 0
    for hex_, start, bearing, km, alt, extra in planes:
        events = []
        for i in range(3):
            seq += 1
            lat, lon = offset(bearing, km)
            events.append(event(seq, hex_, start + timedelta(minutes=i), lat, alt, lon=lon, **extra))
        write_batch(root, events)


def test_one_evening_says_only_what_it_can(tmp_path: Path) -> None:
    history(
        tmp_path,
        [
            ("aaaaaa", T0, 180, 15, 10000, {"type_code": "A320"}),  # south, 10,000 ft: in the window
            ("bbbbbb", T0, 0, 15, 10000, {"type_code": "B38M"}),  # north: behind the house
            ("cccccc", T0, 180, 55, 10000, {}),  # south but too far to call in view
        ],
    )
    got = facts(connect(tmp_path, *HOME), T0 + timedelta(minutes=5), TZ, SOUTH)
    # An hour of one day: no busiest hour, no first-timers, no rarest type yet.
    assert got == ["One plane crossed your window today."]

    all_round = facts(connect(tmp_path, *HOME), T0 + timedelta(minutes=5), TZ, Window())
    assert all_round == ["2 planes crossed your window today."]


def test_a_few_days_of_history(tmp_path: Path) -> None:
    day = timedelta(days=1)
    planes = []
    # Four days of the regular C-GKWA, an A220, morning and evening.
    for d in (3, 2, 1, 0):
        for h in (0, 5):
            planes.append(
                (
                    "c0ffee",
                    T0 - d * day + timedelta(hours=h - 6),
                    180,
                    15,
                    9000,
                    {"type_code": "BCS3", "registration": "C-GKWA"},
                )
            )
    # A one-off An-124 two days ago, and two newcomers today in different hours.
    planes.append(("a12400", T0 - 2 * day, 180, 20, 12000, {"type_code": "A124"}))
    planes.append(("new001", T0 - timedelta(hours=1), 0, 10, 8000, {"type_code": "B38M"}))
    planes.append(("new002", T0 - timedelta(hours=3), 0, 10, 8000, {"type_code": "B38M"}))
    planes.append(("new003", T0 - timedelta(hours=3, minutes=-10), 0, 10, 8000, {"type_code": "B38M"}))
    history(tmp_path, planes)

    got = facts(connect(tmp_path, *HOME), T0, TZ, SOUTH)
    assert "3 aircraft passed by for the first time today." in got
    assert "The rarest type this month is the A124, seen once." in got
    assert "C-GKWA is this month's regular: 8 visits." in got
    # Today in Toronto has passes at 08:00, 11:00, 11:10 and 13:00 (twice): two hours tie on 2, the earlier wins.
    assert "The busiest hour today was 11:00 to 12:00, with 2 aircraft." in got
    assert got[0] == "One plane crossed your window today."


def test_cli_writes_facts_for_the_live_api(tmp_path: Path) -> None:
    import json

    from overhead_stats.cli import main

    history(tmp_path / "h", [("aaaaaa", T0, 180, 15, 10000, {})])
    out = tmp_path / "facts.json"
    args = ["facts", "--history", str(tmp_path / "h"), "--out", str(out), "--lat", str(HOME[0]), "--lon", str(HOME[1])]
    assert main(args) == 0
    doc = json.loads(out.read_text())
    assert "generated" in doc and isinstance(doc["facts"], list)


def test_an_evening_across_midnight_is_not_two_days(tmp_path: Path) -> None:
    # 23:30 to 00:20 in Toronto: two dates, fifty minutes of history. Nothing is "the first time" yet.
    late = T0.replace(hour=3, minute=30)  # 23:30 EDT the evening before
    history(
        tmp_path, [("aaaaaa", late, 180, 15, 10000, {}), ("bbbbbb", late + timedelta(minutes=45), 180, 15, 10000, {})]
    )
    got = facts(connect(tmp_path, *HOME), late + timedelta(minutes=50), TZ, SOUTH)
    assert not any("first time" in f for f in got), got
