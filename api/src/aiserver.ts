import express, { type Express } from "express";
import type { Pool } from "pg";
import { z } from "zod";
import {
  explainProse,
  tailorFor,
  type ExplainResult,
  type TailorResult,
} from "./ai/assist.js";
import type { Gateway, JobFacts } from "./ai/gateway.js";
import type { Explanation } from "./ai/match.js";
import { safeEqual } from "./crypto.js";
import type { Log } from "./log.js";
import type { Vault } from "./vault.js";

/**
 * aiworker's internal endpoint for P5 (ARCHITECTURE.md: aiworker owns the only LLM client). The
 * API calls it with a shared token over the private network, as aiworker calls the extractor.
 * It takes a user id and job facts, never resume text: it reads the user's own rows itself.
 * Replies are gated results only. Nothing about the request's content is logged.
 */
const skills = z.array(z.string().max(80)).max(200);
const Job = z
  .object({ title: z.string().max(400), required: skills, nice: skills, forms: z.record(z.string().max(80), z.string().max(80)).optional() })
  .strict();
const UserId = z.uuid();
const ExplainIn = z
  .object({
    userId: UserId,
    job: Job,
    explanation: z
      .object({
        matched: skills,
        missing: skills,
        niceMatched: skills,
        level: z
          .object({
            job: z.string().max(20),
            profile: z.string().max(20),
            fit: z.string().max(20),
          })
          .strict(),
      })
      .strict(),
  })
  .strict();
const TailorIn = z.object({ userId: UserId, job: Job }).strict();

export function createAiServer(d: {
  token: string;
  /** Model-written reasons on tailoring evidence: off in v1 (PHASE5.md). */
  reasons?: boolean;
  gateway: Gateway;
  pool: Pool;
  vault: Vault;
  log: Log;
}): Express {
  const app = express();
  app.disable("x-powered-by");
  app.use((req, res, next) => {
    const auth = req.headers.authorization ?? "";
    if (!auth.startsWith("Bearer ") || !safeEqual(auth.slice(7), d.token)) {
      res.status(401).json({ error: "unauthorized" });
      return;
    }
    next();
  });
  app.use(express.json({ limit: "64kb" }));

  app.post("/explain", (req, res) => {
    const body = ExplainIn.safeParse(req.body);
    if (!body.success) {
      res.status(400).json({ error: "invalid_request" });
      return;
    }
    const started = Date.now();
    explainProse(
      d.gateway,
      body.data.userId,
      body.data.job,
      body.data.explanation as Explanation,
    )
      .then((r: ExplainResult) => {
        d.log.info({
          event: "explain",
          outcome: r.outcome,
          ms: Date.now() - started,
        });
        res.json(r);
      })
      .catch(() => res.status(500).json({ error: "explain_failed" }));
  });

  app.post("/tailor", (req, res) => {
    const body = TailorIn.safeParse(req.body);
    if (!body.success) {
      res.status(400).json({ error: "invalid_request" });
      return;
    }
    const started = Date.now();
    tailorFor(d.gateway, d.pool, d.vault, body.data.userId, body.data.job, { reasons: d.reasons ?? false })
      .then((r: TailorResult) => {
        d.log.info({
          event: "tailor",
          outcome: r.outcome,
          count: r.evidence.length,
          dropped: r.reasons.dropped.length,
          ms: Date.now() - started,
        });
        res.json(r);
      })
      .catch(() => res.status(500).json({ error: "tailor_failed" }));
  });

  app.use((_req, res) => res.status(404).json({ error: "not_found" }));
  return app;
}

/** The API's side: calls aiworker, and treats any failure as "unavailable", never as content. */
export interface AiClient {
  explain(
    userId: string,
    job: JobFacts,
    explanation: Explanation,
  ): Promise<ExplainResult | null>;
  tailor(userId: string, job: JobFacts): Promise<TailorResult | null>;
}

export function aiClient(
  url: string,
  token: string,
  timeoutMs = 20_000,
): AiClient {
  const post = async <T>(path: string, body: unknown): Promise<T | null> => {
    try {
      const res = await fetch(`${url}${path}`, {
        method: "POST",
        headers: {
          authorization: `Bearer ${token}`,
          "content-type": "application/json",
        },
        body: JSON.stringify(body),
        signal: AbortSignal.timeout(timeoutMs),
      });
      return res.ok ? ((await res.json()) as T) : null;
    } catch {
      return null;
    }
  };
  return {
    explain: (userId, job, explanation) =>
      post<ExplainResult>("/explain", { userId, job, explanation }),
    tailor: (userId, job) => post<TailorResult>("/tailor", { userId, job }),
  };
}
