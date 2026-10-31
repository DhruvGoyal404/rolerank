import assert from "node:assert/strict";
import { test } from "node:test";
import { check, parseExceptions } from "./check-deps.mjs";

const now = "2026-10-31T00:00:00Z";
const lock = (name, version) => ({
  packages: { "": { name: "x" }, [`node_modules/${name}`]: { version } },
});
const pkg = (name, version) => ({ dependencies: { [name]: version } });

test("passes an old, exactly pinned package", () => {
  const times = { a: { "1.0.0": "2026-01-01T00:00:00Z" } };
  const result = check({ pkg: pkg("a", "1.0.0"), lock: lock("a", "1.0.0"), times, exceptions: [], now });
  assert.deepEqual(result.errors, []);
});

test("fails a package under 14 days old", () => {
  const times = { a: { "1.0.0": "2026-10-20T00:00:00Z" } };
  const { errors } = check({ pkg: pkg("a", "1.0.0"), lock: lock("a", "1.0.0"), times, exceptions: [], now });
  assert.match(errors[0], /under 14 days old/);
});

test("fails a too-new transitive package too", () => {
  const nested = { packages: { "": {}, "node_modules/b/node_modules/c": { version: "2.0.0" } } };
  const times = { c: { "2.0.0": "2026-10-30T00:00:00Z" } };
  const { errors } = check({ pkg: {}, lock: nested, times, exceptions: [], now });
  assert.match(errors[0], /c@2.0.0/);
});

test("fails a range instead of an exact pin", () => {
  const times = { a: { "1.0.0": "2026-01-01T00:00:00Z" } };
  for (const spec of ["^1.0.0", "~1.0.0", "latest", "1.x"]) {
    const { errors } = check({ pkg: pkg("a", spec), lock: lock("a", "1.0.0"), times, exceptions: [], now });
    assert.match(errors[0], /not an exact pin/);
  }
});

test("a current exception allows a young package, an expired one doesn't", () => {
  const times = { a: { "1.0.0": "2026-10-25T00:00:00Z" } };
  const args = { pkg: pkg("a", "1.0.0"), lock: lock("a", "1.0.0"), times, now };
  const current = [{ name: "a", version: "1.0.0", expires: "2026-11-08" }];
  assert.deepEqual(check({ ...args, exceptions: current }).errors, []);
  const expired = [{ name: "a", version: "1.0.0", expires: "2026-10-30" }];
  assert.match(check({ ...args, exceptions: expired }).errors[0], /expired/);
});

test("an exception for another version doesn't count", () => {
  const times = { a: { "1.0.0": "2026-10-25T00:00:00Z" } };
  const exceptions = [{ name: "a", version: "9.9.9", expires: "2026-11-08" }];
  const { errors } = check({ pkg: pkg("a", "1.0.0"), lock: lock("a", "1.0.0"), times, exceptions, now });
  assert.equal(errors.length, 1);
});

test("fails when a publish date can't be found", () => {
  const { errors } = check({ pkg: pkg("a", "1.0.0"), lock: lock("a", "1.0.0"), times: {}, exceptions: [], now });
  assert.match(errors[0], /no publish date/);
});

test("reads the exceptions table", () => {
  const rows = parseExceptions(
    "| Package | Version | Published | Advisory | Recorded | Expires |\n|---|---|---|---|---|---|\n| body-parser | 2.4.0 | 2026-10-22 | GHSA-x | 2026-10-31 | 2026-11-05 |\n",
  );
  assert.deepEqual(rows, [
    { name: "body-parser", version: "2.4.0", advisory: "GHSA-x", expires: "2026-11-05" },
  ]);
});
