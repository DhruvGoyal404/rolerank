// rolerank-up.sh's model check, run inside the aiworker container: one fixed call through the
// gateway under the ledger's ops scope and its own daily cap. Prints the outcome, never content.
// Exit 0 only on "ok".
import Anthropic from "@anthropic-ai/sdk";
import pg from "pg";
import { Gateway } from "../ai/gateway.js";

const url = process.env.APP_DATABASE_URL;
const key = process.env.ANTHROPIC_API_KEY;
if (!url || !key) {
  console.error("APP_DATABASE_URL and ANTHROPIC_API_KEY are required");
  process.exit(1);
}
const pool = new pg.Pool({ connectionString: url, max: 1 });
const outcome = await new Gateway(new Anthropic({ apiKey: key, maxRetries: 2, timeout: 60_000 }), pool).check();
await pool.end();
console.log(`llm-check: ${outcome}`);
process.exit(outcome === "ok" ? 0 : 1);
