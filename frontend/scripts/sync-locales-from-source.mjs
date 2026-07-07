import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const srcDir = path.join(__dirname, "../src");
const localesDir = path.join(__dirname, "../src/i18n/locales");

function walk(dir, files = []) {
  for (const ent of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, ent.name);
    if (ent.isDirectory()) walk(p, files);
    else if (/\.(tsx?|jsx?)$/.test(ent.name)) files.push(p);
  }
  return files;
}

function set(obj, keyPath, value) {
  const parts = keyPath.split(".");
  let cur = obj;
  for (let i = 0; i < parts.length - 1; i++) {
    cur[parts[i]] ??= {};
    cur = cur[parts[i]];
  }
  if (cur[parts[parts.length - 1]] === undefined) {
    cur[parts[parts.length - 1]] = value;
  }
}

function keyToEnglish(key) {
  const last = key.split(".").pop() ?? key;
  return last
    .replace(/([A-Z])/g, " $1")
    .replace(/[-_]/g, " ")
    .replace(/\s+/g, " ")
    .trim()
    .replace(/^\w/, (c) => c.toUpperCase());
}

function flatten(obj, prefix = "", out = {}) {
  for (const [k, v] of Object.entries(obj)) {
    const p = prefix ? `${prefix}.${k}` : k;
    if (v && typeof v === "object" && !Array.isArray(v)) flatten(v, p, out);
    else out[p] = v;
  }
  return out;
}

function fromFlat(flat) {
  const obj = {};
  for (const [k, v] of Object.entries(flat)) set(obj, k, v);
  return obj;
}

const keyRe = /t\(\s*["'`]([^"'`]+)["'`]/g;
const keys = new Set();
for (const file of walk(srcDir)) {
  const text = fs.readFileSync(file, "utf8");
  let m;
  while ((m = keyRe.exec(text))) keys.add(m[1]);
}

const enPath = path.join(localesDir, "en.json");
const en = JSON.parse(fs.readFileSync(enPath, "utf8"));
const flat = flatten(en);
for (const key of keys) {
  if (!(key in flat)) flat[key] = keyToEnglish(key);
}
const newEn = fromFlat(flat);
fs.writeFileSync(enPath, JSON.stringify(newEn, null, 2) + "\n");

// copy en structure to pt/fr/ru (pt gets manual overrides from build-locales)
import { spawnSync } from "child_process";
spawnSync("node", ["scripts/build-locales.mjs"], { cwd: path.join(__dirname, ".."), stdio: "inherit" });

console.log(`Synced ${keys.size} keys from source; en.json has ${Object.keys(flat).length} entries`);
