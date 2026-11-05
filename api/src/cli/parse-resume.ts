import { readFileSync } from "node:fs";
import Anthropic from "@anthropic-ai/sdk";
import pg from "pg";
import { stripContacts } from "../ai/contacts.js";
import { Gateway, PARSE_PROMPT_VERSION } from "../ai/gateway.js";
import { httpExtractor } from "../ai/pipeline.js";
import { experienceMonths } from "../ai/profile.js";

/**
 * Parses one local resume through the real sandbox and the real model, and prints the draft
 * profile, for the owner to check before the review screen exists. Run by
 * scripts/parse-resume.sh. Nothing about the resume is stored: only the ledger's cost rows, in
 * the local dev database, under a fixed local user.
 */
const LOCAL_USER = "00000000-0000-4000-8000-00000000c11a";

const file = process.argv[2];
if (!file) {
  console.error("usage: parse-resume <file.pdf|file.docx>");
  process.exit(2);
}
const need = (name: string): string => {
  const v = process.env[name];
  if (!v) {
    console.error(`${name} is not set`);
    process.exit(2);
  }
  return v;
};

const pool = new pg.Pool({ connectionString: need("APP_DATABASE_URL"), max: 2 });
try {
  await pool.query(`INSERT INTO users (id, age_confirmed_at) VALUES ($1, now()) ON CONFLICT (id) DO NOTHING`, [LOCAL_USER]);
  const extracted = await httpExtractor(need("EXTRACTOR_URL"), need("EXTRACTOR_TOKEN")).extract(readFileSync(file));
  if (extracted.status !== "ok") {
    console.error(`extraction failed: ${extracted.code}`);
    process.exit(1);
  }
  const lines = extracted.lines;
  const hidden = lines.filter((l) => l.hidden || l.tiny);
  const stripped = stripContacts(lines);
  console.log(`Read ${lines.length} lines.`);
  if (hidden.length) console.log(`Hidden or tiny text, never sent to the model: lines ${hidden.map((l) => l.n).join(", ")}`);
  console.log(`Contact details removed before the model: ${stripped.contacts.map((c) => c.kind).join(", ") || "none"}`);
  if (lines.filter((l) => !l.hidden).length === 0) {
    console.error("no text: a scanned image? Scanned resumes aren't supported in v1.");
    process.exit(1);
  }

  const started = Date.now();
  const r = await new Gateway(new Anthropic({ apiKey: need("ANTHROPIC_API_KEY"), maxRetries: 2 }), pool).parseResume(
    LOCAL_USER,
    stripped.lines,
  );
  if (r.outcome !== "ok" || !r.grounded) {
    console.error(`parse ${r.outcome} after ${r.attempts} attempt(s)`);
    process.exit(1);
  }
  const { profile, dropped, droppedBy, school } = r.grounded;
  const { rows } = await pool.query<{ usd: number }>(
    `SELECT COALESCE(cost_usd, 0)::float AS usd FROM llm_calls WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1`,
    [LOCAL_USER],
  );
  console.log(`\nDraft profile (prompt ${PARSE_PROMPT_VERSION}, ${((Date.now() - started) / 1000).toFixed(1)} s, $${(rows[0]?.usd ?? 0).toFixed(5)}):`);
  console.log(JSON.stringify(profile, null, 2));
  console.log(`\nWork experience: ${experienceMonths(profile)} months (jobs and internships, overlaps once; programmes excluded).`);
  if (school) console.log(`School rows left out of education: ${school}.`);
  console.log(
    dropped
      ? `Dropped by the grounding gate (a value not found in the line it cited): ${JSON.stringify(droppedBy)}`
      : "Nothing dropped by the grounding gate.",
  );
  console.log("\nEach value cites its resume line (L<n>):");
  const byN = new Map(stripped.lines.map((l) => [l.n, l.text]));
  for (const s of profile.skills.slice(0, 5)) console.log(`  skill ${s.name}  <- L${s.line}: ${byN.get(s.line) ?? ""}`);
} finally {
  await pool.end();
}
