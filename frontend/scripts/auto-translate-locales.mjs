import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const PROPER_NOUNS = [
  "MagicHandy",
  "Handy",
  "Intiface",
  "Ollama",
  "Autospeak",
  "StrokeGPT",
  "BPM",
  "JSON",
  "HSSP",
  "TTS",
  "STT",
  "GPU",
  "WebSocket",
  "funscript",
  "Edge-TTS",
  "Whisper",
  "Ctrl+Enter",
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
  if (res.status === 429 && attempt < 5) {
    await sleep(5000 * (attempt + 1));
    return translateText(text, targetLang, attempt + 1);
  }
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  const data = await res.json();
  if (data.responseStatus !== 200) throw new Error(data.responseDetails || "translate failed");
  return restoreTerms(data.responseData?.translatedText ?? text, tokens);
}

function formatExport(name, obj) {
  const lines = Object.entries(obj)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([k, v]) => `  ${JSON.stringify(k)}: ${JSON.stringify(v)},`);
  return `/** Auto-generated locale overrides */\nexport const ${name} = {\n${lines.join("\n")}\n};\n`;
}

async function translateMap(entries, targetLang, label, existing = {}) {
  const out = { ...existing };
  const keys = Object.keys(entries).filter((k) => !out[k] || out[k] === entries[k]);
  for (let i = 0; i < keys.length; i++) {
    const key = keys[i];
    const en = entries[key];
    try {
      out[key] = await translateText(en, targetLang);
    } catch (err) {
      console.warn(`[${label}] ${key}: ${err.message}`);
      out[key] = en;
    }
    if ((i + 1) % 10 === 0 || i === keys.length - 1) {
      console.log(`[${label}] ${i + 1}/${keys.length} (total ${Object.keys(out).length})`);
      fs.writeFileSync(path.join(__dirname, `_${label}-progress.json`), JSON.stringify(out, null, 2));
    }
    await sleep(1200);
  }
  return out;
}

const ruMissing = JSON.parse(fs.readFileSync(path.join(__dirname, "_ru-missing.json"), "utf8"));
const ptMissing = JSON.parse(fs.readFileSync(path.join(__dirname, "_pt-missing.json"), "utf8"));

let ruExisting = {};
try {
  ruExisting = JSON.parse(fs.readFileSync(path.join(__dirname, "_ru-progress.json"), "utf8"));
} catch {}

const ruOut = await translateMap(ruMissing, "ru", "ru", ruExisting);
const ptOut = await translateMap(ptMissing, "pt", "pt");

fs.writeFileSync(path.join(__dirname, "locale-overrides-ru.mjs"), formatExport("ruFullOverrides", ruOut));
fs.writeFileSync(path.join(__dirname, "locale-overrides-pt.mjs"), formatExport("ptRemainingOverrides", ptOut));
console.log("Done. RU:", Object.keys(ruOut).length, "PT:", Object.keys(ptOut).length);
