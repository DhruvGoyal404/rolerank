import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import {
  ageErrors,
  expiredExceptions,
  goItems,
  npmItems,
  npmPinErrors,
  parseExceptions,
} from "./check-deps.mjs";

const now = "2026-10-31T00:00:00Z";
const OLD = "2026-01-01T00:00:00Z";
const YOUNG = "2026-10-25T00:00:00Z";
const header =
  "| Ecosystem | Package | Version | Published | Advisory | Recorded | Expires |\n|---|---|---|---|---|---|---|\n";
const npmItem = { ecosystem: "npm", name: "a", version: "1.0.0" };
const times = (published, ecosystem = "npm", name = "a", version = "1.0.0") => ({
  [ecosystem]: { [name]: { [version]: published } },
});

test("passes an old package", () => {
  const { errors } = ageErrors({ items: [npmItem], times: times(OLD), exceptions: [], now });
  assert.deepEqual(errors, []);
});

test("fails a package under 14 days old", () => {
  const { errors } = ageErrors({ items: [npmItem], times: times(YOUNG), exceptions: [], now });
  assert.match(errors[0], /under 14 days old/);
});

test("reads transitive npm packages, so a too-new one fails too", () => {
  const lock = { packages: { "": {}, "node_modules/b/node_modules/c": { version: "2.0.0" } } };
  const items = npmItems(lock);
  assert.deepEqual(items, [{ ecosystem: "npm", name: "c", version: "2.0.0" }]);
  const { errors } = ageErrors({ items, times: times(YOUNG, "npm", "c", "2.0.0"), exceptions: [], now });
  assert.match(errors[0], /c@2.0.0/);
});

test("fails a range instead of an exact pin", () => {
  for (const spec of ["^1.0.0", "~1.0.0", "latest", "1.x"]) {
    assert.match(npmPinErrors({ dependencies: { a: spec } })[0], /not an exact pin/);
  }
  assert.deepEqual(npmPinErrors({ dependencies: { a: "1.0.0" } }), []);
});

test("a matching exception allows a young package", () => {
  const exceptions = [{ ecosystem: "npm", name: "a", version: "1.0.0", expires: "2026-11-08" }];
  const result = ageErrors({ items: [npmItem], times: times(YOUNG), exceptions, now });
  assert.deepEqual(result.errors, []);
  assert.deepEqual(result.used, ["npm a@1.0.0"]);
});

test("an exception for another version or ecosystem doesn't count", () => {
  for (const other of [
    { ecosystem: "npm", name: "a", version: "9.9.9" },
    { ecosystem: "go", name: "a", version: "1.0.0" },
  ]) {
    const { errors } = ageErrors({ items: [npmItem], times: times(YOUNG), exceptions: [other], now });
    assert.equal(errors.length, 1);
  }
});

test("fails when a publish date can't be found", () => {
  const { errors } = ageErrors({ items: [npmItem], times: {}, exceptions: [], now });
  assert.match(errors[0], /no publish date/);
});

test("reads the Go toolchain and every required module", () => {
  const items = goItems(`module x

go 1.27.3

toolchain go1.27.9

require github.com/a/b v1.0.0

require (
\tgithub.com/C/d v2.0.0 // indirect
\tgolang.org/x/sys v0.47.0
)
`);
  assert.deepEqual(items, [
    { ecosystem: "go-toolchain", name: "go", version: "1.27.3" },
    { ecosystem: "go-toolchain", name: "go", version: "1.27.9" },
    { ecosystem: "go", name: "github.com/a/b", version: "v1.0.0" },
    { ecosystem: "go", name: "github.com/C/d", version: "v2.0.0" },
    { ecosystem: "go", name: "golang.org/x/sys", version: "v0.47.0" },
  ]);
});

test("applies the age rule to Go modules and the toolchain", () => {
  const items = [{ ecosystem: "go-toolchain", name: "go", version: "1.27.3" }];
  const young = times(YOUNG, "go-toolchain", "go", "1.27.3");
  assert.match(ageErrors({ items, times: young, exceptions: [], now }).errors[0], /under 14 days/);
  const exceptions = [{ ecosystem: "go-toolchain", name: "go", version: "1.27.3" }];
  assert.deepEqual(ageErrors({ items, times: young, exceptions, now }).errors, []);
});

test("every ecosystem's lapsed exception fails, even if the package is now old enough", () => {
  for (const ecosystem of ["npm", "go", "go-toolchain"]) {
    const rows = [{ ecosystem, name: "a", version: "1.0.0", expires: "2026-10-30" }];
    assert.match(expiredExceptions(rows, now)[0], /expired on 2026-10-30/);
  }
});

test("an exception holds through its expiry day, then lapses", () => {
  const rows = [{ ecosystem: "go", name: "a", version: "1", expires: "2026-11-02" }];
  assert.deepEqual(expiredExceptions(rows, "2026-11-02T23:00:00Z"), []);
  assert.equal(expiredExceptions(rows, "2026-11-03T00:00:00Z").length, 1);
});

test("reads the table", () => {
  const { rows, errors } = parseExceptions(
    `${header}| go-toolchain | go | 1.27.3 | 2026-10-19 | GO-2026-6809 | 2026-10-31 | 2026-11-02 |\n`,
  );
  assert.deepEqual(errors, []);
  assert.deepEqual(rows, [
    { ecosystem: "go-toolchain", name: "go", version: "1.27.3", advisory: "GO-2026-6809", expires: "2026-11-02" },
  ]);
});

test("a row it can't read is an error, never silently skipped", () => {
  for (const row of [
    "| pypi | a | 1 | 2026-10-19 | GHSA-x | 2026-10-31 | 2026-11-02 |",
    "| npm | a | 1 | 2026-10-19 | GHSA-x | 2026-10-31 | soon |",
    "| npm | a | 1 | 2026-10-19 | | 2026-10-31 | 2026-11-02 |",
    "| npm | a | 1 |",
  ]) {
    const { rows, errors } = parseExceptions(`${header}${row}\n`);
    assert.equal(rows.length, 0, row);
    assert.equal(errors.length, 1, row);
  }
});

function runCli(exceptionRow) {
  const dir = mkdtempSync(join(tmpdir(), "check-deps-"));
  writeFileSync(join(dir, "go.mod"), "module x\n\ngo 1.27.3\n");
  writeFileSync(join(dir, "ex.md"), `${header}${exceptionRow}\n`);
  writeFileSync(
    join(dir, "times.json"),
    JSON.stringify({ "go-toolchain": { go: { "1.27.3": "2026-10-19T00:00:00Z" } } }),
  );
  return spawnSync(
    process.execPath,
    ["scripts/check-deps.mjs", "--now", "2026-11-03T00:00:00Z", "--times", join(dir, "times.json"), "--exceptions", join(dir, "ex.md"), dir],
    { encoding: "utf8" },
  );
}

test("the real command exits 1 on a lapsed Go exception", () => {
  const lapsed = "| go-toolchain | go | 1.27.3 | 2026-10-19 | GO-2026-6809 | 2026-10-31 | 2026-11-02 |";
  const result = runCli(lapsed);
  assert.equal(result.status, 1);
  assert.match(result.stderr, /exception expired on 2026-11-02/);
});

test("the real command exits 0 once the lapsed row is gone and the version has aged", () => {
  assert.equal(runCli("").status, 0);
});
