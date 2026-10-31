#!/usr/bin/env node
// Enforces the dependency rules in CLAUDE.md on every lockfile: exact pins for direct
// dependencies, and every package at least 14 days old unless a current entry in
// docs/DEPENDENCY_EXCEPTIONS.md covers it.
//
//   node scripts/check-deps.mjs [--now 2026-10-31] [--times fixture.json] [--exceptions file] dir...
import { readFileSync } from "node:fs";
import { join } from "node:path";

const DAY_MS = 86_400_000;
const MIN_AGE_DAYS = 14;

export function parseExceptions(markdown) {
  const rows = [];
  for (const line of markdown.split("\n")) {
    const cells = line.split("|").map((cell) => cell.trim());
    if (cells.length < 8 || !/^\d{4}-\d{2}-\d{2}$/.test(cells[6] ?? "")) continue;
    rows.push({ name: cells[1], version: cells[2], advisory: cells[4], expires: cells[6] });
  }
  return rows;
}

export function installed(lock) {
  const out = [];
  for (const [path, entry] of Object.entries(lock.packages ?? {})) {
    if (path === "" || !entry.version || entry.link) continue;
    const name = entry.name ?? path.slice(path.lastIndexOf("node_modules/") + "node_modules/".length);
    out.push({ name, version: entry.version });
  }
  return out;
}

/** Pure, so the fixtures in scripts/check-deps.test.mjs can drive every failure. */
export function check({ pkg, lock, times, exceptions, now }) {
  const errors = [];
  for (const field of ["dependencies", "devDependencies"]) {
    for (const [name, spec] of Object.entries(pkg[field] ?? {})) {
      if (!/^\d+\.\d+\.\d+$/.test(spec)) errors.push(`${name}: ${spec} is not an exact pin`);
    }
  }
  const nowDate = new Date(now);
  const used = new Set();
  for (const { name, version } of installed(lock)) {
    const key = `${name}@${version}`;
    const published = times[name]?.[version];
    if (!published) {
      errors.push(`${key}: no publish date found`);
      continue;
    }
    const age = (nowDate - new Date(published)) / DAY_MS;
    if (age >= MIN_AGE_DAYS) continue;
    const exception = exceptions.find((e) => e.name === name && e.version === version);
    if (!exception) {
      errors.push(`${key}: published ${published.slice(0, 10)}, under ${MIN_AGE_DAYS} days old`);
    } else if (new Date(`${exception.expires}T23:59:59Z`) < nowDate) {
      errors.push(`${key}: exception expired on ${exception.expires}`);
    } else {
      used.add(`${name}@${version}`);
    }
  }
  return { errors, used: [...used] };
}

async function fetchTimes(names) {
  const times = {};
  const queue = [...names];
  const worker = async () => {
    for (let name = queue.pop(); name; name = queue.pop()) {
      const url = `https://registry.npmjs.org/${name.replace("/", "%2f")}`;
      for (let attempt = 1; ; attempt += 1) {
        try {
          const response = await fetch(url);
          if (!response.ok) throw new Error(`HTTP ${response.status}`);
          times[name] = (await response.json()).time;
          break;
        } catch (error) {
          if (attempt === 3) throw new Error(`${name}: ${error.message}`, { cause: error });
          await new Promise((resolve) => setTimeout(resolve, 1000 * attempt));
        }
      }
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
  const exceptions = parseExceptions(readFileSync(exceptionsFile, "utf8"));

  let failed = false;
  for (const dir of args) {
    const pkg = JSON.parse(readFileSync(join(dir, "package.json"), "utf8"));
    const lock = JSON.parse(readFileSync(join(dir, "package-lock.json"), "utf8"));
    const names = new Set(installed(lock).map((entry) => entry.name));
    const times = timesFile ? JSON.parse(readFileSync(timesFile, "utf8")) : await fetchTimes(names);
    const { errors, used } = check({ pkg, lock, times, exceptions, now });
    console.log(`${dir}: ${names.size} packages, exceptions in use: ${used.join(", ") || "none"}`);
    for (const error of errors) console.error(`  ${error}`);
    failed ||= errors.length > 0;
  }
  if (failed) process.exit(1);
}

if (process.argv[1]?.endsWith("check-deps.mjs")) await main();
