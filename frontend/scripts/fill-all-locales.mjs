/**
 * Fill all missing locale translations via MyMemory API.
 * Writes: locale-overrides-pt.mjs, locale-overrides-ru.mjs,
 *         updates additionalFrOverrides/additionalRuOverrides in locale-additions.mjs
 */
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { execSync } from "child_process";
import {
  additionalPtOverrides,
  additionalFrOverrides,
  additionalRuOverrides,
} from "./locale-additions.mjs";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.join(__dirname, "..");
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const PROPER_NOUNS = [
  "MagicHandy", "Handy", "Intiface", "Ollama", "Autospeak", "StrokeGPT",
  "BPM", "JSON", "HSSP", "TTS", "STT", "GPU", "WebSocket", "funscript",
  "Edge-TTS", "Whisper", "Ctrl+Enter",
];

function protectTerms(text) {
  const tokens = [];
  let out = text;
  PROPER_NOUNS.forEach((term, i) => {
    const token = `__PN${i}__`;
    if (out.includes(term)) {
      tokens.push({ token, term });
      out = out.split(term).join(token);
    }
  });
  return { out, tokens };
}

function restoreTerms(text, tokens) {
  let out = text;
  for (const { token, term } of tokens) out = out.split(token).join(term);
  return out;
}

async function translateText(text, targetLang, attempt = 0) {
  if (!text || !text.trim()) return text;
  const { out, tokens } = protectTerms(text);
  const url =
    "https://api.mymemory.translated.net/get?q=" +
    encodeURIComponent(out) +
    `&langpair=en|${targetLang}&de=magichandy@local.dev`;
  const res = await fetch(url);
  if (res.status === 429 && attempt < 8) {
    await sleep(4000 * (attempt + 1));
    return translateText(text, targetLang, attempt + 1);
  }
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  const data = await res.json();
  if (data.responseStatus !== 200) throw new Error(data.responseDetails || "translate failed");
  let result = restoreTerms(data.responseData?.translatedText ?? text, tokens);
  // Fix common API artifacts
  result = result.replace(/&quot;/g, '"').replace(/&#39;/g, "'").replace(/&amp;/g, "&");
  return result;
}

function flatten(obj, prefix = "") {
  const out = {};
  for (const [k, v] of Object.entries(obj)) {
    const key = prefix ? `${prefix}.${k}` : k;
    if (v && typeof v === "object" && !Array.isArray(v)) Object.assign(out, flatten(v, key));
    else out[key] = v;
  }
  return out;
}

function formatExport(name, obj) {
  const lines = Object.entries(obj)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([k, v]) => `  ${JSON.stringify(k)}: ${JSON.stringify(v)},`);
  return `/** Auto-generated locale overrides */\nexport const ${name} = {\n${lines.join("\n")}\n};\n`;
}

function untranslatedKeys(en, target) {
  const properRe =
    /MagicHandy|Handy|Intiface|Ollama|Autospeak|StrokeGPT|BPM|JSON|HSSP|TTS|STT|GPU|WebSocket|funscript|Edge-TTS|Whisper|Ctrl\+Enter/i;
  return Object.keys(en).filter((k) => target[k] === en[k] && !properRe.test(en[k]));
}

async function translateMap(entries, targetLang, label, existing = {}) {
  const out = { ...existing };
  const keys = Object.keys(entries).filter((k) => !out[k] || out[k] === entries[k]);
  console.log(`[${label}] translating ${keys.length} keys...`);
  for (let i = 0; i < keys.length; i++) {
    const key = keys[i];
    const en = entries[key];
    try {
      out[key] = await translateText(en, targetLang);
    } catch (err) {
      console.warn(`[${label}] ${key}: ${err.message}`);
      out[key] = en;
    }
    if ((i + 1) % 5 === 0 || i === keys.length - 1) {
      console.log(`[${label}] ${i + 1}/${keys.length}`);
      fs.writeFileSync(
        path.join(__dirname, `_${label}-progress.json`),
        JSON.stringify(out, null, 2)
      );
    }
    await sleep(800);
  }
  return out;
}

function replaceExportBlock(filePath, exportName, newObj) {
  const content = fs.readFileSync(filePath, "utf8");
  const startMarker = `export const ${exportName} = {`;
  const startIdx = content.indexOf(startMarker);
  if (startIdx === -1) throw new Error(`Cannot find ${exportName} in ${filePath}`);
  let depth = 0;
  let endIdx = startIdx + startMarker.length - 1;
  for (let i = endIdx; i < content.length; i++) {
    if (content[i] === "{") depth++;
    if (content[i] === "}") {
      depth--;
      if (depth === 0) {
        endIdx = i + 1;
        break;
      }
    }
  }
  const lines = Object.entries(newObj)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([k, v]) => `  ${JSON.stringify(k)}: ${JSON.stringify(v)},`);
  const newBlock = `export const ${exportName} = {\n${lines.join("\n")}\n};`;
  const updated = content.slice(0, startIdx) + newBlock + content.slice(endIdx);
  fs.writeFileSync(filePath, updated);
}

// Build fresh locales
execSync("node scripts/build-locales.mjs", { cwd: root, stdio: "inherit" });

const localesDir = path.join(root, "src/i18n/locales");
const en = flatten(JSON.parse(fs.readFileSync(path.join(localesDir, "en.json"), "utf8")));
const pt = flatten(JSON.parse(fs.readFileSync(path.join(localesDir, "pt.json"), "utf8")));
const fr = flatten(JSON.parse(fs.readFileSync(path.join(localesDir, "fr.json"), "utf8")));
const ru = flatten(JSON.parse(fs.readFileSync(path.join(localesDir, "ru.json"), "utf8")));

const ptMissing = Object.fromEntries(untranslatedKeys(en, pt).map((k) => [k, en[k]]));
const ruMissing = Object.fromEntries(untranslatedKeys(en, ru).map((k) => [k, en[k]]));

console.log("PT missing:", Object.keys(ptMissing).length);
console.log("RU missing:", Object.keys(ruMissing).length);

// Load existing progress
function loadProgress(name) {
  try {
    return JSON.parse(fs.readFileSync(path.join(__dirname, `_${name}-progress.json`), "utf8"));
  } catch {
    return {};
  }
}

// 1. PT remaining overrides
let ptExisting = loadProgress("pt-fill");
const ptOut = await translateMap(ptMissing, "pt-BR", "pt-fill", ptExisting);
fs.writeFileSync(
  path.join(__dirname, "locale-overrides-pt.mjs"),
  formatExport("ptRemainingOverrides", ptOut)
);

// 2. RU full overrides
let ruExisting = loadProgress("ru-fill");
const ruOut = await translateMap(ruMissing, "ru", "ru-fill", ruExisting);
fs.writeFileSync(
  path.join(__dirname, "locale-overrides-ru.mjs"),
  formatExport("ruFullOverrides", ruOut)
);

// 3. Mirror additionalPtOverrides to FR and RU in locale-additions.mjs
const ptKeys = Object.keys(additionalPtOverrides);
const frMirror = { ...additionalFrOverrides };
const ruMirror = { ...additionalRuOverrides };

// Try to reuse frFullOverrides for FR mirror
let frFull = {};
try {
  const mod = await import("./locale-overrides-fr.mjs");
  frFull = mod.frFullOverrides ?? {};
} catch {}

const mirrorMissingFr = {};
const mirrorMissingRu = {};
for (const key of ptKeys) {
  const enVal = en[key];
  if (!enVal) continue;
  if (!frMirror[key] || frMirror[key] === enVal) {
    if (frFull[key] && frFull[key] !== enVal) {
      frMirror[key] = frFull[key];
    } else {
      mirrorMissingFr[key] = enVal;
    }
  }
  if (!ruMirror[key] || ruMirror[key] === enVal) {
    mirrorMissingRu[key] = enVal;
  }
}

console.log("FR mirror missing:", Object.keys(mirrorMissingFr).length);
console.log("RU mirror missing:", Object.keys(mirrorMissingRu).length);

const frMirrorTranslated = await translateMap(mirrorMissingFr, "fr", "fr-mirror", {});
Object.assign(frMirror, frMirrorTranslated);

const ruMirrorTranslated = await translateMap(mirrorMissingRu, "ru", "ru-mirror", {});
Object.assign(ruMirror, ruMirrorTranslated);

const additionsPath = path.join(__dirname, "locale-additions.mjs");
replaceExportBlock(additionsPath, "additionalFrOverrides", frMirror);
replaceExportBlock(additionsPath, "additionalRuOverrides", ruMirror);

// Rebuild and report
execSync("node scripts/build-locales.mjs", { cwd: root, stdio: "inherit" });
execSync("node scripts/check-untranslated.mjs", { cwd: root, stdio: "inherit" });

console.log("Done!");
