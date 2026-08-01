// Re-labels the 171 curated builtin patterns so a local model can choose between
// them, and so the catalog stops dominating the system prompt.
//
// The imported labels were unusable for selection. Every part of a source script
// carried an identical name, description and tag set -- "Blowjob · Finish ·
// Finisher · Fast · v2 · 10.0s · Part 1/7" through "Part 7/7" -- while the parts
// differ in measured intensity by 2.04x at the median and up to 12.45x. The model
// saw seven indistinguishable options that feel nothing alike.
//
// Span is 100% of the range at every percentile, so depth carries no information
// here and the labels do not mention it. Intensity (mean stroke speed) and
// cadence are the axes that actually vary, so those are what the labels carry.

const fs = require("fs");
const path = require("path");
const DIR = path.resolve(__dirname, "..", "internal", "motion", "builtinpatterns", "curated");
const CATALOG_PATH = path.join(DIR, "_catalog.json");
const APPLY = process.env.APPLY === "1";

// Bands span the whole catalog, not just the curated import, so a label means the
// same thing next to the designed patterns (Drift 57, Stroke 182, Hard and
// Regular 434) as it does here.
const BANDS = [
  { name: "Gentle", max: 90, blurb: "gentle" },
  { name: "Easy", max: 150, blurb: "easy" },
  { name: "Steady", max: 240, blurb: "steady" },
  { name: "Fast", max: 380, blurb: "fast" },
  { name: "Intense", max: Infinity, blurb: "intense" },
];

const band = (mean) => BANDS.find((b) => mean < b.max);

function measure(doc) {
  const pts = doc.points;
  let travel = 0, strokes = 0;
  const speeds = [];
  for (let i = 1; i < pts.length; i++) {
    const dp = Math.abs(pts[i].position_percent - pts[i - 1].position_percent);
    const dt = pts[i].time_ms - pts[i - 1].time_ms;
    if (dp > 0 && dt > 0) { travel += dp; strokes++; speeds.push((dp / dt) * 1000); }
  }
  const seconds = doc.cycle_ms / 1000;
  const mu = speeds.reduce((a, b) => a + b, 0) / speeds.length;
  const cv = Math.sqrt(speeds.reduce((a, b) => a + (b - mu) ** 2, 0) / speeds.length) / mu;
  return { mean: travel / seconds, spm: (strokes / seconds) * 60, cv };
}

// Rhythm word from how much stroke speed varies across the clip. This is the one
// thing besides pace that a listener would actually name.
function rhythm(cv) {
  if (cv < 0.35) return { word: "Drive", blurb: "even" };
  if (cv < 0.70) return { word: "Roll", blurb: "rolling" };
  return { word: "Surge", blurb: "surging" };
}

const files = fs.readdirSync(DIR).filter((f) => f.endsWith(".mhpattern.json"));
const entries = files.map((file) => {
  const full = path.join(DIR, file);
  const doc = JSON.parse(fs.readFileSync(full, "utf8"));
  const m = measure(doc);
  return { file, full, doc, ...m, band: band(m.mean), rhythm: rhythm(m.cv) };
});

// Order inside a band by intensity so the numbering is meaningful rather than
// alphabetical by source filename.
entries.sort((a, b) => a.mean - b.mean);
const counters = {};
for (const e of entries) {
  const key = `${e.band.name} ${e.rhythm.word}`;
  counters[key] = (counters[key] || 0) + 1;
  e.index = counters[key];
}
for (const e of entries) {
  const key = `${e.band.name} ${e.rhythm.word}`;
  const total = counters[key];
  e.newName = total > 1 ? `${key} ${e.index}` : key;
  e.newId = "curated-" + e.newName.toLowerCase().replace(/\s+/g, "-");
  // The name already carries pace and rhythm, and every curated clip uses the
  // full range, so the description carries only cadence -- the one axis left.
  // Repeating the name in prose would cost ~9 KB of prompt for no information.
  e.newDesc = e.spm < 90 ? "Slow cadence." : e.spm < 220 ? "Medium cadence." : "Quick cadence.";
}

const sum = (f) => entries.reduce((s, e) => s + f(e), 0);
console.log(`${entries.length} curated patterns\n`);
console.log("band distribution:");
for (const b of BANDS) {
  const inBand = entries.filter((e) => e.band.name === b.name);
  if (!inBand.length) continue;
  const ms = inBand.map((e) => e.mean);
  console.log(`  ${b.name.padEnd(8)} ${String(inBand.length).padStart(3)} patterns  ` +
    `${Math.min(...ms).toFixed(0)}-${Math.max(...ms).toFixed(0)} %/s`);
}
console.log("\nrhythm distribution:");
for (const w of ["Drive", "Roll", "Surge"]) {
  console.log(`  ${w.padEnd(6)} ${entries.filter((e) => e.rhythm.word === w).length}`);
}

const before = { id: sum((e) => ("curated-" + e.file.replace(".mhpattern.json", "")).length),
  name: sum((e) => e.doc.name.length), desc: sum((e) => (e.doc.description || "").length) };
const after = { id: sum((e) => e.newId.length), name: sum((e) => e.newName.length), desc: sum((e) => e.newDesc.length) };
console.log(`\n            ids     names   descriptions   total`);
console.log(`before   ${String(before.id).padStart(6)}  ${String(before.name).padStart(6)}  ${String(before.desc).padStart(12)}  ${String(before.id + before.name + before.desc).padStart(6)} B`);
console.log(`after    ${String(after.id).padStart(6)}  ${String(after.name).padStart(6)}  ${String(after.desc).padStart(12)}  ${String(after.id + after.name + after.desc).padStart(6)} B`);
const saved = (before.id + before.name + before.desc) - (after.id + after.name + after.desc);
console.log(`saved    ${saved} B of catalog payload`);

console.log("\nsample (evenly spaced by intensity):");
for (let i = 0; i < entries.length; i += Math.floor(entries.length / 12)) {
  const e = entries[i];
  console.log(`  ${e.mean.toFixed(0).padStart(3)} %/s  ${e.spm.toFixed(0).padStart(3)} spm  cv=${e.cv.toFixed(2)}  ` +
    `${e.newName.padEnd(18)} ${e.newDesc}`);
}

const ids = new Set(entries.map((e) => e.newId));
if (ids.size !== entries.length) {
  console.log(`\nERROR: ${entries.length - ids.size} duplicate ids`);
  process.exit(1);
}
console.log(`\nall ${ids.size} ids unique`);

if (APPLY) {
  const catalogEntries = [];
  for (const e of entries) {
    e.doc.name = e.newName;
    e.doc.description = e.newDesc;
    fs.writeFileSync(e.full, JSON.stringify(e.doc, null, 2) + "\n");
    const target = path.join(DIR, e.newId.replace(/^curated-/, "") + ".mhpattern.json");
    if (target !== e.full) { fs.renameSync(e.full, target); }
    catalogEntries.push({
      file: path.basename(target),
      name: e.newName,
    });
  }
  catalogEntries.sort((a, b) => a.file.localeCompare(b.file));
  const catalog = {
    schema: "magichandy.generated-pattern-catalog.v3",
    status_policy: "runtime-budget-audit",
    normal_speed_controls: true,
    reason: "Generated clips remain available; problematic curves are experimental, unsafe source timing is resampled, and every curve passes normal catalog budgets without a bulk exemption.",
    pattern_count: catalogEntries.length,
    patterns: catalogEntries,
  };
  fs.writeFileSync(CATALOG_PATH, JSON.stringify(catalog, null, 2) + "\n");
  console.log("applied: rewrote labels, renamed files, and synchronized the generated catalog");
}
