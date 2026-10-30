"""overhead-stats: passes and facts from the history, for people (passes) and for the screen (facts)."""

import argparse
import json
import os
import sys
import time
from datetime import UTC, datetime
from pathlib import Path

from overhead_stats import connect, passes
from overhead_stats.facts import Window, facts


def _env_float(name: str, default: float) -> float:
    value = os.environ.get(name, "").strip()
    return float(value) if value else default


def main(argv: list[str] | None = None) -> int:
    data = Path(os.environ.get("OVERHEAD_DATA", "data"))
    p = argparse.ArgumentParser(prog="overhead-stats", description=__doc__)
    p.add_argument("command", choices=["passes", "facts"])
    p.add_argument("--history", type=Path, default=data / "history", help="the history writer's directory")
    p.add_argument("--lat", type=float, default=_env_float("OVERHEAD_LAT", 43.6777))
    p.add_argument("--lon", type=float, default=_env_float("OVERHEAD_LON", -79.6248))
    p.add_argument("--tz", default=os.environ.get("OVERHEAD_TZ", "America/Toronto"), help="for days and hours")
    p.add_argument("--window-az", type=float, default=_env_float("OVERHEAD_WINDOW_AZ", 0))
    p.add_argument("--window-fov", type=float, default=_env_float("OVERHEAD_WINDOW_FOV", 360))
    p.add_argument("--window-min-el", type=float, default=_env_float("OVERHEAD_WINDOW_MIN_EL", 5))
    p.add_argument("--window-max-el", type=float, default=_env_float("OVERHEAD_WINDOW_MAX_EL", 90))
    p.add_argument("--home-elev", type=float, default=_env_float("OVERHEAD_HOME_ELEV", 0))
    p.add_argument("--out", type=Path, default=data / "facts.json", help="facts: where the live API reads them")
    p.add_argument("--every", type=float, default=0, help="facts: rewrite every this many minutes (0: once)")
    args = p.parse_args(argv)

    if args.command == "passes":
        df = passes(connect(args.history, args.lat, args.lon))
        df.to_csv(sys.stdout, index=False)
        return 0

    window = Window(
        azimuth_deg=args.window_az,
        fov_deg=args.window_fov,
        min_elevation_deg=args.window_min_el,
        max_elevation_deg=args.window_max_el,
        home_elevation_m=args.home_elev,
    )
    while True:
        now = datetime.now(UTC)
        try:
            # No history yet (a fresh install) is no facts, not an error.
            found = (
                facts(connect(args.history, args.lat, args.lon), now, args.tz, window)
                if has_history(args.history)
                else []
            )
            _write(args.out, {"generated": now.isoformat(), "facts": found})
            print(f"{now:%H:%M:%S} wrote {len(found)} facts to {args.out}", file=sys.stderr)
        except Exception as e:  # one bad run must not stop a job that rewrites every few minutes
            if args.every <= 0:
                raise
            print(f"{now:%H:%M:%S} facts failed, will try again: {e}", file=sys.stderr)
        if args.every <= 0:
            return 0
        time.sleep(args.every * 60)


def has_history(history: Path) -> bool:
    """Whether the history writer has stored any batch yet."""
    return any(history.glob("positions/*/*/*.jsonl.gz"))


def _write(path: Path, doc: dict) -> None:
    """Writes atomically, so the live API never reads half a file."""
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_suffix(".tmp")
    tmp.write_text(json.dumps(doc, indent=2) + "\n")
    tmp.replace(path)


if __name__ == "__main__":
    sys.exit(main())
