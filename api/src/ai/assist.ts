import type { Pool } from "pg";
import type { Vault } from "../vault.js";
import { stripContacts, type Line } from "./contacts.js";
import {
  matchBlock,
  tailorBlock,
  type Gateway,
  type JobFacts,
  type Outcome,
} from "./gateway.js";
import { proseNamesOnly, type Explanation } from "./match.js";
import type { Profile } from "./profile.js";
import { gate, type Suggestion } from "./tailor.js";

/**
 * P5's LLM features, run inside aiworker (it owns the only LLM client). Both put a gate between
 * the model and the user: prose that names a skill outside the deterministic lists is withheld,
 * and a tailoring suggestion that fails the grounding gate is dropped and counted. Neither ever
 * returns ungated model output.
 */

export interface Confirmed {
  profileId: string;
  profile: Profile;
  resumeId: string | null;
}

/** The user's latest confirmed profile, read through a query scoped by user_id. */
export async function latestConfirmed(
  pool: Pool,
  vault: Vault,
  userId: string,
): Promise<Confirmed | null> {
  const { rows } = await pool.query<{
    id: string;
    resume_id: string | null;
    data_ct: Buffer;
  }>(
    `SELECT id, resume_id, data_ct FROM profiles WHERE user_id = $1 AND confirmed_at IS NOT NULL ORDER BY version DESC LIMIT 1`,
    [userId],
  );
  const row = rows[0];
  if (!row) return null;
  const data = await vault.decrypt<{ profile: Profile }>(
    pool,
    userId,
    "profiles.data_ct",
    row.id,
    row.data_ct,
  );
  return { profileId: row.id, profile: data.profile, resumeId: row.resume_id };
}

async function resumeLines(
  pool: Pool,
  vault: Vault,
  userId: string,
  resumeId: string,
): Promise<Line[] | null> {
  const { rows } = await pool.query<{ lines_ct: Buffer | null }>(
    `SELECT lines_ct FROM resumes WHERE id = $1 AND user_id = $2`,
    [resumeId, userId],
  );
  const ct = rows[0]?.lines_ct;
  return ct
    ? vault.decrypt<Line[]>(pool, userId, "resumes.lines_ct", resumeId, ct)
    : null;
}

export interface ExplainResult {
  outcome: Outcome | "gated";
  prose: string | null;
  extra?: string[]; // skills the withheld prose named outside the lists (counted, never shown)
}

export async function explainProse(
  gw: Gateway,
  userId: string,
  job: JobFacts,
  e: Explanation,
): Promise<ExplainResult> {
  const r = await gw.explain(userId, matchBlock(job, e));
  if (r.outcome !== "ok" || r.text === undefined)
    return { outcome: r.outcome, prose: null };
  const g = proseNamesOnly(r.text, e);
  return g.ok
    ? { outcome: "ok", prose: r.text }
    : { outcome: "gated", prose: null, extra: g.extra };
}

export interface TailorResult {
  outcome: Outcome | "no_profile" | "no_resume";
  suggestions: Suggestion[];
  proposed: number; // what the model returned, before the gate
  dropped: Array<{ line: number; why: string }>; // never the text: only where and why
}

export async function tailorFor(
  gw: Gateway,
  pool: Pool,
  vault: Vault,
  userId: string,
  job: JobFacts,
): Promise<TailorResult> {
  const c = await latestConfirmed(pool, vault, userId);
  if (!c)
    return { outcome: "no_profile", suggestions: [], proposed: 0, dropped: [] };
  const raw = c.resumeId
    ? await resumeLines(pool, vault, userId, c.resumeId)
    : null;
  if (!raw)
    return { outcome: "no_resume", suggestions: [], proposed: 0, dropped: [] };
  return tailorLines(gw, userId, raw, c.profile, job);
}

/** Contacts are stripped before the model sees the lines, and the gate checks the same lines. */
export async function tailorLines(
  gw: Gateway,
  userId: string,
  raw: Line[],
  profile: Profile,
  job: JobFacts,
): Promise<TailorResult> {
  const lines = stripContacts(raw).lines;
  const r = await gw.tailor(userId, tailorBlock(lines, profile, job));
  if (r.outcome !== "ok" || !r.suggestions)
    return { outcome: r.outcome, suggestions: [], proposed: 0, dropped: [] };
  const g = gate(r.suggestions, lines, profile);
  return {
    outcome: "ok",
    suggestions: g.kept,
    proposed: r.suggestions.length,
    dropped: g.dropped.map((d) => ({ line: d.suggestion.line, why: d.why })),
  };
}
