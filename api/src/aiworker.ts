import Anthropic from "@anthropic-ai/sdk";
import pg from "pg";
import { z } from "zod";
import { Gateway } from "./ai/gateway.js";
import { expireStale } from "./ai/ledger.js";
import { httpExtractor, processNext } from "./ai/pipeline.js";
import { createLog } from "./log.js";
import { Vault } from "./vault.js";

/**
 * aiworker: takes queued resumes from the database, runs them through the sandboxed extractor
 * and the parse gateway, and settles stale ledger reservations. It owns the only LLM client.
 */
const env = z
  .object({
    APP_DATABASE_URL: z.string().min(1),
    ROLERANK_KEK: z.string().min(1),
    ROLERANK_KEK_VERSION: z.coerce.number().int().min(1).default(1),
    EXTRACTOR_URL: z.url(),
    EXTRACTOR_TOKEN: z.string().min(32),
    ANTHROPIC_API_KEY: z.string().min(1),
  })
  .safeParse(process.env);

const log = createLog();
if (!env.success) {
  // Names the fields, never their values.
  console.error(`invalid configuration: ${env.error.issues.map((i) => i.path.join(".")).join(", ")}`);
  process.exit(1);
}
const kek = Buffer.from(env.data.ROLERANK_KEK, "base64");
if (kek.length !== 32) {
  console.error("ROLERANK_KEK must be 32 bytes, base64");
  process.exit(1);
}

const pool = new pg.Pool({ connectionString: env.data.APP_DATABASE_URL, statement_timeout: 30_000, max: 4 });
const deps = {
  pool,
  vault: new Vault(kek, env.data.ROLERANK_KEK_VERSION),
  extractor: httpExtractor(env.data.EXTRACTOR_URL, env.data.EXTRACTOR_TOKEN),
  gateway: new Gateway(new Anthropic({ apiKey: env.data.ANTHROPIC_API_KEY, maxRetries: 2, timeout: 60_000 }), pool),
  log,
};

const state = { stopping: false };
process.on("SIGTERM", () => (state.stopping = true));
process.on("SIGINT", () => (state.stopping = true));

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));
let lastExpiry = 0;
log.info({ event: "aiworker_started" });
while (!state.stopping) {
  try {
    if (Date.now() - lastExpiry > 60_000) {
      await expireStale(pool);
      lastExpiry = Date.now();
    }
    const outcome = await processNext(deps);
    // Busy: take the next one at once. Idle, or waiting for budget: poll again shortly.
    if (outcome === null || outcome === "capped") await sleep(outcome === "capped" ? 60_000 : 2000);
  } catch (error) {
    log.error({ event: "aiworker_error", type: error instanceof Error ? error.name : "unknown" });
    await sleep(5000);
  }
}
await pool.end();
log.info({ event: "aiworker_stopped" });
