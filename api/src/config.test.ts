import { describe, expect, it } from "vitest";
import { loadConfig } from "./config.js";

const base = {
  NODE_ENV: "test",
  PUBLIC_ORIGIN: "http://localhost:5173",
  APP_DATABASE_URL: "postgres://u:p@localhost/app_test",
  VALKEY_URL: "redis://localhost:6379",
  GOOGLE_CLIENT_ID: "g",
  GOOGLE_CLIENT_SECRET: "gs",
  GITHUB_CLIENT_ID: "h",
  GITHUB_CLIENT_SECRET: "hs",
};
const production = {
  ...base,
  NODE_ENV: "production",
  PUBLIC_ORIGIN: "https://rolerank.dhruvgoyal.tech",
  APP_DATABASE_URL: "postgres://u:p@db/app",
  ROLERANK_KEK: Buffer.alloc(32, 7).toString("base64"),
};

describe("loadConfig", () => {
  it("accepts a valid test config", () => {
    expect(loadConfig(base).env).toBe("test");
  });

  it("accepts the shortcut only with NODE_ENV=test", () => {
    expect(loadConfig({ ...base, TEST_AUTH_SHORTCUT: "1" }).testAuthShortcut).toBe(true);
    const dev = { ...base, NODE_ENV: "development", APP_DATABASE_URL: "postgres://u:p@h/app" };
    expect(() => loadConfig({ ...dev, TEST_AUTH_SHORTCUT: "1" })).toThrow(/TEST_AUTH_SHORTCUT/);
    expect(() => loadConfig({ ...production, TEST_AUTH_SHORTCUT: "1" })).toThrow(
      /TEST_AUTH_SHORTCUT/,
    );
  });

  it("refuses tests against a database not ending in _test", () => {
    expect(() =>
      loadConfig({ ...base, APP_DATABASE_URL: "postgres://u:p@localhost/app" }),
    ).toThrow(/_test/);
  });

  it("needs a 32-byte key-encryption key in production", () => {
    expect(loadConfig(production).kek?.key.length).toBe(32);
    expect(() => loadConfig({ ...production, ROLERANK_KEK: undefined })).toThrow(/required in production/);
    expect(() => loadConfig({ ...production, ROLERANK_KEK: Buffer.alloc(16).toString("base64") })).toThrow(/32 bytes/);
    expect(loadConfig(base).kek).toBeNull();
  });

  it("refuses http and provider overrides in production", () => {
    expect(() => loadConfig({ ...production, PUBLIC_ORIGIN: "http://x.example" })).toThrow(/https/);
    expect(() => loadConfig({ ...production, GOOGLE_ISSUER: "https://evil.example" })).toThrow(
      /overridden/,
    );
    expect(() => loadConfig({ ...production, GITHUB_API_URL: "https://evil.example" })).toThrow(
      /overridden/,
    );
  });

  it("names missing fields without printing any value", () => {
    const { GOOGLE_CLIENT_SECRET: _secret, ...missing } = base;
    let message = "";
    try {
      loadConfig({ ...missing, GITHUB_CLIENT_SECRET: "top-secret" });
    } catch (error) {
      message = (error as Error).message;
    }
    expect(message).toContain("GOOGLE_CLIENT_SECRET");
    expect(message).not.toContain("top-secret");
  });
});
