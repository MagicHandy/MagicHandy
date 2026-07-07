import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { execSync } from "child_process";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const localesDir = path.join(__dirname, "../src/i18n/locales");

execSync("node scripts/build-locales.mjs", { cwd: path.join(__dirname, ".."), stdio: "inherit" });

function flatten(obj, prefix = "") {
  const out = {};
  for (const [k, v] of Object.entries(obj)) {
    const key = prefix ? `${prefix}.${k}` : k;
    if (v && typeof v === "object" && !Array.isArray(v)) Object.assign(out, flatten(v, key));
    else out[key] = v;
  }
  return out;
}

const en = flatten(JSON.parse(fs.readFileSync(path.join(localesDir, "en.json"), "utf8")));
const pt = flatten(JSON.parse(fs.readFileSync(path.join(localesDir, "pt.json"), "utf8")));
const fr = flatten(JSON.parse(fs.readFileSync(path.join(localesDir, "fr.json"), "utf8")));
const ru = flatten(JSON.parse(fs.readFileSync(path.join(localesDir, "ru.json"), "utf8")));

const properNouns =
  /MagicHandy|Handy|Intiface|Ollama|Autospeak|StrokeGPT|BPM|JSON|HSSP|TTS|STT|GPU|WebSocket|funscript|Edge-TTS|Whisper|Ctrl\+Enter/i;

function untranslated(target, source) {
  return Object.keys(source).filter((k) => target[k] === source[k] && !properNouns.test(source[k]));
}

const ptU = untranslated(pt, en);
const frU = untranslated(fr, en);
const ruU = untranslated(ru, en);

console.log("Total keys:", Object.keys(en).length);
console.log("PT untranslated:", ptU.length);
console.log("FR untranslated:", frU.length);
console.log("RU untranslated:", ruU.length);

fs.writeFileSync(
  path.join(__dirname, "untranslated-report.json"),
  JSON.stringify({ pt: ptU, fr: frU, ru: ruU, en: Object.fromEntries(ptU.map((k) => [k, en[k]])) }, null, 2)
);
