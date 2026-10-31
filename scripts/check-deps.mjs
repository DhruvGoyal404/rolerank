#!/usr/bin/env node
// Enforces the dependency rules in CLAUDE.md across every ecosystem we use: exact pins,
// and every package at least 14 days old unless docs/DEPENDENCY_EXCEPTIONS.md covers it.
// Any exception past its expiry date fails, whether or not it's still needed.
//
//   node scripts/check-deps.mjs [--now 2026-10-31] [--times fixture.json] [--exceptions file] dir...
//
// A directory with go.mod is checked as Go, one with package-lock.json as npm. Adding an
// ecosystem means adding it to ECOSYSTEMS and a reader below.
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";

const DAY_MS = 86_400_000;
const MIN_AGE_DAYS = 14;
const ECOSYSTEMS = new Set(["npm", "go", "go-toolchain"]);
const DATE = /^\d{4}-\d{2}-\d{2}$/;

/** Reads the table. Anything that looks like a row but can't be read is an error. */
export function parseExceptions(markdown) {
  const rows = [];
  const errors = [];
  for (const line of markdown.split("\n")) {
    if (!line.startsWith("|") || /^\|[\s:|-]+\|?$/.test(line) || /^\|\s*Ecosystem\b/.test(line)) continue;
    const cells = line.split("|").slice(1, -1).map((cell) => cell.trim());
    const [ecosystem, name, version, , advisory, , expires] = cells;
    if (cells.length !== 7 || !ECOSYSTEMS.has(ecosystem) || !DATE.test(expires ?? "") || !advisory) {
      errors.push(`exceptions table: can't read "${line}"`);
      continue;
    }
    rows.push({ ecosystem, name, version, advisory, expires });
  }
  return { rows, errors };
}

export function expiredExceptions(rows, now) {
  const nowDate = new Date(now);
  return rows
    .filter((row) => new Date(`${row.expires}T23:59:59Z`) < nowDate)
    .map(
      (row) =>
        `${row.ecosystem} ${row.name}@${row.version}: exception expired on ${row.expires}; remove it or fix the dependency`,
    );
}

export function npmItems(lock) {
  const out = [];
  for (const [path, entry] of Object.entries(lock.packages ?? {})) {
    if (path === "" || !entry.version || entry.link) continue;
    const name = entry.name ?? path.slice(path.lastIndexOf("node_modules/") + "node_modules/".length);
    out.push({ ecosystem: "npm", name, version: entry.version });
  }
  return out;
}

export function npmPinErrors(pkg) {
  const errors = [];
  for (const field of ["dependencies", "devDependencies"]) {
    for (const [name, spec] of Object.entries(pkg[field] ?? {})) {
      if (!/^\d+\.\d+\.\d+$/.test(spec)) errors.push(`${name}: ${spec} is not an exact pin`);
    }
  }
  return errors;
}

/** The toolchain and every required module, indirect ones included. */
export function goItems(goMod) {
  const items = [];
  let block = false;
  for (const raw of goMod.split("\n")) {
    const line = raw.replace(/\/\/.*$/, "").trim();
    const go = /^go\s+(\d+\.\d+(?:\.\d+)?)$/.exec(line);
    const toolchain = /^toolchain\s+go(\S+)$/.exec(line);
    if (go || toolchain) {
      items.push({ ecosystem: "go-toolchain", name: "go", version: (toolchain ?? go)[1] });
    } else if (line === "require (") {
      block = true;
    } else if (block && line === ")") {
      block = false;
    } else {
      const single = /^require\s+(\S+)\s+(\S+)$/.exec(line);
      const inBlock = block ? /^(\S+)\s+(\S+)$/.exec(line) : null;
      const match = single ?? inBlock;
      if (match) items.push({ ecosystem: "go", name: match[1], version: match[2] });
    }
  }
  return items;
}

/** Pure, so the fixtures in scripts/check-deps.test.mjs can drive every failure. */
export function ageErrors({ items, times, exceptions, now }) {
  const errors = [];
  const used = new Set();
  const nowDate = new Date(now);
  for (const { ecosystem, name, version } of items) {
    const key = `${name}@${version}`;
    const published = times[ecosystem]?.[name]?.[version];
    if (!published) {
      errors.push(`${ecosystem} ${key}: no publish date found`);
      continue;
    }
    if ((nowDate - new Date(published)) / DAY_MS >= MIN_AGE_DAYS) continue;
    const exception = exceptions.find(
      (e) => e.ecosystem === ecosystem && e.name === name && e.version === version,
    );
    if (exception) used.add(`${ecosystem} ${key}`);
    else errors.push(`${ecosystem} ${key}: published ${published.slice(0, 10)}, under ${MIN_AGE_DAYS} days old`);
  }
  return { errors, used: [...used] };
}

async function getJson(url) {
  for (let attempt = 1; ; attempt += 1) {
    try {
      const response = await fetch(url);
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      return await response.json();
    } catch (error) {
      if (attempt === 3) throw new Error(`${url}: ${error.message}`, { cause: error });
      await new Promise((resolve) => setTimeout(resolve, 1000 * attempt));
    }
  }
}

const escapeModule = (path) => path.replace(/[A-Z]/g, (c) => `!${c.toLowerCase()}`);

async function publishTime({ ecosystem, name, version }) {
  if (ecosystem === "npm") {
    return (await getJson(`https://registry.npmjs.org/${name.replace("/", "%2f")}`)).time;
  }
  const url =
    ecosystem === "go"
      ? `https://proxy.golang.org/${escapeModule(name)}/@v/${version}.info`
      : `https://proxy.golang.org/golang.org/toolchain/@v/v0.0.1-go${version}.linux-amd64.info`;
  return { [version]: (await getJson(url)).Time };
}

async function fetchTimes(items) {
  const times = {};
  const queue = [...items];
  const worker = async () => {
    for (let item = queue.pop(); item; item = queue.pop()) {
      const byName = (times[item.ecosystem] ??= {});
      if (item.ecosystem === "npm" && byName[item.name]) continue;
      byName[item.name] = { ...byName[item.name], ...(await publishTime(item)) };
    }
  };
  await Promise.all(Array.from({ length: 8 }, worker));
  return times;
}

async function main() {
  const args = process.argv.slice(2);
  const option = (flag) => {
    const index = args.indexOf(flag);
    return index === -1 ? undefined : args.splice(index, 2)[1];
  };
  const now = option("--now") ?? new Date().toISOString();
  const timesFile = option("--times");
  const exceptionsFile = option("--exceptions") ?? "docs/DEPENDENCY_EXCEPTIONS.md";

  const { rows, errors: tableErrors } = parseExceptions(readFileSync(exceptionsFile, "utf8"));
  const problems = [...tableErrors, ...expiredExceptions(rows, now)];

  for (const dir of args) {
    let items;
    if (existsSync(join(dir, "go.mod"))) {
      items = goItems(readFileSync(join(dir, "go.mod"), "utf8"));
    } else {
      const pkg = JSON.parse(readFileSync(join(dir, "package.json"), "utf8"));
      problems.push(...npmPinErrors(pkg));
      items = npmItems(JSON.parse(readFileSync(join(dir, "package-lock.json"), "utf8")));
    }
    const times = timesFile ? JSON.parse(readFileSync(timesFile, "utf8")) : await fetchTimes(items);
    const { errors, used } = ageErrors({ items, times, exceptions: rows, now });
    console.log(`${dir}: ${items.length} packages, exceptions in use: ${used.join(", ") || "none"}`);
    problems.push(...errors);
  }
  for (const problem of problems) console.error(`  ${problem}`);
  if (problems.length > 0) process.exit(1);
}

if (process.argv[1]?.endsWith("check-deps.mjs")) await main();
