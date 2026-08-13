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
// LABELLING. Names and tags describe geometry and relative rhythm only. Absolute
// pace is deliberately absent: every loop now shares one normalized speed
// control, so an authored clip's old Gentle/Fast/Intense tier is both misleading
// and harmful to model selection. Stable filenames and IDs are retained for
// user preferences and saved sessions.

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

const SHAPE_METADATA = {
  "easy-drive-1.mhpattern.json": ["Notched Full Sweep", "Mostly full-span reversals with one shortened peak.", ["full-span", "single-notch"]],
  "easy-drive-2.mhpattern.json": ["Full Sweep Run A", "Repeating full-span reversals.", ["full-span", "repeating"]],
  "easy-drive-3.mhpattern.json": ["Full Sweep Run B", "Repeating full-span reversals.", ["full-span", "repeating"]],
  "easy-drive-4.mhpattern.json": ["Full Sweep Run C", "Repeating full-span reversals.", ["full-span", "repeating"]],
  "easy-drive-5.mhpattern.json": ["Peak Steps", "A fixed lower return alternates among several upper peaks.", ["stepped-peaks", "fixed-return"]],
  "easy-roll.mhpattern.json": ["Upper Cascade", "An upper-biased sequence moves through several return depths and peaks.", ["upper-biased", "multi-level"]],
  "fast-drive-1.mhpattern.json": ["Return Depth Shuffle", "Broad strokes shuffle among full and partial return depths.", ["varied-returns", "broad-strokes"]],
  "fast-drive-10.mhpattern.json": ["Broken Full Sweep", "Full-span reversals are interrupted by a small set of partial returns.", ["full-span", "partial-breaks"]],
  "fast-drive-11.mhpattern.json": ["Peak Ladder Mosaic", "Peak heights and return depths change in short stepped groups.", ["stepped-peaks", "varied-returns"]],
  "fast-drive-12.mhpattern.json": ["Midrange Break", "A run of full sweeps gives way briefly to a centered pair before resetting.", ["full-to-midrange", "grouped"]],
  "fast-drive-13.mhpattern.json": ["Descending Peak Finish", "Mostly full-depth returns meet upper peaks that shorten near the end.", ["descending-peaks", "deep-return"]],
  "fast-drive-2.mhpattern.json": ["Multi-Level Circuit A", "Full, partial, and centered strokes rotate through an irregular circuit.", ["multi-level", "alternating"]],
  "fast-drive-3.mhpattern.json": ["Upper Shelf Pulse", "Upper-range pulses settle onto a repeated shallow return shelf.", ["upper-range", "return-shelf"]],
  "fast-drive-4.mhpattern.json": ["Return Ladder", "Broad upper peaks meet a ladder of progressively different return depths.", ["return-ladder", "broad-peaks"]],
  "fast-drive-5.mhpattern.json": ["Multi-Level Circuit B", "Broad sweeps and middle-range strokes alternate through several levels.", ["multi-level", "broad-contrast"]],
  "fast-drive-6.mhpattern.json": ["Return Depth Accents", "Full sweeps are punctuated by several shallower return accents.", ["varied-returns", "full-span"]],
  "fast-drive-7.mhpattern.json": ["Peak Drift", "Peak heights drift while mostly deep returns anchor the phrase.", ["drifting-peaks", "deep-return"]],
  "fast-drive-8.mhpattern.json": ["Descending Peak Blocks", "Deep returns support grouped peaks that step down in height.", ["descending-peaks", "grouped"]],
  "fast-drive-9.mhpattern.json": ["Lowered Return Shuffle", "Upper peaks repeat over a shuffled set of lower return depths.", ["varied-returns", "upper-peaks"]],
  "fast-roll-1.mhpattern.json": ["Upper to Full Blocks", "Upper-range blocks transition into broad and full-span sweeps.", ["upper-to-full", "grouped"]],
  "fast-roll-2.mhpattern.json": ["Full Sweep Tail", "A long full-sweep run ends with two partial-depth accents.", ["full-span", "tail-accent"]],
  "fast-roll-3.mhpattern.json": ["Upper Step Blocks", "A full sweep frames a block of repeated upper-half strokes.", ["upper-block", "full-frame"]],
  "fast-roll-4.mhpattern.json": ["Near-Full Accent", "Near-full reversals carry a few small endpoint offsets.", ["near-full", "endpoint-offsets"]],
  "fast-surge-1.mhpattern.json": ["Rising Ladder Reset", "Lower-anchored peaks rise in stages before an upper multi-level reset.", ["rising-peaks", "lower-anchor"]],
  "fast-surge-2.mhpattern.json": ["Offset Zigzag", "Both endpoints shift through a broad irregular zigzag.", ["offset-zigzag", "multi-level"]],
  "gentle-drive-1.mhpattern.json": ["Full Sweep Run D", "Repeating full-span reversals.", ["full-span", "repeating"]],
  "gentle-drive-2.mhpattern.json": ["Full Sweep Run E", "Repeating full-span reversals.", ["full-span", "repeating"]],
  "gentle-drive-3.mhpattern.json": ["Full Sweep Run F", "Repeating full-span reversals.", ["full-span", "repeating"]],
  "intense-drive-1.mhpattern.json": ["Split-Level Pulse", "Full sweeps alternate with a repeated middle-range pulse.", ["split-level", "middle-pulse"]],
  "intense-drive-10.mhpattern.json": ["Full Sweep Run G", "Repeating full-span reversals.", ["full-span", "repeating"]],
  "intense-drive-11.mhpattern.json": ["Return Ladder Mosaic", "Upper peaks combine with a stepped mosaic of return depths.", ["return-ladder", "varied-peaks"]],
  "intense-drive-12.mhpattern.json": ["Peak Groups", "Deep returns anchor grouped upper peaks of several heights.", ["grouped-peaks", "deep-return"]],
  "intense-drive-13.mhpattern.json": ["Full Sweep Run H", "Repeating full-span reversals.", ["full-span", "repeating"]],
  "intense-drive-14.mhpattern.json": ["Full Sweep Run I", "Repeating full-span reversals.", ["full-span", "repeating"]],
  "intense-drive-15.mhpattern.json": ["Two-Stage Peaks", "Deep returns support one repeated peak height followed by a higher one.", ["two-stage-peaks", "deep-return"]],
  "intense-drive-16.mhpattern.json": ["Full Sweep Run J", "Repeating full-span reversals.", ["full-span", "repeating"]],
  "intense-drive-2.mhpattern.json": ["Peak Accent Chain", "Mostly full sweeps carry a chain of slightly shortened peak accents.", ["peak-accents", "deep-return"]],
  "intense-drive-3.mhpattern.json": ["Single Shallow Return", "A full-sweep run contains one shallow return and one shortened peak.", ["full-span", "single-return-accent"]],
  "intense-drive-4.mhpattern.json": ["Lowered Finish", "A full-sweep run ends on a shortened upper peak.", ["full-span", "shortened-finish"]],
  "intense-drive-5.mhpattern.json": ["Framed Full Sweep", "A short upper-range frame interrupts an otherwise full-sweep run.", ["full-span", "upper-frame"]],
  "intense-drive-6.mhpattern.json": ["Full Sweep Run K", "Repeating full-span reversals.", ["full-span", "repeating"]],
  "intense-drive-7.mhpattern.json": ["Descending Peaks", "Deep returns support a sequence of upper peaks at changing heights.", ["descending-peaks", "deep-return"]],
  "intense-drive-8.mhpattern.json": ["Peak Alternation", "Full peaks alternate with a repeated slightly shortened peak.", ["alternating-peaks", "deep-return"]],
  "intense-drive-9.mhpattern.json": ["Full Sweep Run L", "Repeating full-span reversals.", ["full-span", "repeating"]],
  "intense-roll-1.mhpattern.json": ["Offset Full Sweep", "Full sweeps carry a small set of near-endpoint offsets.", ["near-full", "endpoint-offsets"]],
  "intense-roll-2.mhpattern.json": ["Double Peak Accent", "A full-sweep run contains a paired set of shortened peak accents.", ["paired-peaks", "full-span"]],
  "intense-roll-3.mhpattern.json": ["Tapered Peak Finish", "Full sweeps taper to a shortened peak at each end of the phrase.", ["tapered-peaks", "deep-return"]],
  "intense-roll-4.mhpattern.json": ["Full Sweep Run M", "Repeating full-span reversals.", ["full-span", "repeating"]],
  "intense-roll-5.mhpattern.json": ["Descending Entry", "A descending staircase enters a run of full sweeps.", ["descending-entry", "full-span"]],
  "intense-surge-1.mhpattern.json": ["Peak Shuffle", "Deep returns support a shuffled set of near-full peak heights.", ["varied-peaks", "deep-return"]],
  "intense-surge-2.mhpattern.json": ["Full Sweep Run N", "Repeating full-span reversals.", ["full-span", "repeating"]],
  "intense-surge-3.mhpattern.json": ["Near-Full Run", "Repeating full-span reversals with one near-full peak.", ["near-full", "repeating"]],
  "steady-drive-1.mhpattern.json": ["Lower Anchor Circuit", "An upper entry drops into repeated lower-range anchors before a full reset.", ["lower-anchor", "multi-level"]],
  "steady-drive-2.mhpattern.json": ["Peak Accent Run", "Deep returns repeat under a sequence of full and shortened peaks.", ["varied-peaks", "deep-return"]],
  "steady-drive-3.mhpattern.json": ["Three-Level Pairing", "Three middle levels form a paired phrase framed by full sweeps.", ["three-level", "paired"]],
  "steady-drive-4.mhpattern.json": ["Upper Accent Circuit", "Full sweeps transition into an upper-biased multi-level circuit.", ["upper-biased", "multi-level"]],
  "steady-drive-5.mhpattern.json": ["Deep Step Quartet", "A four-level descending step repeats between full sweeps.", ["descending-steps", "full-frame"]],
  "steady-roll-1.mhpattern.json": ["Upper Pulse Run", "A long upper-range pulse run resolves through one full sweep.", ["upper-pulses", "full-resolution"]],
  "steady-roll-2.mhpattern.json": ["Multi-Level Drift", "Both endpoints drift through an irregular set of partial and broad strokes.", ["multi-level", "drifting-window"]],
};

function relativeRhythm(cv) {
  if (cv < 0.15) return { tag: "even-rhythm", sentence: "Reversal timing is even." };
  if (cv < 0.45) return { tag: "varied-rhythm", sentence: "Reversal timing varies within the loop." };
  return { tag: "syncopated", sentence: "Grouped timing creates offbeat accents." };
}

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

kept.sort((a, b) => a.file.localeCompare(b.file, "en"));
for (const entry of kept) {
  const metadata = SHAPE_METADATA[entry.file];
  if (!metadata) throw new Error(`missing shape metadata for ${entry.file}`);
  const rhythm = relativeRhythm(entry.after.cv);
  entry.newName = metadata[0];
  entry.newDesc = `${metadata[1]} ${rhythm.sentence}`;
  entry.newTags = [...new Set([...metadata[2], rhythm.tag])];
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

console.log("\nkept, by relative rhythm:");
for (const tag of ["even-rhythm", "varied-rhythm", "syncopated"]) {
  console.log(`  ${tag.padEnd(16)} ${String(kept.filter((k) => k.newTags.includes(tag)).length).padStart(3)}`);
}
console.log(`\nexcised ${(kept.reduce((s, k) => s + k.excised, 0) / 1000).toFixed(1)}s of dead time from the kept clips`);

if (new Set(kept.map((k) => k.newName)).size !== kept.length) {
  console.error("ERROR: duplicate display names");
  process.exit(1);
}

if (APPLY) {
  for (const d of dropped) fs.unlinkSync(path.join(DIR, d.file));
  for (const entry of kept) {
    entry.doc.name = entry.newName;
    entry.doc.description = entry.newDesc;
    entry.doc.tags = entry.newTags;
    entry.doc.cycle_ms = entry.built.cycle;
    entry.doc.points = entry.built.points;
    fs.writeFileSync(entry.full, JSON.stringify(entry.doc, null, 2) + "\n");
  }
  const catalog = JSON.parse(fs.readFileSync(CATALOG_PATH, "utf8"));
  catalog.pattern_count = kept.length;
  catalog.patterns = kept
    .map((k) => ({ file: k.file, name: k.newName }))
    .sort((a, b) => a.file.localeCompare(b.file, "en"));
  fs.writeFileSync(CATALOG_PATH, JSON.stringify(catalog, null, 2) + "\n");
  console.log(`\napplied: ${kept.length} rewritten, ${dropped.length} deleted, catalog reindexed`);
}
