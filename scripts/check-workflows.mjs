#!/usr/bin/env node
// Lints GitHub workflows: actions pinned by commit SHA, images by digest, and
// `set -o pipefail` in any step that pipes. A plain line scan, on purpose: no YAML
// dependency, and the patterns it checks are simple.
//
//   node scripts/check-workflows.mjs [file...]   (default: .github/workflows/*.yml)
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";

const SHA = /@[0-9a-f]{40}(\s|$)/;

/** Returns each `run:` body with the line it starts on. */
export function runBlocks(lines) {
  const blocks = [];
  for (let i = 0; i < lines.length; i += 1) {
    const match = /^(\s*)(?:- )?run:\s*(.*)$/.exec(lines[i]);
    if (!match) continue;
    const indent = match[1].length;
    const value = match[2];
    if (!/^[|>][-+]?\s*$/.test(value)) {
      blocks.push({ line: i + 1, text: value });
      continue;
    }
    const body = [];
    let j = i + 1;
    for (; j < lines.length; j += 1) {
      const current = lines[j];
      if (current.trim() !== "" && current.length - current.trimStart().length <= indent) break;
      body.push(current);
    }
    blocks.push({ line: i + 1, text: body.join("\n") });
    i = j - 1;
  }
  return blocks;
}

// A pipe, not `||`. Strips quoted strings first, so a `|` in a message doesn't count.
export function hasPipe(text) {
  const bare = text.replace(/'[^']*'/g, "").replace(/"[^"]*"/g, "");
  return /(^|[^|])\|([^|]|$)/.test(bare);
}

export function lint(source) {
  const errors = [];
  const lines = source.split("\n");
  lines.forEach((line, index) => {
    const uses = /^\s*(?:- )?uses:\s*(\S+)/.exec(line);
    if (uses && !uses[1].startsWith("./") && !SHA.test(uses[1])) {
      errors.push(`line ${index + 1}: ${uses[1]} is not pinned to a commit SHA`);
    }
    const image = /docker run\b.*?\s([\w./-]+:[\w.-]+)(\s|$)/.exec(line);
    if (image && !line.includes("@sha256:")) {
      errors.push(`line ${index + 1}: ${image[1]} is not pinned by digest`);
    }
    if (/\bpull_request_target\b/.test(line)) {
      errors.push(`line ${index + 1}: pull_request_target is banned`);
    }
  });
  for (const block of runBlocks(lines)) {
    if (hasPipe(block.text) && !/set -[a-z]*o pipefail/.test(block.text)) {
      errors.push(`line ${block.line}: piped step needs set -o pipefail`);
    }
  }
  return errors;
}

function main() {
  const dir = ".github/workflows";
  const args = process.argv.slice(2);
  const files = args.length > 0 ? args : readdirSync(dir).map((file) => join(dir, file));
  let failed = false;
  for (const file of files) {
    const errors = lint(readFileSync(file, "utf8"));
    console.log(`${file}: ${errors.length === 0 ? "ok" : `${errors.length} problems`}`);
    for (const error of errors) console.error(`  ${error}`);
    failed ||= errors.length > 0;
  }
  if (failed) process.exit(1);
}

if (process.argv[1]?.endsWith("check-workflows.mjs")) main();
