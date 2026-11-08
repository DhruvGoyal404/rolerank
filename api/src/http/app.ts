import express, { type CookieOptions, type Express, type Request, type Response } from "express";
import helmet from "helmet";
import type { Redis } from "iovalkey";
import type { Pool } from "pg";
import { RateLimiterRedis } from "rate-limiter-flexible";
import type { Provider, ProviderName } from "../auth/providers.js";
import { createSession, deleteSession } from "../auth/sessions.js";
import { deleteUser, loginOrSignup } from "../auth/users.js";
import type { Config } from "../config.js";
import { randomToken, safeEqual, sha256 } from "../crypto.js";
import type { Log } from "../log.js";
import type { Vault } from "../vault.js";
import { originCheck, parseCookies, rateLimit, requireUser, sessionLoader } from "./middleware.js";
import type { AiClient } from "../aiserver.js";
import type { Ranker } from "../ranker.js";
import { mountJobs } from "./jobs.js";
import { mountResumes } from "./resumes.js";
import { mountAccount } from "./account.js";
import { mountTracker } from "./tracker.js";

export interface AppDeps {
  config: Config;
  pool: Pool;
  valkey: Redis;
  providers: Record<ProviderName, Provider>;
  log: Log;
  /** Resume and profile routes exist only with a vault: they never run without encryption. */
  vault?: Vault;
  /** P5: postings, read as corpus_ro; and aiworker for explanations and tailoring. */
  corpus?: Pool;
  ai?: AiClient;
  ranker?: Ranker;
}

interface PendingLogin {
  provider: ProviderName;
  verifier: string;
  nonce: string;
  next: string;
  inviteCode: string | null;
  ageConfirmed: boolean;
}

const STATE_TTL_SECONDS = 600;
const SESSION_MAX_AGE_MS = 30 * 86_400_000;

/** Same-origin paths only. Anything else, including `//host`, goes home. */
export function safeNext(value: unknown): string {
  if (typeof value !== "string" || !/^\/(?![/\\])/.test(value) || value.includes("\\")) return "/";
  return value;
}

function asProvider(value: string | undefined): ProviderName | null {
  return value === "google" || value === "github" ? value : null;
}

export function createApp(deps: AppDeps): Express {
  const { config, pool, valkey, providers, log } = deps;
  const app = express();
  app.disable("x-powered-by");
  if (config.env === "production") app.set("trust proxy", 1);

  const prefix = config.secureCookies ? "__Host-" : "";
  const sessionCookie = `${prefix}rr_session`;
  const stateCookie = `${prefix}rr_oauth_state`;
  const cookieOptions: CookieOptions = {
    httpOnly: true,
    sameSite: "lax",
    secure: config.secureCookies,
    path: "/",
  };

  app.use(
    helmet({
      contentSecurityPolicy: {
        useDefaults: false,
        directives: { defaultSrc: ["'none'"], frameAncestors: ["'none'"] },
      },
      strictTransportSecurity: config.secureCookies,
      referrerPolicy: { policy: "strict-origin" },
    }),
  );
  app.use((req, res, next) => {
    const started = performance.now();
    res.on("finish", () => {
      log.info({
        event: "request",
        method: req.method,
        // The matched pattern, never the raw URL: query strings can carry codes.
        route: (req.route as { path?: string } | undefined)?.path ?? "unmatched",
        status: res.statusCode,
        ms: Math.round(performance.now() - started),
      });
    });
    next();
  });
  app.use((_req, res, next) => {
    res.set("Cache-Control", "no-store");
    next();
  });
  app.use(originCheck(config.publicOrigin));
  app.use(express.json({ limit: "100kb" }));
  app.use(sessionLoader(pool, sessionCookie));

  const authLimiter = new RateLimiterRedis({
    storeClient: valkey,
    keyPrefix: "rl:auth",
    points: 20,
    duration: 60,
  });

  app.get("/api/health", (_req, res) => {
    Promise.all([pool.query("SELECT 1"), valkey.ping()]).then(
      () => res.json({ status: "ok" }),
      () => res.status(503).json({ status: "unavailable" }),
    );
  });

  const redirectUri = (provider: ProviderName): string =>
    `${config.publicOrigin}/api/auth/${provider}/callback`;

  const fail = (res: Response, code: string): void => {
    log.info({ event: "login_failed", code });
    res.redirect(302, `/signin?error=${code}`);
  };

  app.get("/api/auth/:provider/start", rateLimit(authLimiter), (req: Request, res, next) => {
    const provider = asProvider(req.params.provider as string | undefined);
    if (!provider) {
      res.status(404).json({ error: "not_found" });
      return;
    }
    const state = randomToken();
    const invite = req.query.invite;
    const pending: PendingLogin = {
      provider,
      verifier: randomToken(),
      nonce: randomToken(),
      next: safeNext(req.query.next),
      inviteCode: typeof invite === "string" && invite !== "" ? invite : null,
      ageConfirmed: req.query.age18 === "1",
    };
    const key = `oauth:state:${sha256(state).toString("hex")}`;
    valkey
      .set(key, JSON.stringify(pending), "EX", STATE_TTL_SECONDS)
      .then(() =>
        providers[provider].authorizeUrl({
          state,
          verifier: pending.verifier,
          nonce: pending.nonce,
          redirectUri: redirectUri(provider),
        }),
      )
      .then((url) => {
        res.cookie(stateCookie, state, { ...cookieOptions, maxAge: STATE_TTL_SECONDS * 1000 });
        res.redirect(302, url);
      })
      .catch(next);
  });

  const callback = async (req: Request, res: Response): Promise<void> => {
    const provider = asProvider(req.params.provider as string | undefined);
    if (!provider) {
      res.status(404).json({ error: "not_found" });
      return;
    }
    const { state, code } = req.query;
    const cookieState = parseCookies(req.headers.cookie).get(stateCookie);
    res.clearCookie(stateCookie, cookieOptions);
    if (
      typeof state !== "string" ||
      typeof code !== "string" ||
      !cookieState ||
      !safeEqual(state, cookieState)
    ) {
      fail(res, "state");
      return;
    }
    // Single use: GETDEL means a replayed state finds nothing.
    const raw = await valkey.getdel(`oauth:state:${sha256(state).toString("hex")}`);
    if (!raw) {
      fail(res, "state");
      return;
    }
    const pending = JSON.parse(raw) as PendingLogin;
    if (pending.provider !== provider) {
      fail(res, "state");
      return;
    }

    let identity;
    try {
      identity = await providers[provider].exchange({
        code,
        verifier: pending.verifier,
        nonce: pending.nonce,
        redirectUri: redirectUri(provider),
      });
    } catch (error) {
      log.error({ event: "oauth_exchange_failed", provider, code: (error as Error).name });
      fail(res, "provider");
      return;
    }

    const result = await loginOrSignup(pool, provider, identity, {
      inviteCode: pending.inviteCode,
      ageConfirmed: pending.ageConfirmed,
    });
    if (!result.ok) {
      fail(res, result.reason);
      return;
    }
    const token = await createSession(pool, result.userId);
    res.cookie(sessionCookie, token, { ...cookieOptions, maxAge: SESSION_MAX_AGE_MS });
    res.redirect(302, pending.next);
  };

  app.get("/api/auth/:provider/callback", rateLimit(authLimiter), (req, res, next) => {
    callback(req, res).catch(next);
  });

  app.post("/api/auth/logout", requireUser, (req, res, next) => {
    const token = parseCookies(req.headers.cookie).get(sessionCookie) ?? "";
    deleteSession(pool, token).then(() => {
      res.clearCookie(sessionCookie, cookieOptions);
      res.status(204).end();
    }, next);
  });

  // Where the user is: their latest resume and profile, so the UI knows what to show next.
  app.get("/api/me", requireUser, (req, res, next) => {
    pool
      .query<{ resume_id: string | null; resume_status: string | null; resume_error: string | null; profile_id: string | null; confirmed: boolean | null }>(
        `SELECT r.id AS resume_id, r.status AS resume_status, r.error AS resume_error, p.id AS profile_id, p.confirmed_at IS NOT NULL AS confirmed
         FROM users u
         LEFT JOIN LATERAL (SELECT id, status, error FROM resumes WHERE user_id = u.id ORDER BY created_at DESC LIMIT 1) r ON true
         LEFT JOIN LATERAL (SELECT id, confirmed_at FROM profiles WHERE user_id = u.id ORDER BY version DESC LIMIT 1) p ON true
         WHERE u.id = $1`,
        [req.userId],
      )
      .then(({ rows }) => {
        const r = rows[0];
        res.json({
          id: req.userId,
          resume: r?.resume_id ? { id: r.resume_id, status: r.resume_status, error: r.resume_error } : null,
          profile: r?.profile_id ? { id: r.profile_id, confirmed: r.confirmed === true } : null,
        });
      }, next);
  });

  app.delete("/api/me", requireUser, (req, res, next) => {
    deleteUser(pool, req.userId ?? "").then(() => {
      res.clearCookie(sessionCookie, cookieOptions);
      res.status(204).end();
    }, next);
  });

  if (deps.vault) {
    mountResumes(app, { pool, valkey, vault: deps.vault });
    mountAccount(app, { pool, valkey, vault: deps.vault });
    if (deps.corpus) mountTracker(app, { pool, valkey, corpus: deps.corpus });
    mountJobs(app, { pool, valkey, vault: deps.vault, ...(deps.corpus ? { corpus: deps.corpus } : {}), ...(deps.ai ? { ai: deps.ai } : {}), ...(deps.ranker ? { ranker: deps.ranker } : {}) });
  }

  // Exists only with NODE_ENV=test: loadConfig refuses the flag anywhere else.
  if (config.testAuthShortcut) {
    app.post("/test/login", (req: Request, res, next) => {
      const body = req.body as { subject?: unknown };
      const subject = typeof body.subject === "string" ? body.subject : "test";
      pool
        .query<{ user_id: string }>(
          `SELECT user_id FROM identities WHERE provider = 'github' AND subject = $1`,
          [subject],
        )
        .then((found) => {
          const userId = found.rows[0]?.user_id;
          if (!userId) {
            res.status(404).json({ error: "no_such_user" });
            return undefined;
          }
          return createSession(pool, userId).then((token) => {
            res.cookie(sessionCookie, token, cookieOptions);
            res.status(204).end();
          });
        })
        .catch(next);
    });
  }

  app.use((_req, res) => {
    res.status(404).json({ error: "not_found" });
  });

  // Logs the error's type only: messages can carry user data.
  app.use((error: Error, _req: Request, res: Response, next: (e: Error) => void) => {
    if (res.headersSent) {
      next(error);
      return;
    }
    const type = (error as { type?: string }).type;
    if (type === "entity.too.large") {
      res.status(413).json({ error: "too_large" });
      return;
    }
    if (type === "entity.parse.failed") {
      res.status(400).json({ error: "bad_request" });
      return;
    }
    log.error({ event: "unhandled_error", code: error.name });
    res.status(500).json({ error: "internal" });
  });

  return app;
}
