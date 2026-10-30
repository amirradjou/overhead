"""Builds web/lib/scenery/toronto-harbour.json from OpenStreetMap extracts; see fetch.sh.

Usage: python3 build.py <dir with raw.json, water.json, lake_rel.json> <out.json>
"""
import json, math, os, sys

SRC, OUT = sys.argv[1], sys.argv[2]
raw = json.load(open(os.path.join(SRC, "raw.json")))["elements"]
water = json.load(open(os.path.join(SRC, "water.json")))["elements"]
rel = json.load(open(os.path.join(SRC, "lake_rel.json")))["elements"][0]
roles = {m["ref"]: m["role"] for m in rel["members"] if m["type"] == "way"}
lake_ways = {e["id"]: [(p["lat"], p["lon"]) for p in e["geometry"]] for e in water if e["type"] == "way" and e["id"] in roles}

def stitch(lines):
    """Joins polylines that share end points; returns closed rings and the open lines left."""
    lines = [list(l) for l in lines]
    rings, open_ = [], []
    while lines:
        cur = lines.pop()
        grown = True
        while grown and cur[0] != cur[-1]:
            grown = False
            for i, l in enumerate(lines):
                if l[0] == cur[-1]: cur += l[1:]
                elif l[-1] == cur[-1]: cur += l[::-1][1:]
                elif l[-1] == cur[0]: cur = l + cur[1:]
                elif l[0] == cur[0]: cur = l[::-1] + cur[1:]
                else: continue
                lines.pop(i); grown = True; break
        (rings if cur[0] == cur[-1] else open_).append(cur)
    return rings, open_

LAT0 = 43.63
def xy(p): return ((p[1] + 79.39) * 111320 * math.cos(math.radians(LAT0)), (p[0] - LAT0) * 110540)

def simplify(pts, tol):
    """Douglas-Peucker in metres."""
    if len(pts) < 3: return pts
    a, b = xy(pts[0]), xy(pts[-1])
    dx, dy = b[0] - a[0], b[1] - a[1]
    L = math.hypot(dx, dy)
    best, idx = 0, 0
    for i in range(1, len(pts) - 1):
        p = xy(pts[i])
        d = abs(dy * p[0] - dx * p[1] + b[0] * a[1] - b[1] * a[0]) / L if L else math.hypot(p[0] - a[0], p[1] - a[1])
        if d > best: best, idx = d, i
    if best <= tol: return [pts[0], pts[-1]]
    return simplify(pts[: idx + 1], tol)[:-1] + simplify(pts[idx:], tol)

def rnd(pts): return [[round(p[0], 5), round(p[1], 5)] for p in pts]
def area(ring):
    s = 0
    for p, q in zip(ring, ring[1:]):
        (x1, y1), (x2, y2) = xy(p), xy(q)
        s += x1 * y2 - x2 * y1
    return abs(s) / 2

inner = [w for i, w in lake_ways.items() if roles[i] == "inner"]
outer = [w for i, w in lake_ways.items() if roles[i] == "outer"]
islands, loose = stitch(inner)
print(f"islands: {len(islands)} rings, {len(loose)} open pieces left", file=sys.stderr)
islands = [rnd(simplify(r, 2.0)) for r in islands if area(r) > 300]
shore_rings, shore = stitch(outer)
shore = max(shore, key=len)
# Land is north of the shore: close the line through points well inland.
if shore[0][1] > shore[-1][1]: shore = shore[::-1]  # west to east
mainland = shore + [(43.70, shore[-1][1]), (43.70, shore[0][1]), shore[0]]
mainland = rnd(simplify(mainland, 2.0))

def tags(e): return e.get("tags", {})
def geom(e): return [(p["lat"], p["lon"]) for p in e["geometry"]]
runways = [{"ref": tags(e)["ref"], "width": float(tags(e).get("width", 30)), "line": rnd(geom(e))}
           for e in raw if tags(e).get("aeroway") == "runway"]
aprons = [rnd(simplify(geom(e), 1.5)) for e in raw if tags(e).get("aeroway") == "apron"]
piers = []
for e in raw:
    mm = tags(e).get("man_made")
    if mm in ("pier", "breakwater", "groyne"):
        g = geom(e)
        piers.append({"kind": mm, "area": g[0] == g[-1] and len(g) > 3, "pts": rnd(simplify(g, 1.0))})

def height(t):
    for k in ("height", "building:height"):
        try: return float(str(t[k]).replace("m", "").strip())
        except (KeyError, ValueError): pass
    try: return float(t["building:levels"]) * 3.2 + 2
    except (KeyError, ValueError): return 9.0

def inside(g, s, n, w, e): return all(s <= p[0] <= n and w <= p[1] <= e for p in g)
buildings = []
for e in raw:
    t = tags(e)
    if "building" not in t or e["type"] != "way": continue
    g = geom(e)
    if g[0] != g[-1] or len(g) < 4: continue
    harbourfront = inside(g, 43.6300, 43.6420, -79.4050, -79.3700)
    island = inside(g, 43.6100, 43.6300, -79.4050, -79.3450)
    if not (harbourfront or island) or area(g) < 40: continue
    buildings.append({"h": round(height(t), 1), "ring": rnd(simplify(g, 1.0))})

out = {
    "about": "Toronto harbour from OpenStreetMap (© OpenStreetMap contributors, ODbL), fetched 2026-10-30: the "
             "lake's shores (islands are its inner rings, the mainland its outer edge closed inland), Billy Bishop's "
             "runways and aprons, piers and breakwaters, and the buildings of the harbour front and the islands.",
    "lake_m": 74.5, "ground_m": 76.5,
    "islands": islands, "mainland": mainland, "runways": runways, "aprons": aprons,
    "piers": piers, "buildings": buildings,
}
with open(OUT, "w") as f:
    json.dump(out, f, separators=(",", ":"))
    f.write("\n")
n = sum(len(r) for r in islands) + len(mainland) + sum(len(b["ring"]) for b in buildings) + sum(len(p["pts"]) for p in piers)
print(f"{len(islands)} islands, mainland {len(mainland)} pts, {len(runways)} runways, {len(aprons)} aprons, "
      f"{len(piers)} piers, {len(buildings)} buildings, {n} points", file=sys.stderr)
