import type { NextFunction, Request, RequestHandler, Response } from "express";
import type { Pool } from "pg";
import type { RateLimiterAbstract } from "rate-limiter-flexible";
import { findSession } from "../auth/sessions.js";

const SAFE_METHODS = new Set(["GET", "HEAD", "OPTIONS"]);

export function parseCookies(header: string | undefined): Map<string, string> {
  const cookies = new Map<string, string>();
  for (const part of (header ?? "").split(";")) {
    const index = part.indexOf("=");
    if (index > 0) cookies.set(part.slice(0, index).trim(), part.slice(index + 1).trim());
  }
  return cookies;
}

function originOf(value: string): string | null {
  try {
    return new URL(value).origin;
  } catch {
    return null;
  }
}

/** Every state-changing request must come from our own site. */
export function originCheck(publicOrigin: string): RequestHandler {
  return (req, res, next) => {
    if (SAFE_METHODS.has(req.method)) {
      next();
      return;
    }
    const origin = req.get("origin");
    const referer = req.get("referer");
    const source = origin ?? (referer ? originOf(referer) : null);
    if (source !== publicOrigin) {
      res.status(403).json({ error: "bad_origin" });
      return;
    }
    next();
  };
}

export function rateLimit(limiter: RateLimiterAbstract): RequestHandler {
  return (req, res, next) => {
    limiter.consume(req.ip ?? "unknown").then(
      () => {
        next();
      },
      (rejection: unknown) => {
        if (rejection instanceof Error) {
          next(rejection);
          return;
        }
        const wait = (rejection as { msBeforeNext: number }).msBeforeNext;
        res.set("Retry-After", String(Math.ceil(wait / 1000)));
        res.status(429).json({ error: "rate_limited" });
      },
    );
  };
}

declare module "express-serve-static-core" {
  interface Request {
    userId?: string;
  }
}

export function sessionLoader(pool: Pool, cookieName: string): RequestHandler {
  return (req: Request, _res: Response, next: NextFunction) => {
    const token = parseCookies(req.headers.cookie).get(cookieName);
    if (!token) {
      next();
      return;
    }
    findSession(pool, token).then((session) => {
      if (session) req.userId = session.userId;
      next();
    }, next);
  };
}

export const requireUser: RequestHandler = (req, res, next) => {
  if (!req.userId) {
    res.status(401).json({ error: "unauthenticated" });
    return;
  }
  next();
};
