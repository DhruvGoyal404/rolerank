import { randomBytes } from "node:crypto";
import pg from "pg";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { expireStale, reserve, settle } from "../src/ai/ledger.js";
import { Vault } from "../src/vault.js";
import { createTestDatabases, urlFor, type TestDatabases } from "./harness.js";

let dbs: TestDatabases;
let pool: pg.Pool;

beforeAll(async () => {
  dbs = await createTestDatabases();
  pool = new pg.Pool({ connectionString: urlFor(dbs.roles.appRw, dbs.passwords.APP_RW ?? "", dbs.appDb), max: 30 });
});
afterAll(async () => {
  await pool.end();
  await dbs.drop();
});

async function newUser(): Promise<string> {
  const { rows } = await pool.query<{ id: string }>(`INSERT INTO users (age_confirmed_at) VALUES (now()) RETURNING id`);
  return rows[0]?.id ?? "";
}

const call = (userId: string, maxUsd: number) => ({ userId, task: "parse", model: "m", promptVersion: "p1", maxUsd });

describe("spend ledger", () => {
  it("holds the per-user cap under 50 concurrent reservations", async () => {
    const user = await newUser();
    const results = await Promise.all(
      Array.from({ length: 50 }, () => reserve(pool, call(user, 0.01), { userUsd: 0.05, globalUsd: 100 })),
    );
    expect(results.filter((r) => r !== null)).toHaveLength(5);
    const { rows } = await pool.query<{ r: number }>(`SELECT reserved_usd::float AS r FROM llm_ledger WHERE scope = $1`, [user]);
    expect(rows[0]?.r).toBeCloseTo(0.05, 6);
  });

  it("holds the global cap across users", async () => {
    const caps = { userUsd: 1, globalUsd: 0.03 };
    // Other tests' spend counts towards the same global row, so start from a fresh day's total.
    const { rows } = await pool.query<{ used: number }>(
      `SELECT COALESCE(SUM(reserved_usd + spent_usd), 0)::float AS used FROM llm_ledger WHERE scope = 'global'`,
    );
    const room = caps.globalUsd + (rows[0]?.used ?? 0);
    const users = await Promise.all(Array.from({ length: 10 }, () => newUser()));
    const got = await Promise.all(users.map((u) => reserve(pool, call(u, 0.01), { ...caps, globalUsd: room })));
    expect(got.filter((r) => r !== null)).toHaveLength(3);
  });

  it("settles a reservation to its real cost, once", async () => {
    const user = await newUser();
    const r = await reserve(pool, call(user, 0.02));
    expect(r).not.toBeNull();
    await settle(pool, r?.id ?? "", { costUsd: 0.004, inputTokens: 1000, outputTokens: 500, outcome: "ok" });
    await settle(pool, r?.id ?? "", { costUsd: 0.004, outcome: "ok" });
    const { rows } = await pool.query(
      `SELECT reserved_usd::float AS r, spent_usd::float AS s FROM llm_ledger WHERE scope = $1`,
      [user],
    );
    expect(rows[0]).toEqual({ r: 0, s: 0.004 });
    // Room freed by settling is usable again.
    expect(await reserve(pool, call(user, 0.04))).not.toBeNull();
  });

  it("counts a reservation never settled in full", async () => {
    const user = await newUser();
    const r = await reserve(pool, call(user, 0.02));
    await pool.query(`UPDATE llm_calls SET created_at = now() - interval '11 minutes' WHERE id = $1`, [r?.id]);
    expect(await expireStale(pool)).toBeGreaterThanOrEqual(1);
    const { rows } = await pool.query<{ s: number }>(`SELECT spent_usd::float AS s FROM llm_ledger WHERE scope = $1`, [user]);
    expect(rows[0]?.s).toBeCloseTo(0.02, 6);
  });
});

describe("vault keys", () => {
  it("creates one key per user, stored only wrapped", async () => {
    const vault = new Vault(randomBytes(32), 1);
    const user = await newUser();
    const [a, b] = await Promise.all([vault.userKey(pool, user), vault.userKey(pool, user)]);
    expect(a.equals(b)).toBe(true);
    const { rows } = await pool.query<{ wrapped: Buffer }>(`SELECT wrapped FROM user_keys WHERE user_id = $1`, [user]);
    expect(rows[0]?.wrapped.includes(a)).toBe(false);
    const sealed = await vault.encrypt(pool, user, "resumes.lines_ct", "r1", { text: "Kubernetes at Acme" });
    expect(await vault.decrypt(pool, user, "resumes.lines_ct", "r1", sealed)).toEqual({ text: "Kubernetes at Acme" });
    // Another user's key can't open it.
    const other = await newUser();
    await expect(vault.decrypt(pool, other, "resumes.lines_ct", "r1", sealed)).rejects.toThrow();
  });
});
