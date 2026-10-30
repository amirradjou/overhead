#!/usr/bin/env node
// bright-stars: writes lib/bright-stars.json, the stars bright enough to see from a city window, from the Yale
// Bright Star Catalogue, 5th revised edition (Hoffleit & Warren 1991), as NASA's HEASARC publishes it:
//
//   curl -O https://heasarc.gsfc.nasa.gov/FTP/heasarc/dbase/tdat_files/heasarc_bsc5p.tdat.gz
//   node scripts/bright-stars.mjs heasarc_bsc5p.tdat.gz > lib/bright-stars.json
//
// Each star is [right ascension°, declination° (J2000), V magnitude, B−V colour]. Positions are rounded to 0.01°, far
// finer than a pixel on the screen; precession since J2000 (about 0.36°) is left out for the same reason.
import { readFileSync } from "node:fs";
import { gunzipSync } from "node:zlib";

const limit = 4.5; // a dark suburban sky shows about magnitude 5; Toronto's glow leaves less

const raw = readFileSync(process.argv[2]);
const text = (process.argv[2].endsWith(".gz") ? gunzipSync(raw) : raw).toString("latin1");
const fields = text
  .match(/^line\[1\] = (.*)$/m)[1]
  .trim()
  .split(/\s+/);
const col = (name) => fields.indexOf(name);
const [ra, dec, vmag, bv] = [col("ra"), col("dec"), col("vmag"), col("bv_color")];

const stars = [];
for (const line of text.slice(text.indexOf("<DATA>") + 6, text.indexOf("<END>")).split("\n")) {
  const f = line.split("|");
  if (f.length < fields.length || f[vmag] === "") continue;
  const mag = Number(f[vmag]);
  if (!(mag <= limit)) continue;
  stars.push([
    Number(Number(f[ra]).toFixed(2)),
    Number(Number(f[dec]).toFixed(2)),
    Number(mag.toFixed(1)),
    f[bv] === "" ? 0.6 : Number(Number(f[bv]).toFixed(1)),
  ]);
}
stars.sort((a, b) => a[2] - b[2]); // brightest first
process.stdout.write("[\n" + stars.map((s) => JSON.stringify(s)).join(",\n") + "\n]\n");
console.error(`${stars.length} stars to magnitude ${limit}`);
