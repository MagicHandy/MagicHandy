// Fails when a component uses a CSS class whose only definition lives in a
// stylesheet that the component's chunk does not load.
//
// Vite emits a lazily loaded route's stylesheet only when that route's chunk
// loads. A component rendered somewhere else that relies on those classes then
// renders unstyled until the person happens to visit the owning route first.
// (alpha.89 shipped exactly that: Autopilot in Chat, the persona switcher, and
// the device connection choices lost their styles.)
//
// The check models the bundler: entries are main.tsx and every dynamically
// imported module; an entry loads the stylesheets in its static import closure
// plus everything main.tsx loads. Every module in an entry's closure must find
// each class its className attributes name, if that class is defined only in
// lazily loaded stylesheets, in a stylesheet that entry loads.
import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import ts from "typescript";

const root = path.resolve(import.meta.dirname, "..");
const src = path.join(root, "src");

function walk(directory) {
  return fs.readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const full = path.join(directory, entry.name);
    if (entry.isDirectory()) return entry.name === "dist" ? [] : walk(full);
    return /\.tsx?$/.test(entry.name) && !/\.test\.tsx?$/.test(entry.name) ? [full] : [];
  });
}

const posix = (file) => path.relative(src, file).replaceAll("\\", "/");
const files = walk(src);
const known = new Set(files.map(posix));

function resolve(from, specifier) {
  if (!specifier.startsWith(".")) return null;
  const base = path.posix.normalize(path.posix.join(path.posix.dirname(from), specifier));
  if (base.endsWith(".css")) return fs.existsSync(path.join(src, base)) ? base : null;
  for (const candidate of [base, `${base}.tsx`, `${base}.ts`, `${base}/index.tsx`, `${base}/index.ts`]) {
    if (known.has(candidate)) return candidate;
  }
  return null;
}

// Every string a className expression can produce: literals and the literal
// parts of templates, through conditionals, joins and helper calls.
function classTokens(node, out) {
  if (ts.isStringLiteralLike(node)) out.push(...node.text.split(/\s+/));
  else if (ts.isTemplateExpression(node)) {
    out.push(...node.head.text.split(/\s+/));
    for (const span of node.templateSpans) {
      classTokens(span.expression, out);
      out.push(...span.literal.text.split(/\s+/));
    }
  } else node.forEachChild((child) => classTokens(child, out));
}

const graph = new Map();
for (const file of files) {
  const name = posix(file);
  const source = ts.createSourceFile(file, fs.readFileSync(file, "utf8"), ts.ScriptTarget.Latest, true, name.endsWith(".tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS);
  const node = { statics: [], dynamics: [], styles: [], classes: new Set() };
  const visit = (child) => {
    if (ts.isImportDeclaration(child) && !child.importClause?.isTypeOnly && ts.isStringLiteral(child.moduleSpecifier)) {
      const target = resolve(name, child.moduleSpecifier.text);
      if (target) (target.endsWith(".css") ? node.styles : node.statics).push(target);
    } else if (ts.isExportDeclaration(child) && !child.isTypeOnly && child.moduleSpecifier && ts.isStringLiteral(child.moduleSpecifier)) {
      const target = resolve(name, child.moduleSpecifier.text);
      if (target) node.statics.push(target);
    } else if (ts.isCallExpression(child) && child.expression.kind === ts.SyntaxKind.ImportKeyword && child.arguments[0] && ts.isStringLiteral(child.arguments[0])) {
      const target = resolve(name, child.arguments[0].text);
      if (target) node.dynamics.push(target);
    } else if (ts.isJsxAttribute(child) && child.name.getText(source) === "className" && child.initializer) {
      const tokens = [];
      classTokens(child.initializer, tokens);
      for (const token of tokens) if (token) node.classes.add(token);
    }
    child.forEachChild(visit);
  };
  source.forEachChild(visit);
  graph.set(name, node);
}

function closure(entry) {
  const seen = new Set();
  const queue = [entry];
  while (queue.length) {
    const file = queue.pop();
    if (seen.has(file) || !graph.has(file)) continue;
    seen.add(file);
    queue.push(...graph.get(file).statics);
  }
  return seen;
}

const entries = new Set(["main.tsx"]);
for (const queue = ["main.tsx"]; queue.length;) {
  for (const file of closure(queue.pop())) {
    for (const target of graph.get(file).dynamics) {
      if (!entries.has(target)) {
        entries.add(target);
        queue.push(target);
      }
    }
  }
}

const closures = new Map([...entries].map((entry) => [entry, closure(entry)]));
const stylesOf = (members) => new Set([...members].flatMap((file) => graph.get(file).styles));
const globalStyles = stylesOf(closures.get("main.tsx"));

function classesIn(sheet) {
  const css = fs.readFileSync(path.join(src, sheet), "utf8").replace(/\/\*[\s\S]*?\*\//g, "");
  const names = new Set();
  for (const selector of css.split("{").map((part) => part.split("}").pop())) {
    for (const match of selector.matchAll(/\.([a-zA-Z][\w-]*)/g)) names.add(match[1]);
  }
  return names;
}

const allStyles = new Set([...graph.values()].flatMap((node) => node.styles));
const globalClasses = new Set([...globalStyles].flatMap((sheet) => [...classesIn(sheet)]));
// class -> lazily loaded sheets that define it; globally defined classes are never at risk
const lazyClasses = new Map();
for (const sheet of allStyles) {
  if (globalStyles.has(sheet)) continue;
  for (const name of classesIn(sheet)) {
    if (globalClasses.has(name)) continue;
    if (!lazyClasses.has(name)) lazyClasses.set(name, new Set());
    lazyClasses.get(name).add(sheet);
  }
}

const failures = new Map();
for (const [entry, members] of closures) {
  const loaded = new Set([...globalStyles, ...stylesOf(members)]);
  for (const file of members) {
    for (const name of graph.get(file).classes) {
      const sheets = lazyClasses.get(name);
      if (!sheets || [...sheets].some((sheet) => loaded.has(sheet))) continue;
      const key = `${file} → ${[...sheets].join(", ")}`;
      if (!failures.has(key)) failures.set(key, { classes: new Set(), entries: new Set() });
      failures.get(key).classes.add(name);
      failures.get(key).entries.add(entry === "main.tsx" ? "app shell" : entry);
    }
  }
}

if (failures.size) {
  console.error("Route stylesheet check failed. These components use classes whose stylesheet their chunk does not load:");
  for (const [key, detail] of [...failures].sort()) {
    console.error(`  ${key}\n    classes: .${[...detail.classes].sort().join(" .")}\n    rendered from: ${[...detail.entries].sort().join(", ")}`);
  }
  console.error("Import the stylesheet in the component that uses the class, so it loads wherever the component renders.");
  process.exit(1);
}
console.log(`Route stylesheet check passed (${entries.size} entries, ${lazyClasses.size} classes in lazily loaded stylesheets).`);
