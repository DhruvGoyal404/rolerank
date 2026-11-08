import type { Express, Request, Response } from "express";
import type { Redis } from "iovalkey";
import type { Pool } from "pg";
import { RateLimiterRedis } from "rate-limiter-flexible";
import { latestConfirmed } from "../ai/assist.js";
import { formsIn } from "../ai/evidence.js";
import type { JobFacts } from "../ai/gateway.js";
import {
  analyseJD,
  explain,
  type Explanation,
  type Level,
} from "../ai/match.js";
import type { AiClient } from "../aiserver.js";
import { rankQuery, type Ranker } from "../ranker.js";
import { preferencesOf } from "./account.js";
import type { Vault } from "../vault.js";
import { rateLimit, requireUser } from "./middleware.js";

/**
 * P5: Bring a JD, job detail, explanations and tailoring (ARCHITECTURE.md, "API surface").
 * The deterministic explanation needs no LLM and is always there. LLM prose and tailoring come
 * from aiworker, gated; when it's unavailable or a cap is hit, the deterministic answer stands.
 * A pasted JD is untrusted: it is analysed in memory, never stored, never joins the index, and
 * only its title and taxonomy skills reach a model.
 */
const MAX_JD_BYTES = 20 * 1024;
const KEY = /^[1-9]\d{0,17}$/;
/** The ranker names bands in lower case (worker/internal/rank); the API names them for people. */
const BANDS: Record<string, string> = { strong: "Strong", good: "Good", stretch: "Stretch" };
export const bandName = (b: string): string => BANDS[b.toLowerCase()] ?? "Stretch";

interface Posting {
  id: string;
  title: string;
  company: string;
  location_raw: string | null;
  level: string;
  url: string;
  apply_url: string | null;
  excerpt: string;
  source: string;
  closed_at: Date | null;
  posted_at: Date; // when the source says it was posted, else when we first saw it
  skills_required: string[];
  skills_nice: string[];
  body: string; // read only for the job's own skill wording; never sent or returned
}

export function mountJobs(
  app: Express,
  d: { pool: Pool; valkey: Redis; vault: Vault; corpus?: Pool; ai?: AiClient; ranker?: Ranker },
): void {
  const { pool, vault } = d;
  const limiter = (name: string, points: number) =>
    new RateLimiterRedis({
      storeClient: d.valkey,
      keyPrefix: `rl:${name}:d`,
      points,
      duration: 86400,
    });
  const jdLimit = limiter("jd", 20);
  const explainLimit = limiter("explain", 30);
  const tailorLimit = limiter("tailor", 10);
  const byUser = (req: Request) => req.userId ?? "";

  const profileFor = async (res: Response, userId: string) => {
    const c = await latestConfirmed(pool, vault, userId);
    if (!c) res.status(409).json({ error: "confirm_profile_first" });
    return c;
  };

  /** The pasted JD, or null after answering 400/413. */
  const jdText = (req: Request, res: Response): string | null => {
    const text = (req.body as { text?: unknown } | undefined)?.text;
    if (typeof text !== "string" || text.trim() === "") {
      res.status(400).json({ error: "text_required" });
      return null;
    }
    if (Buffer.byteLength(text, "utf8") > MAX_JD_BYTES) {
      res.status(413).json({ error: "jd_too_long" });
      return null;
    }
    return text;
  };
  const jdTitle = (text: string) =>
    (text.split("\n").find((l) => l.trim() !== "") ?? "").trim().slice(0, 120);

  // Bring a JD: the same breakdown as an indexed job, from pasted text.
  app.post(
    "/api/jd",
    requireUser,
    rateLimit(jdLimit, byUser),
    (req, res, next) => {
      const text = jdText(req, res);
      if (text === null) return;
      (async () => {
        const c = await profileFor(res, byUser(req));
        if (!c) return;
        res.json({ title: jdTitle(text), ...analyseJD(text, c.profile) });
      })().catch(next);
    },
  );

  const withProse = async (
    res: Response,
    userId: string,
    job: JobFacts,
    e: Explanation,
    extra: Record<string, unknown>,
  ) => {
    const r = d.ai ? await d.ai.explain(userId, job, e) : null;
    res.json({
      ...extra,
      explanation: e,
      prose: r?.prose ?? null,
      proseOutcome: r?.outcome ?? "unavailable",
    });
  };
  const tailorReply = async (
    res: Response,
    userId: string,
    job: JobFacts,
    e: Explanation,
  ) => {
    if (!d.ai) {
      res.status(503).json({ error: "tailoring_unavailable" });
      return;
    }
    const r = await d.ai.tailor(userId, job);
    if (!r) {
      res.status(503).json({ error: "tailoring_unavailable" });
      return;
    }
    if (r.outcome === "no_profile") {
      res.status(409).json({ error: "confirm_profile_first" });
      return;
    }
    // Grounded outputs only: evidence (with gated reasons), what isn't shown, alias swaps and
    // emphasis. Why a reason was dropped stays in aiworker's counts, not in the response.
    res.json({
      outcome: r.outcome,
      evidence: r.evidence,
      notShown: r.notShown,
      alignments: r.alignments,
      emphasis: r.emphasis,
      reasons: { kept: r.reasons.kept, dropped: r.reasons.dropped.length, outcome: r.reasons.outcome },
      missing: e.missing,
    });
  };

  app.post(
    "/api/jd/explain",
    requireUser,
    rateLimit(explainLimit, byUser),
    (req, res, next) => {
      const text = jdText(req, res);
      if (text === null) return;
      (async () => {
        const c = await profileFor(res, byUser(req));
        if (!c) return;
        const a = analyseJD(text, c.profile);
        await withProse(
          res,
          byUser(req),
          { title: jdTitle(text), required: a.required, nice: a.nice },
          a,
          { title: jdTitle(text) },
        );
      })().catch(next);
    },
  );

  app.post(
    "/api/jd/tailor",
    requireUser,
    rateLimit(tailorLimit, byUser),
    (req, res, next) => {
      const text = jdText(req, res);
      if (text === null) return;
      (async () => {
        const c = await profileFor(res, byUser(req));
        if (!c) return;
        const a = analyseJD(text, c.profile);
        await tailorReply(
          res,
          byUser(req),
          { title: jdTitle(text), required: a.required, nice: a.nice, forms: formsIn(text) },
          a,
        );
      })().catch(next);
    },
  );

  const corpus = d.corpus;
  if (!corpus) return;

  const posting = async (
    req: Request,
    res: Response,
  ): Promise<Posting | null> => {
    const key = req.params.key;
    if (typeof key !== "string" || !KEY.test(key)) {
      res.status(404).json({ error: "not_found" });
      return null;
    }
    const { rows } = await corpus.query<Posting>(
      `SELECT p.id::text AS id, p.title, p.company, p.location_raw, p.level, p.url, p.apply_url, p.excerpt, s.name AS source,
              p.closed_at, COALESCE(p.posted_at, p.first_seen_at) AS posted_at, p.skills_required, p.skills_nice, p.body
       FROM postings p JOIN sources s ON s.id = p.source_id WHERE p.id = $1`,
      [key],
    );
    const p = rows[0];
    if (!p) res.status(404).json({ error: "not_found" });
    return p ?? null;
  };
  const facts = (p: Posting): JobFacts => ({
    title: p.title,
    required: p.skills_required,
    nice: p.skills_nice,
    forms: formsIn(p.body),
  });
  const explained = (
    p: Posting,
    profile: { skills: Array<{ name: string }>; level: string },
  ) =>
    explain(
      profile.skills.map((s) => s.name),
      profile.level,
      {
        required: p.skills_required,
        nice: p.skills_nice,
        level: p.level as Level,
      },
    );
  // The posting's body never leaves the server; the excerpt is empty for link-only sources.
  const card = (p: Posting) => ({
    key: p.id,
    title: p.title,
    company: p.company,
    location: p.location_raw,
    level: p.level,
    url: p.url,
    applyUrl: p.apply_url,
    excerpt: p.excerpt,
    source: p.source,
    closed: p.closed_at !== null,
    postedAt: p.posted_at,
  });

  // Ranked matches for the confirmed profile, each with its deterministic explanation and fit
  // band (never a percentage). Postings are read as corpus_ro; their bodies never leave.
  const ranker = d.ranker;
  if (ranker) {
    const matchLimit = new RateLimiterRedis({ storeClient: d.valkey, keyPrefix: "rl:matches:m", points: 60, duration: 60 });
    app.get("/api/matches", requireUser, rateLimit(matchLimit, byUser), (req, res, next) => {
      (async () => {
        const c = await profileFor(res, byUser(req));
        if (!c) return;
        // Saved preferences decide, and the query string can override them for one search.
        const prefs = await preferencesOf(pool, byUser(req));
        const country = typeof req.query.country === "string" && /^[A-Z]{2}$/.test(req.query.country) ? req.query.country : prefs.country;
        const remote = req.query.remote === undefined ? prefs.remote : req.query.remote !== "0";
        const limit = Math.min(Math.max(Number(req.query.limit) || 20, 1), 50);
        const q = rankQuery(c.profile, { country, remote, limit });
        const hits = await ranker.search(prefs.targetTitles ? { ...q, titles: prefs.targetTitles } : q);
        if (!hits) {
          res.status(503).json({ error: "search_unavailable" });
          return;
        }
        const { rows } = await corpus.query<Posting>(
          `SELECT p.id::text AS id, p.title, p.company, p.location_raw, p.level, p.url, p.apply_url, p.excerpt, s.name AS source,
                  p.closed_at, COALESCE(p.posted_at, p.first_seen_at) AS posted_at, p.skills_required, p.skills_nice, '' AS body
           FROM postings p JOIN sources s ON s.id = p.source_id WHERE p.id = ANY($1::bigint[])`,
          [hits.map((h) => h.id)],
        );
        const byId = new Map(rows.map((r) => [r.id, r]));
        const matches = hits.flatMap((h) => {
          const p = byId.get(String(h.id));
          return p ? [{ job: card(p), band: bandName(h.band), explanation: explained(p, c.profile) }] : [];
        });
        res.json({ matches, country, remote });
      })().catch(next);
    });
  }

  app.get("/api/jobs/:key", requireUser, (req, res, next) => {
    (async () => {
      const p = await posting(req, res);
      if (!p) return;
      const c = await latestConfirmed(pool, vault, byUser(req));
      res.json({
        job: card(p),
        explanation: c ? explained(p, c.profile) : null,
      });
    })().catch(next);
  });

  app.post(
    "/api/jobs/:key/explain",
    requireUser,
    rateLimit(explainLimit, byUser),
    (req, res, next) => {
      (async () => {
        const p = await posting(req, res);
        if (!p) return;
        const c = await profileFor(res, byUser(req));
        if (!c) return;
        await withProse(res, byUser(req), facts(p), explained(p, c.profile), {
          job: card(p),
        });
      })().catch(next);
    },
  );

  app.post(
    "/api/jobs/:key/tailor",
    requireUser,
    rateLimit(tailorLimit, byUser),
    (req, res, next) => {
      (async () => {
        const p = await posting(req, res);
        if (!p) return;
        const c = await profileFor(res, byUser(req));
        if (!c) return;
        await tailorReply(res, byUser(req), facts(p), explained(p, c.profile));
      })().catch(next);
    },
  );
}
