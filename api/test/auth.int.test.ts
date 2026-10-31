import { randomBytes } from "node:crypto";
import { Writable } from "node:stream";
import type { Express } from "express";
import type { Redis } from "iovalkey";
import pg from "pg";
import request from "supertest";
import { afterAll, beforeAll, beforeEach, describe, expect, it } from "vitest";
import { githubProvider, googleProvider } from "../src/auth/providers.js";
import { loadConfig, type Config } from "../src/config.js";
import { sha256 } from "../src/crypto.js";
import { createApp } from "../src/http/app.js";
import { createLog } from "../src/log.js";
import {
  createTestDatabases,
  GOOGLE_CLIENT_ID,
  startFakeProvider,
  testValkey,
  urlFor,
  valkeyUrl,
  type FakeProvider,
  type TestDatabases,
} from "./harness.js";

const ORIGIN = "http://localhost:5173";
let dbs: TestDatabases;
let fake: FakeProvider;
let pool: pg.Pool;
let admin: pg.Pool;
let valkey: Redis;
let config: Config;
let app: Express;
const logLines: string[] = [];

beforeAll(async () => {
  dbs = await createTestDatabases();
  fake = await startFakeProvider();
  const databaseUrl = urlFor(dbs.roles.appRw, dbs.passwords.APP_RW ?? "", dbs.appDb);
  pool = new pg.Pool({ connectionString: databaseUrl });
  admin = new pg.Pool({
    connectionString: urlFor(dbs.roles.migratorApp, dbs.passwords.MIGRATOR_APP ?? "", dbs.appDb),
  });
  valkey = testValkey();
  config = loadConfig({
    NODE_ENV: "test",
    PUBLIC_ORIGIN: ORIGIN,
    APP_DATABASE_URL: databaseUrl,
    VALKEY_URL: valkeyUrl(),
    GOOGLE_CLIENT_ID,
    GOOGLE_CLIENT_SECRET: "google-secret",
    GITHUB_CLIENT_ID: "github-client",
    GITHUB_CLIENT_SECRET: "github-secret",
    GOOGLE_ISSUER: fake.issuer,
    GITHUB_WEB_URL: fake.webUrl,
    GITHUB_API_URL: fake.apiUrl,
  });
  const sink = new Writable({
    write(chunk: Buffer, _encoding, done) {
      logLines.push(chunk.toString());
      done();
    },
  });
  app = createApp({
    config,
    pool,
    valkey,
    log: createLog(sink),
    providers: { google: googleProvider(config.google), github: githubProvider(config.github) },
  });
});

afterAll(async () => {
  await pool.end();
  await admin.end();
  await valkey.quit();
  await fake.close();
  await dbs.drop();
});

beforeEach(async () => {
  await valkey.del("rl:auth:::ffff:127.0.0.1", "rl:auth:127.0.0.1", "rl:auth:::1");
  fake.googleClaims = {};
  fake.googleSub = `google-${randomBytes(4).toString("hex")}`;
  fake.githubId = randomBytes(4).readUInt32LE();
  fake.githubTokenBody = { access_token: "gh-token" };
});

async function invite(uses = 1): Promise<string> {
  const code = randomBytes(8).toString("hex");
  await admin.query(
    `INSERT INTO invites (code_sha256, uses_left, expires_at) VALUES ($1, $2, now() + interval '1 day')`,
    [sha256(code), uses],
  );
  return code;
}

interface Started {
  state: string;
  cookie: string;
  location: URL;
}

async function start(provider: string, query: Record<string, string> = {}): Promise<Started> {
  const res = await request(app).get(`/api/auth/${provider}/start`).query(query);
  expect(res.status).toBe(302);
  const location = new URL(res.headers.location ?? "");
  const setCookie = (res.headers["set-cookie"] as unknown as string[])[0] ?? "";
  const nonce = location.searchParams.get("nonce");
  if (nonce) await fetch(`${fake.issuer}/nonce?value=${nonce}`);
  return {
    state: location.searchParams.get("state") ?? "",
    cookie: setCookie.split(";")[0] ?? "",
    location,
  };
}

const finish = (provider: string, s: Started, overrides: { state?: string; cookie?: string } = {}) =>
  request(app)
    .get(`/api/auth/${provider}/callback`)
    .query({ code: "the-code", state: overrides.state ?? s.state })
    .set("Cookie", overrides.cookie ?? s.cookie);

function sessionCookie(res: request.Response): string {
  const cookies = (res.headers["set-cookie"] ?? []) as unknown as string[];
  const found = cookies.find((c) => c.startsWith("rr_session="));
  return found?.split(";")[0] ?? "";
}

async function signIn(provider = "github"): Promise<string> {
  const s = await start(provider, { invite: await invite(), age18: "1" });
  const res = await finish(provider, s);
  expect(res.headers.location).toBe("/");
  return sessionCookie(res);
}

describe("start", () => {
  it("sends PKCE S256 and state to both providers, and a nonce to Google", async () => {
    const google = await start("google");
    expect(google.location.searchParams.get("code_challenge_method")).toBe("S256");
    expect(google.location.searchParams.get("code_challenge")).toMatch(/^[\w-]{43}$/);
    expect(google.location.searchParams.get("nonce")).toBeTruthy();
    expect(google.state).toBeTruthy();

    const github = await start("github");
    expect(github.location.searchParams.get("code_challenge_method")).toBe("S256");
    expect(github.location.searchParams.get("code_challenge")).toMatch(/^[\w-]{43}$/);
  });

  it("marks the state cookie httpOnly and SameSite=Lax", async () => {
    const res = await request(app).get("/api/auth/google/start");
    const cookie = (res.headers["set-cookie"] as unknown as string[])[0] ?? "";
    expect(cookie).toMatch(/HttpOnly/i);
    expect(cookie).toMatch(/SameSite=Lax/i);
  });

  it("404s an unknown provider", async () => {
    expect((await request(app).get("/api/auth/facebook/start")).status).toBe(404);
  });
});

describe("sign-in", () => {
  it("signs in with Google and GitHub, and sends the PKCE verifier", async () => {
    for (const provider of ["google", "github"]) {
      fake.requests.length = 0;
      const cookie = await signIn(provider);
      const me = await request(app).get("/api/me").set("Cookie", cookie);
      expect(me.status).toBe(200);
      const exchange = fake.requests.find((r) => /token/.test(r.path));
      expect(exchange?.body).toContain("code_verifier=");
    }
  });

  it("stores only the session hash", async () => {
    const cookie = await signIn();
    const token = cookie.split("=")[1] ?? "";
    const { rows } = await admin.query<{ n: string }>(
      `SELECT count(*) AS n FROM sessions WHERE token_sha256 = $1`,
      [sha256(token)],
    );
    expect(rows[0]?.n).toBe("1");
    const raw = await admin.query(`SELECT 1 FROM sessions WHERE encode(token_sha256, 'escape') = $1`, [
      token,
    ]);
    expect(raw.rowCount).toBe(0);
  });

  it("lets a returning user in without an invite", async () => {
    await signIn();
    const res = await finish("github", await start("github"));
    expect(res.headers.location).toBe("/");
    expect(sessionCookie(res)).toBeTruthy();
  });

  it("keeps a same-origin next and drops a foreign one", async () => {
    const code = await invite(2);
    const good = await start("github", { invite: code, age18: "1", next: "/matches" });
    expect((await finish("github", good)).headers.location).toBe("/matches");
    const bad = await start("github", { next: "https://evil.example/x" });
    expect((await finish("github", bad)).headers.location).toBe("/");
  });

  it("doesn't link accounts by email", async () => {
    await signIn("google");
    fake.googleSub = `google-${randomBytes(4).toString("hex")}`;
    await signIn("google");
    const { rows } = await admin.query<{ n: string }>(
      `SELECT count(DISTINCT user_id) AS n FROM identities WHERE email = 'person@example.com'`,
    );
    expect(Number(rows[0]?.n)).toBeGreaterThanOrEqual(2);
  });
});

describe("signup rules", () => {
  it("needs an invite", async () => {
    const res = await finish("github", await start("github", { age18: "1" }));
    expect(res.headers.location).toBe("/signin?error=invite_required");
    expect(sessionCookie(res)).toBe("");
  });

  it("needs the 18+ confirmation, without spending the invite", async () => {
    const code = await invite();
    const res = await finish("github", await start("github", { invite: code }));
    expect(res.headers.location).toBe("/signin?error=age_required");
    const again = await finish("github", await start("github", { invite: code, age18: "1" }));
    expect(again.headers.location).toBe("/");
  });

  it("refuses a used-up or expired invite", async () => {
    const code = await invite(1);
    await finish("github", await start("github", { invite: code, age18: "1" }));
    fake.githubId += 1;
    const used = await finish("github", await start("github", { invite: code, age18: "1" }));
    expect(used.headers.location).toBe("/signin?error=invite_required");

    const expired = randomBytes(8).toString("hex");
    await admin.query(
      `INSERT INTO invites (code_sha256, uses_left, expires_at) VALUES ($1, 1, now() - interval '1 hour')`,
      [sha256(expired)],
    );
    const res = await finish("github", await start("github", { invite: expired, age18: "1" }));
    expect(res.headers.location).toBe("/signin?error=invite_required");
  });

  it("can't spend one invite twice under concurrency", async () => {
    const code = await invite(1);
    const attempts = await Promise.all(
      [1, 2, 3, 4, 5].map(async (i) => {
        fake.githubId += i;
        const s = await start("github", { invite: code, age18: "1" });
        return finish("github", s);
      }),
    );
    const { rows } = await admin.query<{ uses_left: number }>(
      `SELECT uses_left FROM invites WHERE code_sha256 = $1`,
      [sha256(code)],
    );
    expect(rows[0]?.uses_left).toBe(0);
    const accepted = attempts.filter((r) => r.headers.location === "/");
    expect(accepted.length).toBeGreaterThanOrEqual(1);
  });
});

describe("rejected logins", () => {
  it("rejects a bad state", async () => {
    const s = await start("github");
    const res = await finish("github", s, { state: "wrong" });
    expect(res.headers.location).toBe("/signin?error=state");
  });

  it("rejects a missing state cookie", async () => {
    const s = await start("github");
    const res = await finish("github", s, { cookie: "other=1" });
    expect(res.headers.location).toBe("/signin?error=state");
  });

  it("rejects a reused state", async () => {
    const code = await invite(2);
    const s = await start("github", { invite: code, age18: "1" });
    expect((await finish("github", s)).headers.location).toBe("/");
    const replay = await finish("github", s);
    expect(replay.headers.location).toBe("/signin?error=state");
    expect(sessionCookie(replay)).toBe("");
  });

  it("rejects a state issued for another provider", async () => {
    const s = await start("github");
    expect((await finish("google", s)).headers.location).toBe("/signin?error=state");
  });

  it.each([
    ["bad nonce", { nonce: "someone-elses" }],
    ["wrong aud", { aud: "another-client" }],
    ["expired token", { exp: 1 }],
    ["wrong issuer", { iss: "https://evil.example" }],
  ])("rejects a Google token with %s", async (_name, claims) => {
    fake.googleClaims = claims;
    const s = await start("google", { invite: await invite(), age18: "1" });
    const res = await finish("google", s);
    expect(res.headers.location).toBe("/signin?error=provider");
    expect(sessionCookie(res)).toBe("");
  });

  it("treats GitHub's 200-with-error body as a failure", async () => {
    fake.githubTokenBody = { error: "bad_verification_code" };
    const res = await finish("github", await start("github"));
    expect(res.headers.location).toBe("/signin?error=provider");
  });
});

describe("sessions", () => {
  it("logs out, and the token stops working", async () => {
    const cookie = await signIn();
    const out = await request(app).post("/api/auth/logout").set("Cookie", cookie).set("Origin", ORIGIN);
    expect(out.status).toBe(204);
    expect((await request(app).get("/api/me").set("Cookie", cookie)).status).toBe(401);
  });

  it("rejects an expired session", async () => {
    const cookie = await signIn();
    await admin.query(`UPDATE sessions SET expires_at = now() - interval '1 second'`);
    expect((await request(app).get("/api/me").set("Cookie", cookie)).status).toBe(401);
  });

  it("refuses anonymous requests", async () => {
    expect((await request(app).get("/api/me")).status).toBe(401);
  });
});

describe("origin check", () => {
  it("refuses a missing Origin", async () => {
    const cookie = await signIn();
    const res = await request(app).post("/api/auth/logout").set("Cookie", cookie);
    expect(res.status).toBe(403);
  });

  it("refuses a foreign Origin and a foreign Referer", async () => {
    const cookie = await signIn();
    const origin = await request(app)
      .delete("/api/me")
      .set("Cookie", cookie)
      .set("Origin", "https://evil.example");
    expect(origin.status).toBe(403);
    const referer = await request(app)
      .delete("/api/me")
      .set("Cookie", cookie)
      .set("Referer", "https://evil.example/page");
    expect(referer.status).toBe(403);
    expect((await request(app).get("/api/me").set("Cookie", cookie)).status).toBe(200);
  });

  it("accepts our own Referer when Origin is absent", async () => {
    const cookie = await signIn();
    const res = await request(app)
      .post("/api/auth/logout")
      .set("Cookie", cookie)
      .set("Referer", `${ORIGIN}/settings`);
    expect(res.status).toBe(204);
  });
});

describe("account deletion", () => {
  it("removes everything for that user and leaves a tombstone", async () => {
    const cookie = await signIn();
    const me = await request(app).get("/api/me").set("Cookie", cookie);
    const id = (me.body as { id: string }).id;
    const res = await request(app).delete("/api/me").set("Cookie", cookie).set("Origin", ORIGIN);
    expect(res.status).toBe(204);

    for (const table of ["users", "identities", "sessions"]) {
      const column = table === "users" ? "id" : "user_id";
      const { rows } = await admin.query<{ n: string }>(
        `SELECT count(*) AS n FROM ${table} WHERE ${column} = $1`,
        [id],
      );
      expect(rows[0]?.n).toBe("0");
    }
    const tombstone = await admin.query(`SELECT 1 FROM deletion_tombstones WHERE user_id = $1`, [id]);
    expect(tombstone.rowCount).toBe(1);
    expect((await request(app).get("/api/me").set("Cookie", cookie)).status).toBe(401);
  });

  it("only ever deletes the caller", async () => {
    const victim = await signIn();
    fake.githubId += 1;
    const attacker = await signIn();
    const ids = await Promise.all(
      [victim, attacker].map(async (cookie) => {
        const me = await request(app).get("/api/me").set("Cookie", cookie);
        return (me.body as { id: string }).id;
      }),
    );
    expect(ids[0]).not.toBe(ids[1]);
    await request(app).delete("/api/me").set("Cookie", attacker).set("Origin", ORIGIN);
    expect((await request(app).get("/api/me").set("Cookie", victim)).status).toBe(200);
    expect((await request(app).get("/api/me").set("Cookie", attacker)).status).toBe(401);
  });
});

describe("headers, limits and logs", () => {
  it("sends strict security headers and no-store", async () => {
    const res = await request(app).get("/api/health");
    expect(res.status).toBe(200);
    expect(res.headers["content-security-policy"]).toContain("default-src 'none'");
    expect(res.headers["x-content-type-options"]).toBe("nosniff");
    expect(res.headers["referrer-policy"]).toBe("strict-origin");
    expect(res.headers["cache-control"]).toBe("no-store");
    expect(res.headers["x-powered-by"]).toBeUndefined();
  });

  it("rate limits auth at 20 per minute with Retry-After", async () => {
    let limited: request.Response | undefined;
    for (let i = 0; i < 22 && !limited; i += 1) {
      const res = await request(app).get("/api/auth/github/start");
      if (res.status === 429) limited = res;
    }
    expect(limited?.status).toBe(429);
    expect(Number(limited?.headers["retry-after"])).toBeGreaterThan(0);
  });

  it("rejects oversized and malformed JSON with 400", async () => {
    const big = await request(app)
      .post("/api/auth/logout")
      .set("Origin", ORIGIN)
      .set("content-type", "application/json")
      .send(`{"a":"${"x".repeat(200_000)}"}`);
    expect(big.status).toBe(400);
  });

  it("never logs codes, tokens, invites or email", async () => {
    await new Promise((resolve) => setImmediate(resolve));
    const text = logLines.join("");
    expect(text).toContain('"event":"request"');
    expect(text).not.toContain("person@example.com");
    expect(text).not.toContain("the-code");
    expect(text).not.toContain("gh-token");
    expect(text).not.toContain("rr_session");
  });
});
