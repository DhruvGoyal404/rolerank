import type { Pool } from "pg";

/**
 * The LLM spend ledger (ARCHITECTURE.md, "LLM gateway"). Caps are enforced in code, per user and
 * globally, in USD per UTC day (Q8: $0.05 and $1.00). A call reserves its worst case before it
 * runs, inside one transaction that locks both rows, and settles to its real cost after. A
 * reservation never settled within 10 minutes is counted in full, since the call may have
 * happened. A cap is never bypassed: at the cap, reserve returns null.
 */
export interface Caps {
  userUsd: number;
  globalUsd: number;
}

export const DEFAULT_CAPS: Caps = { userUsd: 0.05, globalUsd: 1.0 };

/** Claude Haiku 5.5, prompts of 100K tokens or fewer, USD per token (checked 2026-11-05). */
export const HAIKU_55 = { model: "claude-haiku-5-5", inputUsd: 0.1 / 1e6, outputUsd: 0.5 / 1e6, maxPromptTokens: 100_000 };

export function cost(inputTokens: number, outputTokens: number, price = HAIKU_55): number {
  return inputTokens * price.inputUsd + outputTokens * price.outputUsd;
}

const today = (): string => new Date().toISOString().slice(0, 10);

export interface Reservation {
  id: string;
  reservedUsd: number;
}

export async function reserve(
  pool: Pool,
  call: { userId: string; task: string; model: string; promptVersion: string; maxUsd: number; inputHash?: Buffer },
  caps: Caps = DEFAULT_CAPS,
): Promise<Reservation | null> {
  const day = today();
  const client = await pool.connect();
  try {
    await client.query("BEGIN");
    await client.query(
      `INSERT INTO llm_ledger (day, scope) VALUES ($1, 'global'), ($1, $2) ON CONFLICT (day, scope) DO NOTHING`,
      [day, call.userId],
    );
    // Global first, always: one lock order, so concurrent reservations can't deadlock.
    const { rows } = await client.query<{ scope: string; used: string }>(
      `SELECT scope, (reserved_usd + spent_usd)::text AS used FROM llm_ledger
       WHERE day = $1 AND scope IN ('global', $2) ORDER BY scope = 'global' DESC FOR UPDATE`,
      [day, call.userId],
    );
    const used = new Map(rows.map((r) => [r.scope, Number(r.used)]));
    if (
      (used.get("global") ?? 0) + call.maxUsd > caps.globalUsd ||
      (used.get(call.userId) ?? 0) + call.maxUsd > caps.userUsd
    ) {
      await client.query("ROLLBACK");
      return null;
    }
    await client.query(
      `UPDATE llm_ledger SET reserved_usd = reserved_usd + $3 WHERE day = $1 AND scope IN ('global', $2)`,
      [day, call.userId, call.maxUsd],
    );
    const { rows: ins } = await client.query<{ id: string }>(
      `INSERT INTO llm_calls (day, user_id, task, model, prompt_version, reserved_usd, input_hash)
       VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
      [day, call.userId, call.task, call.model, call.promptVersion, call.maxUsd, call.inputHash ?? null],
    );
    await client.query("COMMIT");
    return { id: ins[0]?.id ?? "", reservedUsd: call.maxUsd };
  } catch (error) {
    await client.query("ROLLBACK");
    throw error;
  } finally {
    client.release();
  }
}

export interface Outcome {
  costUsd: number;
  inputTokens?: number;
  outputTokens?: number;
  latencyMs?: number;
  outcome: string; // ok, invalid, refused, error, expired: never content
}

/** Replaces a reservation with what the call really cost. Settling twice does nothing. */
export async function settle(pool: Pool, id: string, o: Outcome): Promise<void> {
  const client = await pool.connect();
  try {
    await client.query("BEGIN");
    const { rows } = await client.query<{ day: string; user_id: string | null; reserved: string }>(
      `SELECT day::text, user_id, reserved_usd::text AS reserved FROM llm_calls
       WHERE id = $1 AND settled_at IS NULL FOR UPDATE`,
      [id],
    );
    const call = rows[0];
    if (!call) {
      await client.query("ROLLBACK");
      return;
    }
    // A deleted user's row is gone with them; the global row still settles.
    await client.query(
      `UPDATE llm_ledger SET reserved_usd = GREATEST(reserved_usd - $3, 0), spent_usd = spent_usd + $4
       WHERE day = $1 AND (scope = 'global' OR scope = $2)`,
      [call.day, call.user_id ?? "", Number(call.reserved), o.costUsd],
    );
    await client.query(
      `UPDATE llm_calls SET cost_usd = $2, input_tokens = $3, output_tokens = $4, latency_ms = $5, outcome = $6,
         settled_at = now() WHERE id = $1`,
      [id, o.costUsd, o.inputTokens ?? null, o.outputTokens ?? null, o.latencyMs ?? null, o.outcome],
    );
    await client.query("COMMIT");
  } catch (error) {
    await client.query("ROLLBACK");
    throw error;
  } finally {
    client.release();
  }
}

/** Counts every reservation older than the limit in full and closes it. Returns how many. */
export async function expireStale(pool: Pool, olderThanMinutes = 10): Promise<number> {
  const { rows } = await pool.query<{ id: string; reserved: string }>(
    `SELECT id, reserved_usd::text AS reserved FROM llm_calls
     WHERE settled_at IS NULL AND created_at < now() - make_interval(mins => $1)`,
    [olderThanMinutes],
  );
  for (const r of rows) await settle(pool, r.id, { costUsd: Number(r.reserved), outcome: "expired" });
  return rows.length;
}
