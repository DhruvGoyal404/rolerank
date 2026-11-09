import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

// The production CSP (deploy/Caddyfile) must be the policy the e2e suite runs under
// (web/vite.config.ts), so a page that passes e2e can't break in production.
const directives = (policy) => new Map(policy.split(";").map((d) => d.trim()).filter(Boolean).map((d) => [d.split(/\s+/)[0], d]));

const vite = readFileSync(new URL("../web/vite.config.ts", import.meta.url), "utf8");
const caddy = readFileSync(new URL("../deploy/Caddyfile", import.meta.url), "utf8");
const devPolicy = [...vite.match(/const CSP = \[([\s\S]*?)\]/)[1].matchAll(/"([^"]+)"/g)].map((m) => m[1]).join("; ");
const prodPolicy = caddy.match(/header Content-Security-Policy "([^"]+)"/)[1];

test("the production CSP equals the e2e CSP, plus upgrade-insecure-requests", () => {
  const prod = directives(prodPolicy);
  assert.equal(prod.get("upgrade-insecure-requests"), "upgrade-insecure-requests");
  prod.delete("upgrade-insecure-requests");
  assert.deepEqual([...prod].sort(), [...directives(devPolicy)].sort());
});

test("the production CSP allows the web manifest and Trusted Types are required", () => {
  const prod = directives(prodPolicy);
  assert.equal(prod.get("manifest-src"), "manifest-src 'self'");
  assert.equal(prod.get("script-src"), "script-src 'self'");
  assert.equal(prod.get("require-trusted-types-for"), "require-trusted-types-for 'script'");
});
