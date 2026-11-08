import type { Express, Request } from "express";
import type { Redis } from "iovalkey";
import type { Pool } from "pg";
import { RateLimiterRedis } from "rate-limiter-flexible";
import type { Vault } from "../vault.js";
import { rateLimit, requireUser } from "./middleware.js";

/**
 * Preferences and the data export (SPEC.md, Account and profile). Every read is scoped by
 * user_id. The export holds everything RoleRank keeps about the user, decrypted for them alone.
 */
export interface Preferences {
  country: string;
  remote: boolean;
  targetTitles: string;
}

export const DEFAULT_PREFERENCES: Preferences = { country: "IN", remote: true, targetTitles: "" };

export async function preferencesOf(pool: Pool, userId: string): Promise<Preferences> {
  const { rows } = await pool.query<{ country: string; remote: boolean; target_titles: string }>(
    `SELECT country, remote, target_titles FROM user_preferences WHERE user_id = $1`,
    [userId],
  );
  const r = rows[0];
  return r ? { country: r.country, remote: r.remote, targetTitles: r.target_titles } : DEFAULT_PREFERENCES;
}

export function mountAccount(app: Express, d: { pool: Pool; valkey: Redis; vault: Vault }): void {
  const { pool, vault } = d;
  const byUser = (req: Request) => req.userId ?? "";
  const exports = new RateLimiterRedis({ storeClient: d.valkey, keyPrefix: "rl:export:d", points: 3, duration: 86400 });
  const writes = new RateLimiterRedis({ storeClient: d.valkey, keyPrefix: "rl:prefs:m", points: 30, duration: 60 });

  app.get("/api/preferences", requireUser, (req, res, next) => {
    preferencesOf(pool, byUser(req)).then((p) => res.json(p), next);
  });

  app.put("/api/preferences", requireUser, rateLimit(writes, byUser), (req, res, next) => {
    const b = (req.body ?? {}) as Partial<Record<keyof Preferences, unknown>>;
    const country = typeof b.country === "string" ? b.country : "IN";
    const titles = typeof b.targetTitles === "string" ? b.targetTitles.replace(/\s+/g, " ").trim() : "";
    if (!/^[A-Z]{2}$/.test(country) || typeof b.remote !== "boolean" || titles.length > 200) {
      res.status(400).json({ error: "invalid_preferences" });
      return;
    }
    pool
      .query(
        `INSERT INTO user_preferences (user_id, country, remote, target_titles) VALUES ($1, $2, $3, $4)
         ON CONFLICT (user_id) DO UPDATE SET country = $2, remote = $3, target_titles = $4, updated_at = now()`,
        [byUser(req), country, b.remote, titles],
      )
      .then(() => res.json({ country, remote: b.remote, targetTitles: titles }), next);
  });

  // Everything kept about the user, as JSON they can save.
  app.get("/api/me/export", requireUser, rateLimit(exports, byUser), (req, res, next) => {
    const userId = byUser(req);
    (async () => {
      const q = <T>(sql: string) => pool.query<T & Record<string, unknown>>(sql, [userId]).then((r) => r.rows);
      const [account] = await q<{ id: string; created_at: Date; age_confirmed_at: Date | null }>(
        `SELECT id, created_at, age_confirmed_at FROM users WHERE id = $1`,
      );
      const identities = await q<{ provider: string; created_at: Date }>(`SELECT provider, created_at FROM identities WHERE user_id = $1`);
      const resumes = await q<{ id: string; status: string; kind: string; created_at: Date; lines_ct: Buffer | null; contacts_ct: Buffer | null }>(
        `SELECT id, status, kind, created_at, lines_ct, contacts_ct FROM resumes WHERE user_id = $1 ORDER BY created_at`,
      );
      const profiles = await q<{ id: string; version: number; confirmed_at: Date | null; created_at: Date; data_ct: Buffer }>(
        `SELECT id, version, confirmed_at, created_at, data_ct FROM profiles WHERE user_id = $1 ORDER BY version`,
      );
      const tracker = await q(`SELECT posting_id::text AS key, title, company, url, status, created_at, updated_at FROM tracker_items WHERE user_id = $1`);
      const calls = await q(`SELECT day, task, prompt_version, cost_usd, outcome FROM llm_calls WHERE user_id = $1 ORDER BY day`);
      res.setHeader("Content-Disposition", 'attachment; filename="rolerank-export.json"');
      res.json({
        exportedAt: new Date().toISOString(),
        account: { ...account, identities },
        preferences: await preferencesOf(pool, userId),
        resumes: await Promise.all(
          resumes.map(async (r) => ({
            id: r.id,
            status: r.status,
            kind: r.kind,
            createdAt: r.created_at,
            lines: r.lines_ct ? await vault.decrypt(pool, userId, "resumes.lines_ct", r.id, r.lines_ct) : null,
            contacts: r.contacts_ct ? await vault.decrypt(pool, userId, "resumes.contacts_ct", r.id, r.contacts_ct) : null,
          })),
        ),
        profiles: await Promise.all(
          profiles.map(async (p) => ({
            id: p.id,
            version: p.version,
            confirmedAt: p.confirmed_at,
            createdAt: p.created_at,
            data: await vault.decrypt(pool, userId, "profiles.data_ct", p.id, p.data_ct),
          })),
        ),
        tracker,
        aiCalls: calls,
      });
    })().catch(next);
  });
}
