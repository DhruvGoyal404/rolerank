import express, { type Express, type Request } from "express";
import type { Redis } from "iovalkey";
import type { Pool } from "pg";
import { RateLimiterRedis } from "rate-limiter-flexible";
import { ProfileSchema, type Profile } from "../ai/profile.js";
import type { Vault } from "../vault.js";
import { rateLimit, requireUser } from "./middleware.js";

/**
 * Resume upload and profile review (ARCHITECTURE.md, "API surface"). Every row is read through
 * a query scoped by user_id; a missing or foreign id is 404. Uploads are checked here only for
 * size and magic bytes; the full check runs in the extractor before the sandbox.
 */
const MAX_BYTES = 5 * 1024 * 1024;
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

export function kindOf(b: Buffer): "pdf" | "docx" | null {
  if (b.subarray(0, 5).toString("latin1") === "%PDF-") return "pdf";
  if (b.length > 4 && b[0] === 0x50 && b[1] === 0x4b && b[2] === 0x03 && b[3] === 0x04) return "docx";
  return null;
}

interface Stored {
  profile: Profile;
  dropped: number;
  droppedBy: Record<string, number>;
  months: number;
}

export function mountResumes(app: Express, d: { pool: Pool; valkey: Redis; vault: Vault }): void {
  const { pool, vault } = d;
  const hourly = new RateLimiterRedis({ storeClient: d.valkey, keyPrefix: "rl:upload:h", points: 5, duration: 3600 });
  const daily = new RateLimiterRedis({ storeClient: d.valkey, keyPrefix: "rl:upload:d", points: 20, duration: 86400 });
  const byUser = (req: Request) => req.userId ?? "";
  const id = (req: Request): string | null => {
    const v = req.params.id;
    return typeof v === "string" && UUID.test(v) ? v : null;
  };

  app.post(
    "/api/resumes",
    requireUser,
    rateLimit(hourly, byUser),
    rateLimit(daily, byUser),
    express.raw({ type: () => true, limit: MAX_BYTES }),
    (req, res, next) => {
      const body = Buffer.isBuffer(req.body) ? req.body : Buffer.alloc(0);
      const kind = kindOf(body);
      if (!kind) {
        res.status(415).json({ error: "pdf_or_docx_only" });
        return;
      }
      const userId = req.userId ?? "";
      (async () => {
        const { rows } = await pool.query<{ id: string }>(
          `INSERT INTO resumes (user_id, status, kind) VALUES ($1, 'extracting', $2) RETURNING id`,
          [userId, kind],
        );
        const resumeId = rows[0]?.id ?? "";
        const sealed = await vault.encryptBytes(pool, userId, "resumes.file_ct", resumeId, body);
        // Queued only once the file is stored, so a worker never claims an empty row.
        await pool.query(`UPDATE resumes SET file_ct = $2, status = 'queued' WHERE id = $1`, [resumeId, sealed]);
        res.status(202).json({ id: resumeId });
      })().catch(next);
    },
  );

  app.get("/api/resumes/:id", requireUser, (req, res, next) => {
    const resumeId = id(req);
    if (!resumeId) {
      res.status(404).json({ error: "not_found" });
      return;
    }
    pool
      .query<{ status: string; error: string | null; hidden_lines: number; profile_id: string | null }>(
        `SELECT r.status, r.error, r.hidden_lines,
           (SELECT p.id FROM profiles p WHERE p.resume_id = r.id AND p.user_id = r.user_id ORDER BY version DESC LIMIT 1) AS profile_id
         FROM resumes r WHERE r.id = $1 AND r.user_id = $2`,
        [resumeId, req.userId],
      )
      .then(({ rows }) => {
        const r = rows[0];
        if (!r) res.status(404).json({ error: "not_found" });
        else res.json({ id: resumeId, status: r.status, error: r.error, hiddenLines: r.hidden_lines, profileId: r.profile_id });
      }, next);
  });

  const loadProfile = async (profileId: string, userId: string) => {
    const { rows } = await pool.query<{ version: number; data_ct: Buffer; confirmed_at: Date | null }>(
      `SELECT version, data_ct, confirmed_at FROM profiles WHERE id = $1 AND user_id = $2`,
      [profileId, userId],
    );
    const row = rows[0];
    if (!row) return null;
    const data = await vault.decrypt<Stored>(pool, userId, "profiles.data_ct", profileId, row.data_ct);
    return { row, data };
  };

  app.get("/api/profiles/:id", requireUser, (req, res, next) => {
    const profileId = id(req);
    if (!profileId) {
      res.status(404).json({ error: "not_found" });
      return;
    }
    loadProfile(profileId, req.userId ?? "").then((p) => {
      if (!p) res.status(404).json({ error: "not_found" });
      else
        res.json({
          id: profileId,
          version: p.row.version,
          confirmed: p.row.confirmed_at !== null,
          profile: p.data.profile,
          dropped: p.data.dropped,
          months: p.data.months,
        });
    }, next);
  });

  // The user corrects their draft. Their own edits need no grounding: a confirmed profile is
  // what the user says is true. A confirmed profile is never edited in place.
  app.put("/api/profiles/:id", requireUser, (req, res, next) => {
    const profileId = id(req);
    const parsed = ProfileSchema.safeParse((req.body as { profile?: unknown } | undefined)?.profile);
    if (!profileId) {
      res.status(404).json({ error: "not_found" });
      return;
    }
    if (!parsed.success) {
      res.status(400).json({ error: "invalid_profile" });
      return;
    }
    const userId = req.userId ?? "";
    (async () => {
      const p = await loadProfile(profileId, userId);
      if (!p) return res.status(404).json({ error: "not_found" });
      if (p.row.confirmed_at) return res.status(409).json({ error: "already_confirmed" });
      const data: Stored = { ...p.data, profile: parsed.data };
      await pool.query(`UPDATE profiles SET data_ct = $3, skills = $4, level = $5 WHERE id = $1 AND user_id = $2`, [
        profileId,
        userId,
        await vault.encrypt(pool, userId, "profiles.data_ct", profileId, data),
        parsed.data.skills.map((s) => s.name),
        parsed.data.level,
      ]);
      return res.status(204).end();
    })().catch(next);
  });

  app.post("/api/profiles/:id/confirm", requireUser, (req, res, next) => {
    const profileId = id(req);
    if (!profileId) {
      res.status(404).json({ error: "not_found" });
      return;
    }
    pool
      .query(`UPDATE profiles SET confirmed_at = COALESCE(confirmed_at, now()) WHERE id = $1 AND user_id = $2`, [
        profileId,
        req.userId,
      ])
      .then((r) => (r.rowCount ? res.status(204).end() : res.status(404).json({ error: "not_found" })), next);
  });
}
