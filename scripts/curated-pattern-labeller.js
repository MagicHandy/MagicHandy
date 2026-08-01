// Repairs, filters and labels the curated funscript clips that ship as builtin
// patterns. Run after adding clips to internal/motion/builtinpatterns/curated:
//
//   node scripts/curated-pattern-labeller.js            # report only
//   APPLY=1 node scripts/curated-pattern-labeller.js    # rewrite, delete, reindex
//
// Three things happen, in this order, and the order matters.
//
// EXCISION. A captured clip carries the pauses its source scene had, and those
// read on the device as stuttering or stopping. Excision removes stillness
// without touching geometry: a stroke too small to be felt is merged into its
// neighbour, and a stroke slower than the floor has its duration shortened until
// it reaches the floor. Durations only ever shrink and no turning point moves,
// so the shape that survives is the shape that was captured.
//
// FILTERING. Whatever still fails the envelope after excision is dropped. For a
// large share of these clips the dead time IS the content -- one had two live
// strokes out of twenty-two -- and there is no pattern hiding inside them.
//
// LABELLING. Bands are computed from the excised curve, so they describe live
// motion. The first pass labelled by mean stroke speed across the whole clip,
// which averages motion together with stillness: a clip that was mostly stopped
// scored a low mean and came out labelled "Gentle". Measured afterwards, 100% of
// that Gentle band contained a dead stroke against 23% of Intense. The label was
// reporting how much the clip rested while reading as how hard it worked.

const fs = require("fs");
const path = require("path");
const DIR = path.resolve(__dirname, "..", "internal", "motion", "builtinpatterns", "curated");
const CATALOG_PATH = path.join(DIR, "_catalog.json");
const APPLY = process.env.APPLY === "1";

// Mirrors the envelope enforced in internal/motion by the catalog speed test.
const FLOOR_VELOCITY = 42;    // %/s on the slowest stroke
const MIN_AMPLITUDE = 22;     // MIN_REVERSAL_GAP * FLOOR_VELOCITY / 1000
const MIN_REVERSAL_GAP = 450; // ms, catalogMinReversalGap
const MAX_RATIO = 3.3;        // fastest/slowest stroke inside one pattern
const CYCLE_FLOOR = 6600;     // RoutineCycleFloorMillis
const CYCLE_CEILING = 12000;  // asserted by TestCuratedBuiltinPatternsLoad
const ACCEL_BUDGET = 2600;    // under catalogMaxAcceleration 3000, leaving blend headroom
const MIN_TURNING_POINTS = 5; // fewer than this is a twitch, not a loop

const BANDS = [
  { name: "Gentle", max: 90 }, { name: "Easy", max: 150 },
  { name: "Steady", max: 240 }, { name: "Fast", max: 380 },
  { name: "Intense", max: Infinity },
];
const rhythmOf = (cv) => (cv < 0.35 ? "Drive" : cv < 0.7 ? "Roll" : "Surge");

// A clip as turning positions plus the duration of the stroke leaving each one.
function toStrokes(doc) {
  const pts = doc.points;
  return {
    positions: pts.slice(0, -1).map((p) => p.position_percent),
    durations: pts.slice(1).map((p, i) => p.time_ms - pts[i].time_ms),
  };
}

const amplitudeAt = (positions, i) =>
  Math.abs(positions[(i + 1) % positions.length] - positions[i]);

// Merge away any stroke too short to be felt. Deleting the turning point it
// leads into joins it to the next stroke and keeps position continuous, so the
// device never teleports; its time is carried over rather than dropped.
function mergeShortStrokes({ positions, durations }) {
  let guard = positions.length * 4;
  while (positions.length > MIN_TURNING_POINTS && guard-- > 0) {
    let index = -1;
    for (let i = 0; i < positions.length; i++) {
      if (amplitudeAt(positions, i) < MIN_AMPLITUDE) { index = i; break; }
    }
    if (index < 0) break;
    const next = (index + 1) % positions.length;
    durations[index] += durations[next];
    positions.splice(next, 1);
    durations.splice(next, 1);
  }
  return { positions, durations };
}

// Shorten anything slower than the floor until it reaches the floor. Two lower
// bounds stop the shortening: the reversal gap, and the acceleration budget.
//
// Shortening a stroke raises its acceleration -- monotone-Hermite peak runs near
// 6A/T^2 -- so excising dead time can push a broad stroke over the catalog
// budget. It did: one clip landed at 3000.4 against a 3000 ceiling. The bound
// below only binds on large amplitudes, where the reversal gap alone is not
// enough, and carries margin because the reversal blend adds to the measured
// peak.
function shortenSlowStrokes({ positions, durations }) {
  return {
    positions,
    durations: durations.map((millis, i) => {
      const amplitude = amplitudeAt(positions, i);
      const atFloor = (amplitude / FLOOR_VELOCITY) * 1000;
      const atAccelerationBudget = Math.sqrt((6 * amplitude) / ACCEL_BUDGET) * 1000;
      const shortest = Math.max(MIN_REVERSAL_GAP, atAccelerationBudget);
      return Math.round(Math.min(millis, Math.max(atFloor, shortest)));
    }),
  };
}

function measure({ positions, durations }) {
  const speeds = positions.map((_, i) => (amplitudeAt(positions, i) / durations[i]) * 1000);
  const amplitudes = positions.map((_, i) => amplitudeAt(positions, i));
  const phrase = durations.reduce((a, b) => a + b, 0);
  const mean = speeds.reduce((a, b) => a + b, 0) / speeds.length;
  const cv = Math.sqrt(speeds.reduce((a, b) => a + (b - mean) ** 2, 0) / speeds.length) / mean;
  return {
    phrase, cv,
    vMin: Math.min(...speeds), vMax: Math.max(...speeds),
    minAmplitude: Math.min(...amplitudes), minGap: Math.min(...durations),
    paceMean: (amplitudes.reduce((a, b) => a + b, 0) / phrase) * 1000,
  };
}

// Rejects only what actually stutters or stops.
//
// The reported failure is dead time, and dead time is fully described by two
// things: a stroke too slow to feel, and a stroke too small to feel. Both are
// checked here.
//
// Two rules that guard the designed catalog are deliberately NOT applied.
// Speed spread was a correlate of stalling there, not a cause; excision now
// guarantees a velocity floor, so a wide spread is dynamics rather than a stall.
// The 450ms reversal gap is a budget the designed specs are fitted to by
// mustFitCatalog, which curated clips have never run through -- they normalize
// only -- and a fast reversal is the opposite of the problem being fixed.
// Applying either here rejected 141 clips for reasons unrelated to the symptom.
function rejection({ positions }, m) {
  if (positions.length < MIN_TURNING_POINTS) return `too few strokes left to be a loop`;
  if (!isFinite(m.vMin) || m.vMin < FLOOR_VELOCITY) return `a stroke stays under ${FLOOR_VELOCITY}%/s`;
  if (m.minAmplitude < MIN_AMPLITUDE) return `a stroke stays under ${MIN_AMPLITUDE}% travel`;
  return null;
}

// Reach the cycle floor by repeating the phrase, never by stretching it:
// stretching divides every velocity and puts the stalls straight back.
function buildPoints({ positions, durations }) {
  const phrase = durations.reduce((a, b) => a + b, 0);
  const repeats = Math.max(1, Math.ceil(CYCLE_FLOOR / phrase));
  const points = [{ time_ms: 0, position_percent: positions[0] }];
  let elapsed = 0;
  for (let r = 0; r < repeats; r++) {
    for (let i = 0; i < positions.length; i++) {
      elapsed += durations[i];
      points.push({ time_ms: elapsed, position_percent: positions[(i + 1) % positions.length] });
    }
  }
  return { points, cycle: elapsed, repeats, phrase };
}

// Clips whose rendered curve still stalls after excision, measured in Go against
// the real monotone-Hermite curve. Chord velocity cannot decide this on its own:
// smoothing drives velocity toward zero at every turning point, so a stroke that
// averages above the floor can still sit under it around its reversals. Produce
// the list with:
//
//   EMIT_STALLING=<path> go test ./internal/motion -run TestScratchEmitStallingCurated
//   STALLING=<path> APPLY=1 node scripts/curated-pattern-labeller.js
const stallingList = process.env.STALLING
  ? new Set(JSON.parse(fs.readFileSync(process.env.STALLING, "utf8")))
  : new Set();

const files = fs.readdirSync(DIR).filter((f) => f.endsWith(".mhpattern.json"));
const kept = [];
const dropped = [];

for (const file of files) {
  if (stallingList.has(file)) {
    dropped.push({ file, reason: "still stalls once rendered", survived: "-" });
    continue;
  }
  const full = path.join(DIR, file);
  const doc = JSON.parse(fs.readFileSync(full, "utf8"));
  const original = toStrokes(doc);
  const before = measure(original);
  const repaired = shortenSlowStrokes(mergeShortStrokes(toStrokes(doc)));
  const after = measure(repaired);
  const reason = rejection(repaired, after);
  if (reason) {
    dropped.push({ file, reason, survived: `${repaired.positions.length}/${original.positions.length}` });
    continue;
  }
  const built = buildPoints(repaired);
  if (built.cycle > CYCLE_CEILING) {
    dropped.push({ file, reason: `phrase repeats past the ${CYCLE_CEILING}ms cycle ceiling`, survived: "-" });
    continue;
  }
  kept.push({
    file, full, doc, repaired, after, built,
    excised: Math.max(0, before.phrase - after.phrase),
    untouched: before.phrase === after.phrase && original.positions.length === repaired.positions.length,
  });
}

// Label from the repaired curve, ordered by pace inside each band so the
// numbering carries meaning.
kept.sort((a, b) => a.after.paceMean - b.after.paceMean);
const counters = {};
for (const entry of kept) {
  entry.key = `${BANDS.find((b) => entry.after.paceMean < b.max).name} ${rhythmOf(entry.after.cv)}`;
  counters[entry.key] = (counters[entry.key] || 0) + 1;
  entry.index = counters[entry.key];
}
for (const entry of kept) {
  entry.newName = counters[entry.key] > 1 ? `${entry.key} ${entry.index}` : entry.key;
  entry.newFile = entry.newName.toLowerCase().replace(/\s+/g, "-") + ".mhpattern.json";
  const spm = (entry.repaired.positions.length / (entry.built.phrase / 1000)) * 60;
  entry.newDesc = spm < 90 ? "Slow cadence." : spm < 220 ? "Medium cadence." : "Quick cadence.";
}

console.log(`${files.length} curated clips\n`);
console.log(`  kept    ${String(kept.length).padStart(3)}  ` +
  `(${kept.filter((k) => k.untouched).length} already clean, ` +
  `${kept.filter((k) => !k.untouched).length} repaired by excision)`);
console.log(`  dropped ${String(dropped.length).padStart(3)}\n`);

const byReason = {};
for (const d of dropped) byReason[d.reason] = (byReason[d.reason] || 0) + 1;
console.log("dropped because:");
for (const [reason, count] of Object.entries(byReason).sort((a, b) => b[1] - a[1])) {
  console.log(`  ${String(count).padStart(3)}  ${reason}`);
}

console.log("\nkept, by band:");
for (const band of BANDS) {
  const inBand = kept.filter((k) => k.key.startsWith(band.name));
  if (!inBand.length) continue;
  const paces = inBand.map((k) => k.after.paceMean);
  console.log(`  ${band.name.padEnd(8)} ${String(inBand.length).padStart(3)}  ` +
    `${Math.min(...paces).toFixed(0)}-${Math.max(...paces).toFixed(0)} %/s`);
}
console.log(`\nexcised ${(kept.reduce((s, k) => s + k.excised, 0) / 1000).toFixed(1)}s of dead time from the kept clips`);

if (new Set(kept.map((k) => k.newFile)).size !== kept.length) {
  console.error("ERROR: duplicate output names");
  process.exit(1);
}

if (APPLY) {
  for (const d of dropped) fs.unlinkSync(path.join(DIR, d.file));
  // Two passes: every source moves to a scratch name first, so a clip renaming
  // onto a name another clip still holds cannot clobber it.
  for (const entry of kept) {
    entry.scratch = path.join(DIR, `~${entry.file}`);
    fs.renameSync(entry.full, entry.scratch);
  }
  for (const entry of kept) {
    entry.doc.name = entry.newName;
    entry.doc.description = entry.newDesc;
    entry.doc.cycle_ms = entry.built.cycle;
    entry.doc.points = entry.built.points;
    fs.writeFileSync(entry.scratch, JSON.stringify(entry.doc, null, 2) + "\n");
    fs.renameSync(entry.scratch, path.join(DIR, entry.newFile));
  }
  const catalog = JSON.parse(fs.readFileSync(CATALOG_PATH, "utf8"));
  catalog.pattern_count = kept.length;
  catalog.patterns = kept
    .map((k) => ({ file: k.newFile, name: k.newName }))
    .sort((a, b) => a.file.localeCompare(b.file, "en"));
  fs.writeFileSync(CATALOG_PATH, JSON.stringify(catalog, null, 2) + "\n");
  console.log(`\napplied: ${kept.length} rewritten, ${dropped.length} deleted, catalog reindexed`);
}
