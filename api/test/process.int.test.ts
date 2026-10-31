import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { join } from "node:path";
import { beforeAll, describe, expect, it } from "vitest";

// Runs the real built server, not a function, so the startup guard itself is tested.
const entry = join(import.meta.dirname, "..", "dist", "index.js");

const env = {
  PATH: process.env.PATH,
  PUBLIC_ORIGIN: "https://rolerank.dhruvgoyal.tech",
  APP_DATABASE_URL: "postgres://u:p@127.0.0.1:1/app",
  VALKEY_URL: "redis://127.0.0.1:1",
  GOOGLE_CLIENT_ID: "g",
  GOOGLE_CLIENT_SECRET: "super-secret-google",
  GITHUB_CLIENT_ID: "h",
  GITHUB_CLIENT_SECRET: "super-secret-github",
};

const run = (extra: Record<string, string>): ReturnType<typeof spawnSync> =>
  spawnSync(process.execPath, [entry], { env: { ...env, ...extra }, timeout: 15_000, encoding: "utf8" });

beforeAll(() => {
  expect(existsSync(entry), "build first: npm run build").toBe(true);
});

describe("server startup guards", () => {
  it("refuses TEST_AUTH_SHORTCUT in production", () => {
    const result = run({ NODE_ENV: "production", TEST_AUTH_SHORTCUT: "1" });
    expect(result.status).toBe(1);
    expect(result.stderr).toContain("TEST_AUTH_SHORTCUT");
  });

  it("refuses TEST_AUTH_SHORTCUT in development", () => {
    const result = run({ NODE_ENV: "development", TEST_AUTH_SHORTCUT: "1" });
    expect(result.status).toBe(1);
    expect(result.stderr).toContain("TEST_AUTH_SHORTCUT");
  });

  it("refuses NODE_ENV=test on a database without the _test suffix", () => {
    const result = run({ NODE_ENV: "test", PUBLIC_ORIGIN: "http://localhost:5173" });
    expect(result.status).toBe(1);
    expect(result.stderr).toContain("_test");
  });

  it("prints no secret when it refuses", () => {
    const result = run({ NODE_ENV: "production", TEST_AUTH_SHORTCUT: "1" });
    expect(`${String(result.stdout)}${String(result.stderr)}`).not.toContain("super-secret");
  });
});
