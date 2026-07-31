// Designer for the velocity-authored entries in catalogPatternSpecs
// (internal/motion/content.go). Run it to add or retune a built-in pattern:
//
//   node scripts/pattern-designer.js          # check every spec, report shape collisions
//   EMIT=1 node scripts/pattern-designer.js   # also print pasteable Go specs
//
// Authoring here is (turning positions, stroke velocity); travel time is derived
// as amplitude / velocity. That is the whole point. In the catalog format
// Positions[] and TravelMillis[] are independent lists, so stroke velocity -- the
// only quantity the hand registers -- is an emergent value nobody chose. That is
// how the retired Cascade ended up putting a 14-unit stroke and an 84-unit stroke
// on nearly the same duration, giving 18%/s next to 116%/s and a 2.46s stall in a
// 6.6s loop. Deriving time from velocity makes that unrepresentable.
//
// The cycle floor is reached by REPEATING the phrase, never by stretching it.
// mustFitCatalog reaches RoutineCycleFloorMillis by scaling every timestamp,
// which divides every velocity by the same factor -- it slowed the retired
// Descending Ladder 1.22x out of its authored 410-474ms strokes. Repeating fills
// the floor at unchanged velocity and hands back the phrase periodicity the
// retained patterns have.
//
// Speed scaling is a uniform time factor (plan.go: timeFactor = period/duration),
// so velocity RATIOS inside a pattern are speed-invariant. That is why the rules
// below are ratios or nominal-speed floors, and why no two patterns may differ
// only by pace -- that is what intensity is for.
//
// These constants are a design aid, not the gate. The authority is
// TestCatalogPatternsHoldTheMeasuredSpeedEnvelope in internal/motion, which
// measures the real generated curve; it is deliberately looser here so drafts get
// pushed toward the middle of the envelope rather than its edge. If you change a
// bound, change it there first.

const CYCLE_FLOOR = 6600; // RoutineCycleFloorMillis
const MIN_REVERSAL_GAP = 450; // catalogMinReversalGap
const MAX_ACCEL = 3000; // catalogMaxAcceleration (%/s^2)

// Envelope measured from the 13 patterns that survived the user disabling
// fifteen by hand. Tighter than the Go test, which has to admit those thirteen
// exactly (Drift runs 44%/s, Tease spreads 3.2x).
const FLOOR_VELOCITY = 45; // slowest stroke among kept patterns (Drift, 44)
const MAX_V_RATIO = 2.6; // fastest/slowest stroke within one pattern
const MIN_AMPLITUDE = 22; // MIN_REVERSAL_GAP * FLOOR_VELOCITY / 1000
const MIN_MEAN_VELOCITY = 55; // Drift/Flutter sit at 56-57
const MAX_VOCAB = 6; // kept Tease and Rising Reach both use 6

// The reversal gap caps velocity for short strokes: v <= amplitude / 0.45.
// A 30-unit stroke can never exceed 66%/s, so long strokes are the fast ones.
const maxVelocityFor = (amp) => (amp / MIN_REVERSAL_GAP) * 1000;

function build(spec) {
  const pos = spec.positions;
  const amps = pos.map((_, i) => Math.abs(pos[(i + 1) % pos.length] - pos[i]));
  if (spec.velocities.length !== pos.length) {
    throw new Error(`${spec.id}: ${pos.length} strokes but ${spec.velocities.length} velocities`);
  }
  const ms = amps.map((a, i) => Math.round((a / spec.velocities[i]) * 1000));
  const phrase = ms.reduce((a, b) => a + b, 0);
  const repeats = Math.max(1, Math.ceil(CYCLE_FLOOR / phrase));
  const emitPositions = [];
  const emitTravel = [];
  for (let r = 0; r < repeats; r++) {
    emitPositions.push(...pos);
    emitTravel.push(...ms);
  }
  return { ...spec, amps, ms, phrase, repeats, emitPositions, emitTravel, cycle: phrase * repeats };
}

function measure(p) {
  const v = p.amps.map((a, i) => (a / p.ms[i]) * 1000);
  const mu = v.reduce((a, b) => a + b, 0) / v.length;
  const vocab = new Set(p.amps.map((a) => Math.round(a / 8))).size;
  const accel = Math.max(...p.amps.map((a, i) => (6 * a) / (p.ms[i] / 1000) ** 2));
  const travel = p.amps.reduce((a, b) => a + b, 0);
  const mids = p.positions.map((x, i) => (x + p.positions[(i + 1) % p.positions.length]) / 2);
  const ampMu = travel / p.amps.length;
  return {
    vMin: Math.min(...v), vMax: Math.max(...v), ratio: Math.max(...v) / Math.min(...v),
    vocab, minAmp: Math.min(...p.amps), minGap: Math.min(...p.ms), accel,
    mean: (travel * 1000) / p.phrase,
    lo: Math.min(...p.positions), hi: Math.max(...p.positions),
    // Shape features used only for the distinctness check. Trends are signed, so
    // a pattern and its reverse (Narrowing vs Opening Up) do not collapse onto
    // each other -- converging and diverging are opposite characters, not one.
    f: [
      ampMu / 100,
      Math.sqrt(p.amps.reduce((a, b) => a + (b - ampMu) ** 2, 0) / p.amps.length) / 100,
      mids.reduce((a, b) => a + b, 0) / mids.length / 100,
      trend(p.amps) / 2,
      trend(mids) / 2,
      (Math.max(...v) / Math.min(...v)) / 3,
      p.amps.filter((a) => a > 60).length / p.amps.length,
      blockRun(p.amps) / 8,
      Math.min(p.phrase, 14000) / 14000,
    ],
  };
}

// Correlation of a series with its index: +1 rising through the phrase, -1 falling.
function trend(xs) {
  const n = xs.length;
  if (n < 2) return 0;
  const mx = (n - 1) / 2;
  const my = xs.reduce((a, b) => a + b, 0) / n;
  let num = 0, dx = 0, dy = 0;
  xs.forEach((y, i) => {
    num += (i - mx) * (y - my); dx += (i - mx) ** 2; dy += (y - my) ** 2;
  });
  return dx > 0 && dy > 0 ? num / Math.sqrt(dx * dy) : 0;
}

// Mean run length of consecutive same-width strokes: 1 for fast alternation,
// high for long blocks of one width.
function blockRun(amps) {
  const bucket = amps.map((a) => Math.round(a / 20));
  const runs = [];
  let run = 1;
  for (let i = 1; i < bucket.length; i++) {
    if (bucket[i] === bucket[i - 1]) run++;
    else { runs.push(run); run = 1; }
  }
  runs.push(run);
  return runs.reduce((a, b) => a + b, 0) / runs.length;
}

function check(p, m) {
  const fail = [];
  if (m.vMin < FLOOR_VELOCITY) fail.push(`slow stroke ${m.vMin.toFixed(0)}%/s`);
  if (m.ratio > MAX_V_RATIO) fail.push(`v-ratio ${m.ratio.toFixed(2)}x`);
  if (m.vocab > MAX_VOCAB) fail.push(`vocab ${m.vocab}`);
  if (m.minAmp < MIN_AMPLITUDE) fail.push(`stroke ${m.minAmp} < ${MIN_AMPLITUDE}`);
  if (m.minGap < MIN_REVERSAL_GAP) fail.push(`gap ${m.minGap}ms`);
  if (m.accel > MAX_ACCEL) fail.push(`accel ${m.accel.toFixed(0)}`);
  if (m.mean < MIN_MEAN_VELOCITY) fail.push(`mean ${m.mean.toFixed(0)}%/s`);
  if (p.cycle < CYCLE_FLOOR) fail.push(`cycle ${p.cycle}`);
  if (m.lo < 0 || m.hi > 100) fail.push(`range ${m.lo}-${m.hi}`);
  p.amps.forEach((a, i) => {
    if ((a / p.ms[i]) * 1000 > maxVelocityFor(a) + 1) fail.push(`stroke ${i} over gap cap`);
  });
  return fail;
}

// Names and descriptions are model-facing: prompts.go ships {id, name,
// description, tags} straight into the catalog JSON the model chooses from.
// Each description says what it feels like AND when to reach for it, because
// the model is mapping conversational intent onto a pattern id.
const specs = [
  {
    id: "easing-down", name: "Easing Down",
    description: "The stroke window steps steadily lower without losing pace. Use when winding down from a peak or easing intensity back.",
    tags: ["easing", "descending", "calming"],
    positions: [100, 56, 86, 42, 72, 28, 58, 14],
    velocities: [85, 62, 85, 62, 85, 62, 85, 120],
  },
  {
    id: "building-up", name: "Building Up",
    description: "The stroke window climbs step by step, then one full sweep resets it. Use when building intensity or answering an eager reply.",
    tags: ["building", "ascending", "progressive"],
    positions: [0, 44, 14, 58, 28, 72, 42, 86],
    velocities: [85, 62, 85, 62, 85, 62, 85, 120],
  },
  {
    id: "broad-and-tight", name: "Broad and Tight",
    description: "One wide sweep answered by a run of tight centred strokes. Use for playful contrast that keeps a steady pace.",
    tags: ["contrast", "paired", "centred"],
    positions: [6, 94, 34, 66, 34, 66],
    velocities: [120, 120, 62, 62, 62, 62],
  },
  {
    id: "upper-accents", name: "Upper Accents",
    description: "Quick accents in the upper range answered by one broad sweep. Use to hold attention near the top.",
    tags: ["upper", "accent", "teasing"],
    positions: [8, 96, 62, 96, 62, 96],
    velocities: [122, 70, 70, 70, 70, 122],
  },
  {
    id: "lower-accents", name: "Lower Accents",
    description: "Quick accents in the lower range answered by one broad sweep. Use to keep the motion low and deep.",
    tags: ["lower", "accent", "deep"],
    positions: [92, 4, 38, 4, 38, 4],
    velocities: [122, 70, 70, 70, 70, 122],
  },
  {
    id: "steady-drift", name: "Steady Drift",
    description: "A single unchanging pace while the stroke window wanders up and back. Use for a calm, hypnotic stretch or a dependable baseline.",
    tags: ["steady", "migrating", "calm"],
    positions: [10, 52, 20, 62, 30, 72],
    velocities: [70, 70, 70, 70, 70, 70],
  },
  {
    id: "offbeat", name: "Offbeat",
    description: "Even strokes broken by one deeper reach off the beat. Use when the rhythm should stay unpredictable.",
    tags: ["syncopated", "accent", "varied"],
    positions: [16, 64, 16, 64, 16, 92],
    velocities: [72, 72, 72, 72, 72, 122],
  },
  {
    id: "narrowing", name: "Narrowing",
    description: "Strokes close in toward the centre and the pace eases with them. Use to concentrate and quiet the motion.",
    tags: ["narrowing", "easing", "centred"],
    positions: [15, 85, 21, 79, 28, 72, 35, 65],
    velocities: [134, 122, 112, 98, 86, 72, 62, 98],
  },
  {
    id: "opening-up", name: "Opening Up",
    description: "Strokes widen out from the centre and pick up pace as they go. Use to open the motion out of a tight stretch.",
    tags: ["widening", "building", "centred"],
    positions: [35, 65, 28, 72, 21, 79, 15, 85],
    velocities: [62, 72, 86, 98, 112, 122, 134, 98],
  },
  {
    id: "long-return", name: "Long Return",
    description: "A quick reach answered by an unhurried return, over and over. Use for a leaning, asymmetric feel.",
    tags: ["asymmetric", "leaning", "paired"],
    positions: [10, 78, 10, 78],
    velocities: [120, 62, 120, 62],
  },
  {
    id: "swell", name: "Swell",
    description: "The stroke window rises across the whole cycle and settles back down again. Use for one long unbroken arc rather than a repeating beat.",
    tags: ["arc", "migrating", "long"],
    positions: [5, 45, 15, 55, 25, 65, 35, 75, 25, 65, 15, 55],
    velocities: [80, 62, 80, 62, 80, 62, 80, 98, 80, 98, 80, 98],
  },
  {
    id: "rocking", name: "Rocking",
    description: "Even mid-range strokes at one unchanging pace, nothing else. Use as a plain dependable rhythm to settle into.",
    tags: ["steady", "even", "centred"],
    positions: [25, 75],
    velocities: [90, 90],
  },
  {
    id: "surge-and-settle", name: "Surge and Settle",
    description: "One fast full sweep drops into a long run of moderate mid strokes. Use to punctuate hard, then recover for a while.",
    tags: ["accent", "recovery", "long"],
    positions: [2, 98, 35, 68, 35, 68, 35, 68, 35, 68, 35, 68],
    velocities: [140, 110, 68, 68, 68, 68, 68, 68, 68, 68, 68, 112],
  },
  {
    id: "three-and-one", name: "Three and One",
    description: "Three tight strokes held up high, then one full plunge and recovery. Use for a grouped phrase with a clear landing.",
    tags: ["grouped", "resolving", "upper"],
    positions: [95, 65, 95, 65, 95, 65, 95, 5],
    velocities: [62, 62, 62, 62, 62, 62, 122, 122],
  },
  {
    id: "crosscut", name: "Crosscut",
    description: "A long block of broad strokes trades places with a long block of tight ones. Use for restless variety that still keeps a beat.",
    tags: ["alternating", "blocks", "restless"],
    positions: [8, 88, 8, 88, 8, 88, 55, 85, 55, 85, 55, 85],
    velocities: [125, 125, 125, 125, 125, 68, 62, 62, 62, 62, 62, 122],
  },
];

const built = specs.map(build);
console.log(
  `floor=${CYCLE_FLOOR}ms  v>=${FLOOR_VELOCITY}%/s  ratio<=${MAX_V_RATIO}x  amp>=${MIN_AMPLITUDE}  ` +
  `gap>=${MIN_REVERSAL_GAP}ms  accel<=${MAX_ACCEL}  mean>=${MIN_MEAN_VELOCITY}%/s  vocab<=${MAX_VOCAB}\n`);
let bad = 0;
const measured = [];
for (const p of built) {
  const m = measure(p);
  measured.push(m);
  const fail = check(p, m);
  if (fail.length) bad++;
  console.log(
    `${fail.length ? "FAIL" : "ok  "} ${p.name.padEnd(17)} v=${m.vMin.toFixed(0)}..${m.vMax.toFixed(0)} ` +
    `(${m.ratio.toFixed(2)}x) vocab=${m.vocab} amp>=${String(m.minAmp).padStart(2)} gap>=${m.minGap} ` +
    `accel=${m.accel.toFixed(0).padStart(4)} mean=${m.mean.toFixed(0)} range=${m.lo}-${m.hi} ` +
    `cycle=${p.cycle} (${p.repeats}x${p.phrase})` + (fail.length ? `\n     <-- ${fail.join(", ")}` : ""));
}

// No two patterns may be near-duplicates: the model picks by character, so two
// entries that feel the same are two entries it cannot choose between.
console.log("\nclosest pairs by shape (amp mean/spread, centre, drift, v-ratio, long-stroke share):");
const pairs = [];
for (let i = 0; i < built.length; i++) {
  for (let j = i + 1; j < built.length; j++) {
    const d = Math.sqrt(measured[i].f.reduce((s, x, k) => s + (x - measured[j].f[k]) ** 2, 0));
    pairs.push([d, built[i].name, built[j].name]);
  }
}
pairs.sort((a, b) => a[0] - b[0]);
for (const [d, a, b] of pairs.slice(0, 6)) {
  console.log(`  ${d < 0.18 ? "TOO CLOSE" : "ok       "} ${d.toFixed(3)}  ${a} / ${b}`);
}

console.log(`\n${built.length - bad}/${built.length} inside the envelope; ` +
  `${pairs.filter((p) => p[0] < 0.18).length} pairs too close`);

if (process.env.EMIT) {
  console.log("\n--- Go specs ---");
  for (const p of built) {
    console.log(`	{
		ID: Pattern${p.id.split("-").map((s) => s[0].toUpperCase() + s.slice(1)).join("")}, Name: ${JSON.stringify(p.name)},
		Description:  ${JSON.stringify(p.description)},
		Positions:    []float64{${p.emitPositions.join(", ")}},
		TravelMillis: []int64{${p.emitTravel.join(", ")}},
		Tags:         []string{${p.tags.map((t) => JSON.stringify(t)).join(", ")}},
	},`);
  }
}
