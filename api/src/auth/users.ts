import type { Pool, PoolClient } from "pg";
import { sha256 } from "../crypto.js";
import type { ProviderIdentity, ProviderName } from "./providers.js";

export interface SignupTerms {
  inviteCode: string | null;
  ageConfirmed: boolean;
}

export type LoginResult =
  | { ok: true; userId: string; created: boolean }
  | { ok: false; reason: "invite_required" | "age_required" };

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
 * Returning users sign in. A new user needs a valid invite and the 18+ confirmation.
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
  if (!terms.inviteCode) return { ok: false, reason: "invite_required" };

  const client = await pool.connect();
  try {
    await client.query("BEGIN");
    const invite = await client.query<{ id: string }>(
      `UPDATE invites SET uses_left = uses_left - 1
       WHERE code_sha256 = $1 AND uses_left > 0 AND expires_at > now()
       RETURNING id`,
      [sha256(terms.inviteCode)],
    );
    const inviteId = invite.rows[0]?.id;
    if (!inviteId) {
      await client.query("ROLLBACK");
      return { ok: false, reason: "invite_required" };
    }
    const user = await client.query<{ id: string }>(
      `INSERT INTO users (age_confirmed_at, invite_id) VALUES (now(), $1) RETURNING id`,
      [inviteId],
    );
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
    // Two first logins raced: the winner's row stands and the invite use is rolled back.
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
