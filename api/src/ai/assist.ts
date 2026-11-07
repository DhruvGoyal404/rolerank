import type { Pool } from "pg";
import type { Vault } from "../vault.js";
import { stripContacts, type Line } from "./contacts.js";
import { tailoring, type Alignment, type JobSkills } from "./evidence.js";
import {
  matchBlock,
  tailorBlock,
  type Gateway,
  type JobFacts,
  type Outcome,
} from "./gateway.js";
import { proseNamesOnly, type Explanation } from "./match.js";
import type { Profile } from "./profile.js";
import { gate } from "./tailor.js";

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

export interface ShownLine {
  n: number;
  text: string;
}
export interface TailorResult {
  outcome: Outcome | "no_profile" | "no_resume";
  evidence: Array<{ skill: string; lines: Array<ShownLine & { reason: string | null }> }>;
  notShown: string[];
  alignments: Alignment[];
  emphasis: { top: ShownLine[]; addMetric: ShownLine[] };
  reasons: { proposed: number; kept: number; dropped: Array<{ line: number; why: string }>; outcome: Outcome | "none" };
}

const empty = (outcome: TailorResult["outcome"]): TailorResult => ({
  outcome,
  evidence: [],
  notShown: [],
  alignments: [],
  emphasis: { top: [], addMetric: [] },
  reasons: { proposed: 0, kept: 0, dropped: [], outcome: "none" },
});

export async function tailorFor(gw: Gateway, pool: Pool, vault: Vault, userId: string, job: JobSkills): Promise<TailorResult> {
  const c = await latestConfirmed(pool, vault, userId);
  if (!c) return empty("no_profile");
  const raw = c.resumeId ? await resumeLines(pool, vault, userId, c.resumeId) : null;
  if (!raw) return empty("no_resume");
  return tailorLines(gw, userId, raw, c.profile, job);
}

/**
 * The three grounded outputs come from code (evidence.ts). The model only adds a reason per
 * skill-to-line mapping; each reason must be about a pair the code found and pass the grounding
 * gate against its own line, or it is dropped. At a cap or on failure, the outputs stand without
 * reasons. Contacts are stripped first, and only those lines are shown or sent.
 */
export async function tailorLines(gw: Gateway, userId: string, raw: Line[], profile: Profile, job: JobSkills): Promise<TailorResult> {
  const lines = stripContacts(raw).lines;
  const t = tailoring(lines, job);
  const text = new Map(lines.map((l) => [l.n, l.text]));
  const shown = (n: number): ShownLine => ({ n, text: text.get(n) ?? "" });
  const pairs = t.evidence.flatMap((e) => e.lines.map((line) => ({ skill: e.skill, line })));
  const reasons = new Map<string, string>();
  const stats: TailorResult["reasons"] = { proposed: 0, kept: 0, dropped: [], outcome: "none" };
  if (pairs.length > 0) {
    const r = await gw.tailor(userId, tailorBlock(lines, pairs));
    stats.outcome = r.outcome;
    const found = new Set(pairs.map((x) => `${x.skill}|${x.line}`));
    for (const x of r.reasons ?? []) {
      stats.proposed++;
      const key = `${x.skill}|${x.line}`;
      if (!found.has(key) || reasons.has(key)) {
        stats.dropped.push({ line: x.line, why: "not a pair the program found" });
        continue;
      }
      const g = gate([{ line: x.line, rewrite: x.reason, reason: "" }], lines, profile);
      const d = g.dropped[0];
      if (d) stats.dropped.push({ line: x.line, why: d.why });
      else {
        reasons.set(key, x.reason);
        stats.kept++;
      }
    }
  }
  return {
    outcome: "ok",
    evidence: t.evidence.map((e) => ({ skill: e.skill, lines: e.lines.map((n) => ({ ...shown(n), reason: reasons.get(`${e.skill}|${n}`) ?? null })) })),
    notShown: t.notShown,
    alignments: t.alignments,
    emphasis: { top: t.emphasis.top.map(shown), addMetric: t.emphasis.addMetric.map(shown) },
    reasons: stats,
  };
}
