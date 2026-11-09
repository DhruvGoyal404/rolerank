import { randomBytes } from "node:crypto";
import { createServer } from "node:http";
import type { AddressInfo } from "node:net";
import { Writable } from "node:stream";
import { fileURLToPath } from "node:url";
import Anthropic from "@anthropic-ai/sdk";
import pg from "pg";
import { Gateway } from "../src/ai/gateway.js";
import { processNext, type Extractor } from "../src/ai/pipeline.js";
import { aiClient, createAiServer } from "../src/aiserver.js";
import { githubProvider, googleProvider } from "../src/auth/providers.js";
import { loadConfig } from "../src/config.js";
import { createApp } from "../src/http/app.js";
import { createLog } from "../src/log.js";
import type { Ranker } from "../src/ranker.js";
import { Vault } from "../src/vault.js";
import { createTestDatabases, testValkey, urlFor, valkeyUrl } from "./harness.js";

/**
 * The API for the web e2e suite (P6): the real app, the real resume pipeline and aiworker's real
 * endpoint, on throwaway databases whose names end in _test. Only the outside world is faked: the
 * extractor sandbox, the model and the ranker service. Run by web/playwright.config.ts after
 * scripts/test-env.sh; it listens on 127.0.0.1:8080 (the API) and :8079 (seeding users).
 */
const quiet = createLog(new Writable({ write: (_c, _e, done) => { done(); } }));
// Compiled into api/.e2e, the taxonomy's default path (relative to the module) misses it.
process.env.ROLERANK_TAXONOMY ??= fileURLToPath(new URL("../../../worker/internal/skills/taxonomy.txt", import.meta.url));

// The resume every e2e upload reads as. A file containing SCANNED has no text; PASSWORD is locked.
const LINES = [
  "Asha Rao | asha@example.com | +91 98765 43210",
  "B.Tech in Computer Science, NIT Trichy, 2026",
  "Backend Engineering Intern, Initech",
  "Jun 2025 - Aug 2025",
  "- Built a payments service in Golang and Postgres that cut manual review by 40%",
  "- Containerised the ingestion workers with Docker and wrote their runbooks",
  "Skills: Go, PostgreSQL, Docker, React, Redis",
].map((text, i) => ({ n: i + 1, text }));
const PROFILE = {
  level: "intern",
  skills: ["Go", "PostgreSQL", "Docker", "React", "Redis"].map((name) => ({ name, line: 7 })),
  experience: [{ employer: "Initech", title: "Backend Engineering Intern", kind: "internship", periods: [{ start: "2025-06", end: "2025-08" }], lines: [3, 4] }],
  education: [{ institution: "NIT Trichy", degree: "B.Tech in Computer Science", grad_year: 2026, score: "", lines: [2] }],
  projects: [],
  publications: [],
  achievements: [],
  certifications: [],
};
const extractor: Extractor = {
  extract: (file) => {
    if (file.includes(Buffer.from("SCANNED"))) return Promise.resolve({ status: "ok", lines: [] });
    if (file.includes(Buffer.from("PASSWORD"))) return Promise.resolve({ status: "failed", code: "password_protected" });
    if (file.includes(Buffer.from("SLOWSTART"))) return Promise.resolve({ status: "failed", code: "extract_startup" });
    return Promise.resolve({ status: "ok", lines: LINES });
  },
};

// The model, by task: the system prompt says which call it is.
function reply(system: string): string {
  if (system.startsWith("You extract a structured profile")) return JSON.stringify(PROFILE);
  if (system.startsWith("You write a short explanation"))
    return JSON.stringify({ explanation: "You have Go and PostgreSQL, which this role requires. It also asks for Kubernetes, which your resume doesn't show." });
  return JSON.stringify({ reasons: [] });
}

const days = (n: number) => new Date(Date.now() - n * 86_400_000).toISOString();
const POSTINGS = [
  { source: "greenhouse", title: "Backend Engineer Intern", company: "Initech Labs", level: "intern", location: "Bengaluru", req: ["Go", "PostgreSQL", "Kubernetes"], nice: ["Docker"], posted: days(2), apply: null, body: "We use Golang, Postgres and Kubernetes." },
  { source: "lever", title: "Data Engineer", company: "Northwind", level: "entry", location: "Pune", req: ["Python", "SQL"], nice: [], posted: days(40), apply: null, body: "Python and SQL pipelines." },
  { source: "himalayas", title: "Frontend Developer", company: "Acme Remote", level: "entry", location: "Remote, India", req: ["React", "TypeScript"], nice: [], posted: days(0), apply: "https://himalayas.app/companies/acme/jobs/frontend?ref=himalayas&x=1", body: "React and TypeScript." },
  { source: "remoteok", title: "Platform Engineering Intern", company: "Globex", level: "intern", location: "Remote", req: ["Docker", "Redis", "Kubernetes"], nice: [], posted: days(5), apply: null, body: "Docker, Redis, k8s." },
];
const BANDS = ["strong", "stretch", "good", "good"]; // as the real ranker names them

async function main(): Promise<void> {
  const dbs = await createTestDatabases();
  const pool = new pg.Pool({ connectionString: urlFor(dbs.roles.appRw, dbs.passwords.APP_RW ?? "", dbs.appDb) });
  const corpus = new pg.Pool({ connectionString: urlFor(dbs.roles.corpusRo, dbs.passwords.CORPUS_RO ?? "", dbs.corpusDb) });
  const rw = new pg.Pool({ connectionString: urlFor(dbs.roles.corpusRw, dbs.passwords.CORPUS_RW ?? "", dbs.corpusDb) });
  const ids: number[] = [];
  for (const p of POSTINGS) {
    const { rows: s } = await rw.query<{ id: number }>(`INSERT INTO sources (name) VALUES ($1) ON CONFLICT (name) DO UPDATE SET name = $1 RETURNING id`, [p.source]);
    const { rows } = await rw.query<{ id: number }>(
      `INSERT INTO postings (source_id, source_job_id, title, company, url, apply_url, level, location_raw, body, excerpt, content_hash,
                             skills_required, skills_nice, posted_at)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, '', '\\x00', $10, $11, $12) RETURNING id`,
      [s[0]?.id, `${p.source}-1`, p.title, p.company, `https://jobs.example.com/${p.source}/1`, p.apply, p.level, p.location, p.body, p.req, p.nice, p.posted],
    );
    ids.push(rows[0]?.id ?? 0);
  }
  await rw.end();

  const llm = createServer((req, res) => {
    let body = "";
    req.on("data", (c: Buffer) => (body += c.toString()));
    req.on("end", () => {
      const system = (JSON.parse(body) as { system?: string }).system ?? "";
      res.setHeader("content-type", "application/json");
      res.end(
        JSON.stringify({
          id: "m",
          type: "message",
          role: "assistant",
          model: "claude-haiku-5-5",
          content: [{ type: "text", text: reply(system) }],
          stop_reason: "end_turn",
          stop_sequence: null,
          usage: { input_tokens: 500, output_tokens: 150 },
        }),
      );
    });
  });
  await new Promise<void>((ok) => llm.listen(0, "127.0.0.1", ok));
  const vault = new Vault(randomBytes(32), 1);
  const gateway = new Gateway(new Anthropic({ apiKey: "e2e", baseURL: `http://127.0.0.1:${String((llm.address() as AddressInfo).port)}`, maxRetries: 0 }), pool);
  const token = randomBytes(24).toString("hex");
  const ai = createAiServer({ token, gateway, pool, vault, log: quiet }).listen(0, "127.0.0.1");
  await new Promise<void>((ok) => ai.once("listening", ok));
  // The resume queue, as aiworker runs it.
  // One job at a time, as aiworker runs it: overlapping calls can starve the pool.
  void (async () => {
    for (;;) {
      try {
        if ((await processNext({ pool, vault, extractor, gateway, log: quiet })) !== null) continue;
      } catch (e: unknown) {
        console.error("pipeline:", e instanceof Error ? `${e.name}: ${e.message}` : "unknown"); // test data only
      }
      await new Promise((r) => setTimeout(r, 300));
    }
  })();

  const ranker: Ranker = {
    search: () => Promise.resolve(ids.map((id, i) => ({ id, band: BANDS[i] ?? "Good" }))),
    score: (_q, id) => Promise.resolve({ id, band: BANDS[ids.indexOf(id)] ?? "Good" }),
  };
  const config = loadConfig({
    NODE_ENV: "test",
    PUBLIC_ORIGIN: "http://localhost:5173",
    APP_DATABASE_URL: urlFor(dbs.roles.appRw, dbs.passwords.APP_RW ?? "", dbs.appDb),
    VALKEY_URL: valkeyUrl(),
    GOOGLE_CLIENT_ID: "g",
    GOOGLE_CLIENT_SECRET: "g",
    GITHUB_CLIENT_ID: "h",
    GITHUB_CLIENT_SECRET: "h",
    TEST_AUTH_SHORTCUT: "1",
  });
  const valkey = testValkey();
  const app = createApp({
    config,
    pool,
    valkey,
    vault,
    corpus,
    ranker,
    ai: aiClient(`http://127.0.0.1:${String((ai.address() as AddressInfo).port)}`, token),
    log: quiet,
    providers: { google: googleProvider(config.google), github: githubProvider(config.github) },
  });
  app.listen(8080, "127.0.0.1");

  // Seeding: a user with a GitHub identity, so /test/login can sign them in; rate limits reset.
  createServer((req, res) => {
    let body = "";
    req.on("data", (c: Buffer) => (body += c.toString()));
    req.on("end", () => {
      const subject = (JSON.parse(body || "{}") as { subject?: string }).subject ?? "";
      (async () => {
        const { rows } = await pool.query<{ id: string }>(`INSERT INTO users (age_confirmed_at) VALUES (now()) RETURNING id`);
        await pool.query(`INSERT INTO identities (provider, subject, user_id) VALUES ('github', $1, $2)`, [subject, rows[0]?.id]);
        res.end(JSON.stringify({ id: rows[0]?.id }));
      })().catch(() => {
        res.statusCode = 500;
        res.end("{}");
      });
    });
  }).listen(8079, "127.0.0.1");

  const stop = () => {
    void dbs.drop().finally(() => process.exit(0));
  };
  process.on("SIGTERM", stop);
  process.on("SIGINT", stop);
  console.log("e2e api ready");
}

main().catch((error: unknown) => {
  console.error("e2e server failed:", error instanceof Error ? error.message : "unknown");
  process.exit(1);
});
