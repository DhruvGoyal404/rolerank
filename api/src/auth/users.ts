import type { Pool, PoolClient } from "pg";
import type { ProviderIdentity, ProviderName } from "./providers.js";

export interface SignupTerms {
  ageConfirmed: boolean;
  /** Asked only for a new account, after the age check: the per-address signup limit. */
  signupAllowed?: () => Promise<boolean>;
}

export type LoginResult =
  | { ok: true; userId: string; created: boolean }
  | { ok: false; reason: "age_required" | "signup_limited" };

async function findUser(
  db: Pool | PoolClient,
  provider: ProviderName,
  subject: string,
): Promise<string | null> {
  const { rows } = await db.query<{ user_id: string }>(
    `SELECT user_id FROM identities WHERE provider = $1 AND subject = $2`,
    [provider, subject],
  );
  return rows[0]?.user_id ?? null;
}

const UNIQUE_VIOLATION = "23505";

/**
 * Returning users sign in. A new user needs the 18+ confirmation (signup is open since
 * 2026-11-09; the invites table stays until a later release contracts it).
 * Identity is (provider, subject): the same email at another provider is another user.
 */
export async function loginOrSignup(
  pool: Pool,
  provider: ProviderName,
  identity: ProviderIdentity,
  terms: SignupTerms,
): Promise<LoginResult> {
  const existing = await findUser(pool, provider, identity.subject);
  if (existing) return { ok: true, userId: existing, created: false };

  if (!terms.ageConfirmed) return { ok: false, reason: "age_required" };
  if (terms.signupAllowed && !(await terms.signupAllowed())) return { ok: false, reason: "signup_limited" };

  const client = await pool.connect();
  try {
    await client.query("BEGIN");
    const user = await client.query<{ id: string }>(`INSERT INTO users (age_confirmed_at) VALUES (now()) RETURNING id`);
    const userId = user.rows[0]?.id;
    if (!userId) throw new Error("insert returned no user");
    await client.query(
      `INSERT INTO identities (provider, subject, user_id, email, email_verified)
       VALUES ($1, $2, $3, $4, $5)`,
      [provider, identity.subject, userId, identity.email, identity.emailVerified],
    );
    await client.query(`INSERT INTO audit_log (user_id, event) VALUES ($1, 'signup')`, [userId]);
    await client.query("COMMIT");
    return { ok: true, userId, created: true };
  } catch (error) {
    await client.query("ROLLBACK");
    // Two first logins raced: the winner's row stands.
    if ((error as { code?: string }).code === UNIQUE_VIOLATION) {
      const winner = await findUser(pool, provider, identity.subject);
      if (winner) return { ok: true, userId: winner, created: false };
    }
    throw error;
  } finally {
    client.release();
  }
}

/** Removes everything tied to the user, and leaves a tombstone for restores. */
export async function deleteUser(pool: Pool, userId: string): Promise<void> {
  const client = await pool.connect();
  try {
    await client.query("BEGIN");
    await client.query(`DELETE FROM users WHERE id = $1`, [userId]);
    // Cascades remove keys, resumes and profiles; the ledger's per-user rows are keyed by text.
    await client.query(`DELETE FROM llm_ledger WHERE scope = $1`, [userId]);
    await client.query(
      `INSERT INTO deletion_tombstones (user_id) VALUES ($1) ON CONFLICT DO NOTHING`,
      [userId],
    );
    await client.query(`INSERT INTO audit_log (user_id, event) VALUES ($1, 'account_deleted')`, [
      userId,
    ]);
    await client.query("COMMIT");
  } catch (error) {
    await client.query("ROLLBACK");
    throw error;
  } finally {
    client.release();
  }
}
