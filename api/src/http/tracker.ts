import type { Express, Request } from "express";
import type { Redis } from "iovalkey";
import type { Pool } from "pg";
import { RateLimiterRedis } from "rate-limiter-flexible";
import { rateLimit, requireUser } from "./middleware.js";

/**
 * The simple tracker (P6): save a job, set its status, remove it. Every row is read and written
 * through a query scoped by user_id; a missing or foreign id is 404, never 403.
 */
const STATUSES = ["saved", "applied", "interviewing", "offer", "rejected"] as const;
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const KEY = /^[1-9]\d{0,17}$/;
const MAX_ITEMS = 500;

interface Item {
  id: string;
  posting_id: string;
  title: string;
  company: string;
  url: string;
  status: string;
  updated_at: Date;
}

const shape = (r: Item) => ({ id: r.id, key: r.posting_id, title: r.title, company: r.company, url: r.url, status: r.status, updatedAt: r.updated_at });

export function mountTracker(app: Express, d: { pool: Pool; valkey: Redis; corpus: Pool }): void {
  const { pool, corpus } = d;
  const writes = new RateLimiterRedis({ storeClient: d.valkey, keyPrefix: "rl:tracker:m", points: 60, duration: 60 });
  const byUser = (req: Request) => req.userId ?? "";
  const id = (req: Request): string | null => (typeof req.params.id === "string" && UUID.test(req.params.id) ? req.params.id : null);

  app.get("/api/tracker", requireUser, (req, res, next) => {
    pool
      .query<Item>(
        `SELECT id, posting_id::text, title, company, url, status, updated_at FROM tracker_items WHERE user_id = $1 ORDER BY updated_at DESC`,
        [byUser(req)],
      )
      .then(({ rows }) => res.json({ items: rows.map(shape) }), next);
  });

  app.post("/api/tracker", requireUser, rateLimit(writes, byUser), (req, res, next) => {
    const key = (req.body as { key?: unknown } | undefined)?.key;
    if (typeof key !== "string" || !KEY.test(key)) {
      res.status(400).json({ error: "key_required" });
      return;
    }
    (async () => {
      const { rows: p } = await corpus.query<{ title: string; company: string; url: string }>(`SELECT title, company, url FROM postings WHERE id = $1`, [key]);
      const posting = p[0];
      if (!posting) return res.status(404).json({ error: "not_found" });
      const { rows: n } = await pool.query<{ n: number }>(`SELECT count(*)::int AS n FROM tracker_items WHERE user_id = $1`, [byUser(req)]);
      if ((n[0]?.n ?? 0) >= MAX_ITEMS) return res.status(409).json({ error: "tracker_full" });
      const { rows } = await pool.query<Item>(
        `INSERT INTO tracker_items (user_id, posting_id, title, company, url) VALUES ($1, $2, $3, $4, $5)
         ON CONFLICT (user_id, posting_id) DO UPDATE SET updated_at = tracker_items.updated_at
         RETURNING id, posting_id::text, title, company, url, status, updated_at`,
        [byUser(req), key, posting.title.slice(0, 400), posting.company.slice(0, 200), posting.url],
      );
      return res.status(201).json(rows[0] ? shape(rows[0]) : {});
    })().catch(next);
  });

  app.patch("/api/tracker/:id", requireUser, rateLimit(writes, byUser), (req, res, next) => {
    const itemId = id(req);
    const status = (req.body as { status?: unknown } | undefined)?.status;
    if (!itemId) {
      res.status(404).json({ error: "not_found" });
      return;
    }
    if (typeof status !== "string" || !(STATUSES as readonly string[]).includes(status)) {
      res.status(400).json({ error: "invalid_status" });
      return;
    }
    pool
      .query<Item>(
        `UPDATE tracker_items SET status = $3, updated_at = now() WHERE id = $1 AND user_id = $2
         RETURNING id, posting_id::text, title, company, url, status, updated_at`,
        [itemId, byUser(req), status],
      )
      .then(({ rows }) => (rows[0] ? res.json(shape(rows[0])) : res.status(404).json({ error: "not_found" })), next);
  });

  app.delete("/api/tracker/:id", requireUser, rateLimit(writes, byUser), (req, res, next) => {
    const itemId = id(req);
    if (!itemId) {
      res.status(404).json({ error: "not_found" });
      return;
    }
    pool
      .query(`DELETE FROM tracker_items WHERE id = $1 AND user_id = $2`, [itemId, byUser(req)])
      .then((r) => (r.rowCount ? res.status(204).end() : res.status(404).json({ error: "not_found" })), next);
  });
}
