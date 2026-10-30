#!/usr/bin/env bash
# Rebuilds web/lib/scenery/toronto-harbour.json from OpenStreetMap (© OpenStreetMap contributors, ODbL) through the
# Overpass API: the harbour's piers, runways and buildings, Lake Ontario's shore ways round the harbour, and the
# lake relation's member roles (inner rings are islands). Run from web/: scripts/scenery/fetch.sh
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
api=${OVERPASS:-https://overpass-api.de/api/interpreter}
ask() { curl -sf -A "overhead (personal project; scenery for a home screen)" --max-time 180 --data-urlencode "data$1" "$api" -o "$2"; }
ask "@$here/harbour.overpassql" "$work/raw.json"
ask "@$here/lake.overpassql" "$work/water.json"
ask '=[out:json][timeout:90];rel(1206310);out body;' "$work/lake_rel.json"
python3 "$here/build.py" "$work" "$here/../../lib/scenery/toronto-harbour.json"
