import type { Pool, PoolClient } from "pg";
import { randomToken, sha256 } from "../crypto.js";

export const SESSION_DAYS = 30;
const REFRESH_AFTER_MS = 3_600_000;

export interface Session {
  userId: string;
}

/** Returns the cookie token. Only its hash is stored. */
export async function createSession(db: Pool | PoolClient, userId: string): Promise<string> {
  const token = randomToken();
  await db.query(
    `INSERT INTO sessions (token_sha256, user_id, expires_at)
     VALUES ($1, $2, now() + make_interval(days => $3))`,
    [sha256(token), userId, SESSION_DAYS],
  );
  return token;
}

export async function findSession(db: Pool, token: string): Promise<Session | null> {
  const hash = sha256(token);
  const { rows } = await db.query<{ user_id: string; last_seen_at: Date }>(
    `SELECT user_id, last_seen_at FROM sessions WHERE token_sha256 = $1 AND expires_at > now()`,
    [hash],
  );
  const row = rows[0];
  if (!row) return null;
  if (Date.now() - row.last_seen_at.getTime() > REFRESH_AFTER_MS) {
    await db.query(
      `UPDATE sessions SET last_seen_at = now(), expires_at = now() + make_interval(days => $2)
       WHERE token_sha256 = $1`,
      [hash, SESSION_DAYS],
    );
  }
  return { userId: row.user_id };
}

export async function deleteSession(db: Pool, token: string): Promise<void> {
  await db.query(`DELETE FROM sessions WHERE token_sha256 = $1`, [sha256(token)]);
}
