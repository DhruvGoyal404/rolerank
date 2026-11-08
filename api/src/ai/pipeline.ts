import type { Pool } from "pg";
import type { Log } from "../log.js";
import type { Vault } from "../vault.js";
import { stripContacts, type Line } from "./contacts.js";
import type { Gateway } from "./gateway.js";
import { experienceMonths } from "./profile.js";

/**
 * aiworker's resume pipeline: extract in the sandbox, strip contacts, parse, ground, store.
 * The queue is the resumes table itself (FOR UPDATE SKIP LOCKED), so no job is lost if a worker
 * dies, and a resume waiting for budget simply stays queued. Logs carry ids and outcome codes
 * only, never content.
 */
export interface Extractor {
  extract(file: Buffer): Promise<{ status: "ok"; lines: Line[] } | { status: "failed"; code: string }>;
}

/** The Go extractor service (worker/cmd/extractor). */
export function httpExtractor(url: string, token: string): Extractor {
  return {
    async extract(file) {
      const res = await fetch(`${url}/extract`, {
        method: "POST",
        headers: { authorization: `Bearer ${token}`, "content-type": "application/octet-stream" },
        body: new Uint8Array(file),
      });
      if (res.status === 422) return { status: "failed", code: "rejected" };
      if (!res.ok) return { status: "failed", code: "extract_failed" };
      const out = (await res.json()) as { lines?: Line[]; error?: string };
      if (out.error) return { status: "failed", code: errorCode(out.error) };
      return { status: "ok", lines: out.lines ?? [] };
    },
  };
}

/** Fixed codes for the user: the sandbox's own message never leaves the worker. */
function errorCode(message: string): string {
  if (message.includes("password")) return "password_protected";
  if (message.includes("too many pages")) return "too_many_pages";
  return "unreadable";
}

export interface Deps {
  pool: Pool;
  vault: Vault;
  extractor: Extractor;
  gateway: Gateway;
  log: Log;
}

type Claimed = { id: string; user_id: string; status: string };

/** Claims the oldest queued resume, if any, and moves it on as far as it can go. */
export async function processNext(d: Deps): Promise<string | null> {
  const client = await d.pool.connect();
  let job: Claimed | undefined;
  try {
    await client.query("BEGIN");
    const { rows } = await client.query<Claimed>(
      `SELECT id, user_id, status FROM resumes WHERE status = 'queued'
       ORDER BY updated_at FOR UPDATE SKIP LOCKED LIMIT 1`,
    );
    job = rows[0];
    if (job) {
      await client.query(`UPDATE resumes SET status = 'extracting', error = NULL, updated_at = now() WHERE id = $1`, [
        job.id,
      ]);
    }
    await client.query("COMMIT");
  } catch (error) {
    await client.query("ROLLBACK");
    throw error;
  } finally {
    client.release();
  }
  if (!job) return null;
  // An unexpected error fails the resume, so the user is told; it never sits in a busy state.
  const outcome = await run(d, job.id, job.user_id).catch(async (error: unknown) => {
    d.log.error({ event: "resume_failed", resume_id: job.id, type: error instanceof Error ? error.name : "unknown" });
    return fail(d, job.id, "internal").catch(() => "internal");
  });
  d.log.info({ event: "resume_processed", resume_id: job.id, outcome });
  return outcome;
}

async function fail(d: Deps, id: string, code: string): Promise<string> {
  await d.pool.query(`UPDATE resumes SET status = 'failed', error = $2, file_ct = NULL, updated_at = now() WHERE id = $1`, [
    id,
    code,
  ]);
  return code;
}

async function run(d: Deps, id: string, userId: string): Promise<string> {
  const { pool, vault } = d;
  const { rows } = await pool.query<{ file_ct: Buffer | null; lines_ct: Buffer | null }>(
    `SELECT file_ct, lines_ct FROM resumes WHERE id = $1`,
    [id],
  );
  const row = rows[0];
  if (!row) return "gone";

  let lines: Line[];
  if (row.lines_ct) {
    // Extracted before and waiting for budget: don't extract again.
    lines = await vault.decrypt<Line[]>(pool, userId, "resumes.lines_ct", id, row.lines_ct);
  } else {
    if (!row.file_ct) return fail(d, id, "file_missing");
    const file = await vault.decryptBytes(pool, userId, "resumes.file_ct", id, row.file_ct);
    const got = await d.extractor.extract(file);
    if (got.status === "failed") return fail(d, id, got.code);
    if (got.lines.filter((l) => !l.hidden).length === 0) return fail(d, id, "no_text"); // a scanned image
    lines = got.lines;
    // The original is deleted once extracted (Q10); only the encrypted lines stay.
    await pool.query(
      `UPDATE resumes SET lines_ct = $2, file_ct = NULL, hidden_lines = $3, status = 'parsing', updated_at = now()
       WHERE id = $1`,
      [id, await vault.encrypt(pool, userId, "resumes.lines_ct", id, lines), lines.filter((l) => l.hidden).length],
    );
  }

  const stripped = stripContacts(lines);
  await pool.query(`UPDATE resumes SET contacts_ct = $2 WHERE id = $1`, [
    id,
    await vault.encrypt(pool, userId, "resumes.contacts_ct", id, stripped.contacts),
  ]);
  const parsed = await d.gateway.parseResume(userId, stripped.lines);
  if (parsed.outcome === "capped") {
    // At the cap, parsing waits: back in the queue, tried again later.
    await pool.query(`UPDATE resumes SET status = 'queued', error = 'waiting_for_budget', updated_at = now() WHERE id = $1`, [
      id,
    ]);
    return "capped";
  }
  if (parsed.outcome !== "ok" || !parsed.grounded) return fail(d, id, `parse_${parsed.outcome}`);

  const g = parsed.grounded;
  const client = await pool.connect();
  try {
    await client.query("BEGIN");
    const { rows: v } = await client.query<{ next: number }>(
      `SELECT COALESCE(MAX(version), 0) + 1 AS next FROM profiles WHERE user_id = $1`,
      [userId],
    );
    const { rows: p } = await client.query<{ id: string }>(
      `INSERT INTO profiles (user_id, resume_id, version, data_ct, skills, level)
       VALUES ($1, $2, $3, '\\x00', $4, $5) RETURNING id`,
      [userId, id, v[0]?.next ?? 1, g.profile.skills.map((s) => s.name), g.profile.level],
    );
    const profileId = p[0]?.id ?? "";
    const data = { profile: g.profile, dropped: g.dropped, droppedBy: g.droppedBy, months: experienceMonths(g.profile) };
    await client.query(`UPDATE profiles SET data_ct = $2 WHERE id = $1`, [
      profileId,
      await vault.encrypt(client, userId, "profiles.data_ct", profileId, data),
    ]);
    await client.query(`UPDATE resumes SET status = 'ready', error = NULL, updated_at = now() WHERE id = $1`, [id]);
    await client.query("COMMIT");
  } catch (error) {
    await client.query("ROLLBACK");
    throw error;
  } finally {
    client.release();
  }
  return "ready";
}
