import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import ts from "typescript";

const root = path.resolve(import.meta.dirname, "..");
const sourceRoot = path.join(root, "src");
const localeRoot = path.join(sourceRoot, "i18n", "locales");
const localeNames = ["en", "es", "pt-BR", "zh-Hans", "ja"];
const visibleAttributes = new Set(["alt", "aria-label", "aria-valuetext", "placeholder", "title", "label", "hint", "accessibleLabel", "unavailableTitle"]);
const iconText = /^(?:M|Y|MH|MagicHandy|[\s+\-−–—×%/#:;,.…•·]+|\d+(?:\.\d+)?%?)$/u;
const sameValueAllowed = {
  es: new Set(["CRF {value}", "{rate} kbps", "{count} tokens", "{count} videos", "{rounding} ms", "{seconds} s", "{size} / {location}", "{state}: {message}", "1 video", "Autopilot", "Chat", "Commit", "Error", "Esc", "Funscript", "General", "Intiface Central", "local / {owner}", "MagicHandy", "NeuTTS Air", "Normal", "Original", "script", "Vagina / vulva", "Video", "Videos", "Vulnerable"]),
  "pt-BR": new Set(["CRF {value}", "{rate} kbps", "{count} tokens", "{rounding} ms", "{seconds} s", "{size} / {location}", "{state}: {message}", "Autopilot", "Chat", "Commit", "Esc", "Funscript", "Interface", "Intiface Central", "local / {owner}", "MagicHandy", "NeuTTS Air", "Normal", "Original", "script", "Status", "Tags", "Vagina / vulva"]),
  "zh-Hans": new Set(["CRF {value}", "{rate} kbps", "{rounding} ms", "{size} / {location}", "{state}: {message}", "Esc", "Funscript", "Intiface Central", "MagicHandy", "NeuTTS Air"]),
  ja: new Set(["CRF {value}", "{rate} kbps", "{rounding} ms", "{size} / {location}", "{state}: {message}", "Autopilot", "Esc", "Funscript", "Intiface Central", "MagicHandy", "NeuTTS Air"]),
};

function walk(directory) {
  return fs.readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const full = path.join(directory, entry.name);
    if (entry.isDirectory()) return entry.name === "dist" ? [] : walk(full);
    return /\.[cm]?[jt]sx?$/.test(entry.name) && !entry.name.endsWith(".test.tsx") && !entry.name.endsWith(".test.ts") ? [full] : [];
  });
}

function meaningful(value) {
  const normalized = value.replace(/\s+/g, " ").trim();
  return normalized && !iconText.test(normalized) ? normalized : "";
}

function renderedStrings(expression) {
  if (!expression) return [];
  if (ts.isStringLiteralLike(expression) || ts.isNoSubstitutionTemplateLiteral(expression)) {
    return meaningful(expression.text) ? [expression.text] : [];
  }
  if (ts.isTemplateExpression(expression)) return [expression.getText()];
  if (ts.isConditionalExpression(expression)) {
    return [...renderedStrings(expression.whenTrue), ...renderedStrings(expression.whenFalse)];
  }
  if (ts.isParenthesizedExpression(expression) || ts.isAsExpression(expression)) return renderedStrings(expression.expression);
  if (ts.isArrayLiteralExpression(expression)) return expression.elements.flatMap(renderedStrings);
  if (ts.isBinaryExpression(expression)) {
    if (expression.operatorToken.kind === ts.SyntaxKind.PlusToken) return [...renderedStrings(expression.left), ...renderedStrings(expression.right)];
    if (expression.operatorToken.kind === ts.SyntaxKind.AmpersandAmpersandToken || expression.operatorToken.kind === ts.SyntaxKind.BarBarToken || expression.operatorToken.kind === ts.SyntaxKind.QuestionQuestionToken) return renderedStrings(expression.right);
  }
  if (ts.isCallExpression(expression) && ts.isPropertyAccessExpression(expression.expression)) {
    const method = expression.expression.name.text;
    if (method === "join" || method === "filter") return renderedStrings(expression.expression.expression);
  }
  return [];
}
function directTranslation(expression) {
  if (!expression) return false;
  if (ts.isParenthesizedExpression(expression) || ts.isAsExpression(expression)) return directTranslation(expression.expression);
  return ts.isCallExpression(expression)
    && ts.isIdentifier(expression.expression)
    && expression.expression.text === "t";
}

function plainTextExpression(expression) {
  if (!expression) return false;
  if (ts.isParenthesizedExpression(expression) || ts.isAsExpression(expression)) return plainTextExpression(expression.expression);
  if (ts.isJsxElement(expression) || ts.isJsxSelfClosingElement(expression) || ts.isJsxFragment(expression)) return false;
  if (ts.isConditionalExpression(expression)) return plainTextExpression(expression.whenTrue) || plainTextExpression(expression.whenFalse);
  if (ts.isBinaryExpression(expression)
    && (expression.operatorToken.kind === ts.SyntaxKind.AmpersandAmpersandToken
      || expression.operatorToken.kind === ts.SyntaxKind.BarBarToken)) {
    return plainTextExpression(expression.right);
  }
  return true;
}
const catalogErrors = [];
function readCatalog(locale) {
  const file = path.join(localeRoot, `${locale}.json`);
  const raw = fs.readFileSync(file, "utf8");
  const sourceFile = ts.parseJsonText(file, raw);
  const rootExpression = sourceFile.statements[0]?.expression;
  if (rootExpression && ts.isObjectLiteralExpression(rootExpression)) {
    const seen = new Map();
    for (const property of rootExpression.properties) {
      if (!ts.isPropertyAssignment(property) || !ts.isStringLiteralLike(property.name)) continue;
      const key = property.name.text;
      const line = sourceFile.getLineAndCharacterOfPosition(property.name.getStart(sourceFile)).line + 1;
      const firstLine = seen.get(key);
      if (firstLine !== undefined) {
        catalogErrors.push(`${locale}: duplicate catalog key ${JSON.stringify(key)} at line ${line} (first defined at line ${firstLine})`);
      } else {
        seen.set(key, line);
      }
    }
  }
  return JSON.parse(raw);
}

const catalogs = Object.fromEntries(localeNames.map((locale) => [
  locale,
  readCatalog(locale),
]));
const englishKeys = Object.keys(catalogs.en).sort();
const errors = [...catalogErrors];
const used = new Map();

for (const locale of localeNames.slice(1)) {
  const keys = Object.keys(catalogs[locale]).sort();
  const missing = englishKeys.filter((key) => !(key in catalogs[locale]));
  const extra = keys.filter((key) => !(key in catalogs.en));
  if (missing.length) errors.push(`${locale}: missing catalog keys:\n  ${missing.join("\n  ")}`);
  if (extra.length) errors.push(`${locale}: extra catalog keys:\n  ${extra.join("\n  ")}`);
  for (const key of englishKeys) {
    if (!(key in catalogs[locale])) continue;
    if (typeof catalogs[locale][key] !== "string" || !catalogs[locale][key].trim()) {
      errors.push(`${locale}: empty translation for ${JSON.stringify(key)}`);
    }
    if (catalogs[locale][key] === catalogs.en[key] && /[A-Za-z]{2}/.test(key) && !sameValueAllowed[locale].has(key)) {
      errors.push(`${locale}: untranslated English value for ${JSON.stringify(key)}`);
    }
    if (/[ÃÂ�]/.test(catalogs[locale][key])) {
      errors.push(`${locale}: possible encoding corruption for ${JSON.stringify(key)}`);
    }
    const expected = [...catalogs.en[key].matchAll(/\{([A-Za-z0-9_]+)\}/g)].map((match) => match[1]).sort();
    const actual = [...catalogs[locale][key].matchAll(/\{([A-Za-z0-9_]+)\}/g)].map((match) => match[1]).sort();
    if (expected.join("\0") !== actual.join("\0")) {
      errors.push(`${locale}: placeholder mismatch for ${JSON.stringify(key)}; expected ${expected.join(", ") || "none"}, found ${actual.join(", ") || "none"}`);
    }
  }
}

for (const file of walk(sourceRoot)) {
  if (file.includes(`${path.sep}i18n${path.sep}`)) continue;
  const source = fs.readFileSync(file, "utf8");
  const sourceFile = ts.createSourceFile(file, source, ts.ScriptTarget.Latest, true, file.endsWith("x") ? ts.ScriptKind.TSX : ts.ScriptKind.TS);
  const relative = path.relative(root, file).replaceAll(path.sep, "/");
  const report = (node, message) => {
    const { line, character } = sourceFile.getLineAndCharacterOfPosition(node.getStart(sourceFile));
    errors.push(`${relative}:${line + 1}:${character + 1}: ${message}`);
  };
  const visit = (node) => {
    if (ts.isCallExpression(node)
      && ts.isPropertyAccessExpression(node.expression)
      && node.expression.name.text === "confirm"
      && node.arguments.length > 0) {
      for (const value of renderedStrings(node.arguments[0])) report(node.arguments[0], `raw confirmation string ${JSON.stringify(value)}`);
    }
    if (ts.isCallExpression(node)
      && ts.isIdentifier(node.expression)
      && node.expression.text === "show"
      && node.arguments.length > 0) {
      for (const value of renderedStrings(node.arguments[0])) report(node.arguments[0], `raw toast string ${JSON.stringify(value)}`);
    }
    if (ts.isCallExpression(node)
      && ts.isIdentifier(node.expression)
      && node.expression.text === "t"
      && node.arguments.length > 0) {
      const keyNode = node.arguments[0];
      if (!ts.isStringLiteralLike(keyNode)) report(keyNode, "t() key must be a string literal");
      else {
        const key = keyNode.text;
        if (!(key in catalogs.en)) report(keyNode, `missing English catalog key ${JSON.stringify(key)}`);
        const locations = used.get(key) ?? [];
        locations.push(`${relative}:${sourceFile.getLineAndCharacterOfPosition(keyNode.getStart(sourceFile)).line + 1}`);
        used.set(key, locations);
      }
    }
    if (ts.isJsxElement(node) || ts.isJsxFragment(node)) {
      for (let index = 1; index < node.children.length; index += 1) {
        const previous = node.children[index - 1];
        const current = node.children[index];
        if (ts.isJsxExpression(previous) && ts.isJsxExpression(current)
          && ((directTranslation(previous.expression) && plainTextExpression(current.expression))
            || (directTranslation(current.expression) && plainTextExpression(previous.expression)))) {
          report(current, "adjacent translated/dynamic JSX fragments must use one complete t() template");
        }
      }
    }
    if (ts.isJsxText(node)) {
      const text = meaningful(node.getText(sourceFile));
      if (text) report(node, `raw JSX text ${JSON.stringify(text)}`);
    }
    if (ts.isJsxExpression(node) && node.parent && (ts.isJsxElement(node.parent) || ts.isJsxFragment(node.parent))) {
      for (const value of renderedStrings(node.expression)) report(node, `raw rendered string ${JSON.stringify(value)}`);
    }
    if (ts.isJsxAttribute(node) && visibleAttributes.has(node.name.getText(sourceFile))) {
      if (node.initializer && ts.isStringLiteral(node.initializer)) {
        const text = meaningful(node.initializer.text);
        if (text) report(node, `raw ${node.name.getText(sourceFile)} text ${JSON.stringify(text)}`);
      } else if (node.initializer && ts.isJsxExpression(node.initializer)) {
        for (const value of renderedStrings(node.initializer.expression)) report(node, `raw ${node.name.getText(sourceFile)} string ${JSON.stringify(value)}`);
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(sourceFile);
}

const unused = englishKeys.filter((key) => !used.has(key));
if (errors.length) {
  console.error(`Localization audit failed with ${errors.length} issue(s):\n\n${errors.join("\n\n")}`);
  process.exit(1);
}
console.log(`Localization audit passed: ${englishKeys.length} keys, ${used.size} compile-time UI keys, ${unused.length} status/documented keys, ${localeNames.length} locales.`);
