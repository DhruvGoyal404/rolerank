# Architecture

Design for v1. Nothing is built yet. Versions were resolved on **2026-10-31** against
npm, the Go module proxy, GitHub releases, Docker Hub and endoflife.date, applying the
14-day rule (published on or before 2026-10-17). Where the newest release was too
young, the table names both.

## Overview

```
                         Browser (React SPA)
                                |
                     HTTPS  [TB1]
                                v
                 +--------------------------+
                 |  Caddy: TLS, CSP, static |
                 +--------------------------+
                          | /api [TB2]
                          v
 +------------------------------------------+        +-------------------+
 |  api (Node, Express, TS)                  |<------>| Valkey            |
 |  auth, sessions, profiles, matches,       |  XADD  | streams, limits,  |
 |  tracker, Bring a JD, rate limits         |------->| OAuth state       |
 +------------------------------------------+        +-------------------+
     |  user data                | vector + filters          ^   |  XREADGROUP
     v  (PII)                    v  (no PII) [TB6]           |   v
 +-----------+          +------------------+    +---------------------------+
 | Postgres  |          | Postgres corpus  |<---| ingest (Go)               |
 | app       |          | + pgvector + FTS |    | schedule, fetch, normalise|
 +-----------+          +------------------+    | dedupe, classify, embed   |
     ^                                           +---------------------------+
     |                                                 | [TB5] 1 req/s/host
     |   +-----------------------------+               v
     +---| aiworker (Node, TS)          |      Greenhouse, Lever, Ashby,
         | parse, explain, tailor       |      Himalayas, RemoteOK
         | LLM gateway + spend ledger   |
         +-----------------------------+
            |  [TB3] bytes in, text out        | [TB4] no contact details
            v                                  v
   +--------------------------+         LLM provider (Q5)
   | extractor (Go) runs each |
   | file in a gVisor sandbox |
   | no network, read-only    |
   +--------------------------+

 Off-host: S3 backups (create-only, age-encrypted) [TB8], SSM secrets, SES email,
           CloudWatch alarms, budget alert. GitHub Actions -> AWS via OIDC [TB7].
```

Trust boundaries (TB1 to TB9) are defined in [THREAT_MODEL.md](THREAT_MODEL.md).

## Components

| Component | Language | Responsibility | Scales by |
|---|---|---|---|
| `web` | TypeScript, React | UI. Renders all third-party and model text as text | Static files |
| `api` | TypeScript, Express | HTTP API, OAuth, sessions, Origin check, rate limits, matching queries, tracker | More processes behind Caddy |
| `aiworker` | TypeScript | Consumes `parse`, `explain`, `tailor` streams. Owns the only LLM client, the schemas, validators, grounding gate and spend ledger | Consumers in the group; capped by the ledger, not CPU |
| `extractor` | Go | Consumes `extract`. Starts one gVisor container per file, reads text and layout metadata over stdout | Containers per host |
| `ingest` | Go | Scheduler, fetchers, adapters, normaliser, deduper, classifiers, embedder | Workers in the group; the shared per-host limiter caps the rate |
| `db-app` | Postgres database `app` | Users, profiles, tracker, ledger. Holds PII | Vertical |
| `db-corpus` | Postgres database `corpus`, with pgvector | Postings, boards, embeddings, indexes. No PII | Vertical; read replica later |
| `valkey` | Valkey | Streams, rate-limit buckets, OAuth `state`, short caches | Vertical |
| `caddy` | Caddy | TLS, HTTP/3, security headers, static SPA | n/a |

Why two languages: the same split as CodeJudge. Go for the long-running, I/O-bound
workers and the sandbox runner, where its concurrency and single static binary pay
off and CodeJudge's runner code can be reused. TypeScript for everything a user
touches, including the LLM gateway, so schemas (Zod) are shared between the API and
the validators.

**One Postgres cluster, two databases.** `app` and `corpus` run in one Postgres 18
cluster, which is simpler to run, back up and bring up than two. PII and public data are
still kept apart by credentials, not by convention:

| Role | Can connect to | Used by |
|---|---|---|
| `app_rw` | `app` only | `api`, `aiworker` |
| `corpus_rw` | `corpus` only | `ingest` |
| `corpus_ro` | `corpus` only | `api` (search and job cards) |
| `migrator_app`, `migrator_corpus` | One database each | dbmate, at deploy time |

`CONNECT` is revoked from `PUBLIC` on both databases, there are no cross-database grants,
and `dblink` and `postgres_fdw` are not installed. Tests prove each role is refused by
the other database, and that no PII column exists in `corpus`. The corpus can be
dropped, rebuilt or dumped without touching user data, and `pg_dump` works per database,
so the backup split still holds.

What this gives up: the databases share memory, WAL, CPU and one superuser, so a heavy
ingest can slow user queries, and a Postgres-level compromise crosses both (THREAT_MODEL.md,
R11). **Split into two clusters only if the P3 bench shows interference, with numbers:**
`/api/matches` p95 with `ingest` at full rate more than 25% above p95 with `ingest`
stopped, and the 200 ms target missed because of it.

## Queues: reuse the CodeJudge pattern

Valkey Streams with consumer groups, exactly as CodeJudge proved it:

- The DB row and an outbox entry are written in one transaction. A sweeper re-enqueues
  anything that never reached the stream.
- Consumers `XACK` only after the result commits. A dead consumer's entries are taken
  back with `XAUTOCLAIM`.
- Results use fenced writes (`WHERE attempt = $mine AND status = 'running'`), so a stale
  worker can't overwrite a newer result.
- Messages carry IDs only, never resume text or PII.

Streams: `ingest:fetch` (board IDs), `ingest:enrich` (posting IDs), `ingest:embed`,
`ai:extract`, `ai:parse`, `ai:explain`, `ai:tailor`, `alerts:compute`. Each is capped
with `MAXLEN ~`.

## Ingestion pipeline

```
 scheduler --(next_due boards)--> ingest:fetch
   fetch worker:
     robots.txt check (24 h cache) -> per-host token from Valkey (Lua, shared)
     GET with If-None-Match
       304 -> boards.last_checked = now; done
       200 -> adapter -> normalised postings
       404 x3 -> board retired
       429/503 -> Retry-After, backoff, circuit breaker
     upsert postings by (source, source_job_id); content_hash decides "changed"
     postings missing from a COMPLETE board response -> closed_at = now
       (guard: if the board's count drops by > 50% at once, hold and recheck next cycle)
     changed postings -> ingest:enrich
   enrich worker:
     HTML -> plain text; language; is_engineering; level; employment type;
     geography (countries[], remote_scope); skills required / nice-to-have (taxonomy);
     dedupe key + MinHash signature -> cluster
     in scope and text changed -> ingest:embed
   embed worker: batch of 64 -> embeddings -> corpus
```

- **Refresh:** every 6 hours per board while up, plus a catch-up cycle at `up`. Board
  order is shuffled within each host so one big ATS doesn't monopolise the window.
- **Budget check:** at 1 request per second per host, one host allows 21,600 requests per
  6-hour cycle. Greenhouse at ~3,000 boards needs ~3,000 list calls plus detail calls
  only for new or changed jobs. Fits, with room for retries.
- **Closed detection** per source is in [SOURCES.md](SOURCES.md). Closed postings stay
  for 30 days (for tracker snapshots and dedupe), then are deleted.
- **Classifiers** are deterministic rules first: title patterns, `employmentType`,
  experience phrases ("0-1 years", "2026 graduates"), location parsing against a
  gazetteer of Indian cities and states. An LLM is not used per posting: 2 lakh calls a
  cycle is the wrong cost and latency trade. Measured by EVALS.md, D6.
- **Skills:** a curated taxonomy (~1,500 tech skills with aliases, for example `k8s`
  for Kubernetes, `react.js` for React), matched with a deterministic multi-pattern
  matcher over sections we detect ("requirements", "nice to have"). We write the list
  ourselves; ESCO was considered, but its reuse licence wasn't found on the page we
  read, so it's **unknown** and unused.
- **Dedupe:** exact on `(source, source_job_id)`; Greenhouse's `internal_job_id` groups
  posts of one job; cross-source blocks on normalised company + title + city, then
  MinHash Jaccard >= 0.9 on 5-word shingles. Search returns one posting per cluster,
  preferring the employer's ATS over an aggregator.

## Search and ranking

The LLM never ranks. A user's ranking is deterministic and can be recomputed from stored
features.

1. **Query from the confirmed profile:** skills (canonical), target titles, level,
   locations, remote preference, job types; an embedding of a short profile text
   (skills, titles, project one-liners). The embedding is computed once per profile
   version, stored in `db-app`, and sent to `db-corpus` per query. It is never stored there.
2. **Candidate generation** on a narrow `search_postings` table holding only active,
   in-scope rows, filtered to level- and geo-eligible postings:
   - vector: top 200 by cosine. Exact search over the filtered subset when it's small,
     which is the normal case. HNSW, with `hnsw.iterative_scan = relaxed_order`, only
     for broad queries. The Phase 0 spike below shows why;
   - lexical: top 200 by overlap of canonical skills and title terms, using a GIN index
     on the skill array. `tsvector` search serves typed keyword queries only;
   - fused with Reciprocal Rank Fusion (k = 60), as pgvector's README suggests.
3. **Rerank** the fused 300 with a linear score over explainable features:
   required-skill coverage, nice-to-have coverage, cosine, title similarity, level fit,
   location fit, freshness decay. Weights are fitted on D2 and checked on D3; they're a
   short list of numbers in config, versioned with the eval report that produced them.
4. **Fit band:** Strong, Good or Stretch, from score thresholds fitted on D3 grades. The
   UI shows the band and its reasons, never a percentage that implies false precision.
5. **Explanation without an LLM:** matched skills = required ∩ profile; missing =
   required − profile; plus level and location verdicts. The optional LLM paragraph may
   only rephrase these lists; a gate rejects any skill it names that isn't in them.

Hard filters (never violated): job type, level window, geography. A fresher never sees a
"5+ years" posting unless they widen the filter.

Results are stored per profile version in `match_snapshots`. Paging and client filters
read them. Steps 1 to 4 rerun when a profile changes, and for new postings after a refresh.

**Why Postgres and not a search engine.** One corpus of 50,000 to 1.25 lakh in-scope
postings fits in memory on one host. pgvector and Postgres cover both retrievers, the
filters and the joins in one query, with one backup story. Meilisearch, OpenSearch or
Qdrant would add a service to run, secure, back up and bring up in the up/down budget.
This choice is a hypothesis until P3's bench confirms it. If the bench misses p95 < 200
ms, the order of escalation is: tune the narrow table and filters, partial indexes per
region and level, a read replica, then a dedicated engine.
VectorChord was considered and rejected for licence (AGPL-3.0 or ELv2) and because the
corpus doesn't need its scale.

## Resume pipeline

```
 upload (<= 5 MB) -> api: magic bytes (PDF %PDF-, DOCX zip with [Content_Types].xml)
   -> encrypted to S3 quarantine -> ai:extract
 extractor: fresh gVisor container, no network, read-only, 512 MB, 1 CPU, 20 s,
   bytes via stdin -> {lines[], layout flags: hidden/tiny/white text} via stdout (<= 200 KB)
   -> original deleted (Q10)
 aiworker: strip contact details (email, phone, URLs, address patterns) into a
   separate encrypted field; reserve spend; LLM with JSON schema (no tools);
   Zod validation; grounding check (every value present in the text; line refs valid);
   settle spend -> draft profile
 user reviews and confirms -> profile version -> embedding -> matches
```

- PDF extraction runs pdf.js (`pdfjs-dist`) with scripting and eval off. DOCX runs
  `mammoth` after a zip pre-check: entry count, declared sizes, ratio and a 20 MB
  decompressed cap. Both inside the sandbox, so a bug in either is contained.
- Resume lines are numbered at extraction. Everything downstream cites `L<n>`, which is
  what makes the grounding gate for tailoring possible.

## LLM gateway

One module in `aiworker`, used for parse, explain and tailor.

- **Inputs** are delimited data blocks, after a system prompt that says the blocks are
  data and may contain instructions to ignore. No resume or JD text is ever placed in
  the system prompt.
- **Outputs** use the provider's JSON-schema constrained output, then Zod. Invalid
  output is retried once with the same input, then the task fails visibly.
- **No tools, no browsing, no function calls.**
- **Spend ledger:** before a call, reserve `max_tokens x price` against the user's and
  the global daily rows in one transaction (`SELECT ... FOR UPDATE`). Refuse if either
  would pass its cap. After the call, settle with actual tokens. Reservations older than
  10 minutes are released. A concurrency test proves caps hold under parallel requests.
- **Degradation:** at a cap, explanations fall back to the deterministic version,
  tailoring says "come back tomorrow", parsing waits in the queue.
- **Logged:** model, prompt version, schema version, tokens, cost, latency, outcome,
  input hash. **Never** input or output text.
- **Tailoring grounding gate:** described in EVALS.md. Implemented as a pure function
  with its own unit tests and property tests.

## Embeddings

Default (Q6): `BAAI/bge-small-en-v1.5` (MIT, 384 dims), run in the Go `ingest` worker
with ONNX Runtime through `github.com/yalue/onnxruntime_go`. The model file is pinned by
its Hugging Face revision hash and SHA-256. Stored as `halfvec(384)`: half the memory of
`vector`, and HNSW supports `halfvec` up to 4,000 dimensions.

Bake-off in P3 (EVALS.md): bge-small (local), Titan Text Embeddings V2 (Bedrock, Mumbai,
512 dims), OpenAI `text-embedding-3-small`. Same pool, same reranker. The model name and
version are stored on every row, so a switch is a re-embed, not a migration.

## Data model

### `db-app` (PII)

| Table | Key columns | Notes |
|---|---|---|
| `users` | `id`, `created_at`, `age_confirmed_at`, `invite_id` | |
| `identities` | `(provider, subject)` unique, `user_id`, `email`, `email_verified` | Login keys on provider + subject |
| `sessions` | `token_sha256` PK, `user_id`, `created_at`, `last_seen_at`, `expires_at` | |
| `user_keys` | `user_id`, `wrapped_dek`, `kek_version` | Deleting the row crypto-shreds the user |
| `resumes` | `id`, `user_id`, `status`, `text_ct`, `contacts_ct`, `flags`, `created_at` | `*_ct` = AES-256-GCM ciphertext, AAD binds user and resume |
| `profiles` | `id`, `user_id`, `resume_id`, `version`, `data_ct`, `skills text[]`, `level`, `embedding halfvec(384)`, `embed_model`, `confirmed_at` | Skills and embedding in clear for queries; disk is encrypted too |
| `preferences` | `user_id`, `job_types`, `locations`, `remote`, `target_titles`, `grad_year` | |
| `tracker_items` | `id`, `user_id`, `posting_key`, `snapshot jsonb`, `state`, `notes_ct`, `updated_at` | Snapshot keeps title, company, link after closure |
| `match_snapshots` | `user_id`, `posting_key`, `band`, `score`, `computed_at` | For alerts: "new since last time" |
| `alert_settings`, `email_suppressions` | | |
| `tasks` | `id`, `user_id`, `kind`, `status`, `attempt`, `stream_entry_id`, `error_code` | Outbox and status for async work |
| `llm_ledger` | `id`, `user_id`, `day`, `feature`, `model`, `reserved_usd`, `actual_usd`, `tokens_in`, `tokens_out`, `status` | 90 days |
| `llm_daily` | `(scope, day)` PK, `reserved_usd`, `spent_usd` | Scope = user ID or `global` |
| `audit_log` | `id`, `user_id`, `event`, `at`, `ip_prefix` | No content |
| `invites` | `code_sha256`, `uses_left`, `expires_at` | Q2 |
| `deletion_tombstones` | `user_id`, `deleted_at` | Also appended to S3 (create-only) so it survives a restore |

### `db-corpus` (no PII)

| Table | Key columns | Notes |
|---|---|---|
| `sources` | `id`, `name`, `attribution jsonb`, `enabled` | Attribution rules drive the UI |
| `boards` | `id`, `source_id`, `token`, `company_name`, `status`, `etag`, `last_status`, `last_checked_at`, `next_due_at`, `fail_count`, `denylisted` | |
| `postings` | `id`, `source_id`, `board_id`, `source_job_id`, unique `(source_id, source_job_id)`, `cluster_id`, `title`, `company`, `location_raw`, `countries text[]`, `remote_scope`, `level`, `employment_type`, `is_engineering`, `language`, `text` (cleaned, capped at 20 KB), `excerpt`, `content_hash`, `url`, `apply_url`, `posted_at`, `source_updated_at`, `first_seen_at`, `last_seen_at`, `closed_at`, `salary_*`, `skills_required text[]`, `skills_nice text[]`, `tsv tsvector`, `embed_model` | All roles; embeddings live in `search_postings` |
| `clusters` | `id`, `canonical_posting_id`, `minhash bytea` | |
| `skills` | `id`, `name`, `aliases text[]`, `category` | Our curated taxonomy |
| `robots_cache` | `host`, `body`, `status`, `fetched_at` | |
| `search_postings` | `posting_id`, `level`, `remote_scope`, `countries`, `employment_type`, `skills_required`, `embedding halfvec(384)` | Narrow copy of active, in-scope rows, kept in step by the enrich worker |
| `fetch_log` | `host`, `status`, `bytes`, `ms`, `at` | Partitioned by day, 14 days |

Indexes on `search_postings`: B-tree on `(level, remote_scope)`; GIN on `countries` and
`skills_required`; HNSW on `embedding halfvec_cosine_ops (m = 16, ef_construction = 64)`
for broad queries. On `postings`: GIN on `tsv`, for keyword search.

## API surface (v1)

All JSON. Session cookie. Every non-GET route passes the Origin check. IDs not owned by
the caller return 404.

| Method and path | Purpose | Limit |
|---|---|---|
| `GET /api/auth/{google,github}/start` | Begin OAuth (PKCE, state, nonce) | 20/min per IP |
| `GET /api/auth/{google,github}/callback` | Finish OAuth, set session | 20/min per IP |
| `POST /api/auth/logout` | | |
| `GET /api/me` / `DELETE /api/me` | Account; delete everything | |
| `GET /api/me/export` | JSON export | 3/day |
| `POST /api/resumes` | Upload (multipart) -> task | 5/h, 20/day |
| `GET /api/tasks/:id` | Async status | |
| `GET /api/profiles/:id` / `PUT /api/profiles/:id` | Review, correct, confirm | |
| `GET/PUT /api/preferences` | | |
| `GET /api/matches?profile=&filters&cursor=` | Ranked matches with deterministic explanation | 60/min |
| `GET /api/jobs/:key` | Posting detail, attribution | |
| `POST /api/jobs/:key/explain` | LLM explanation (streams) | Ledger + 30/day |
| `POST /api/jobs/:key/tailor` | Tailoring suggestions | Ledger + 10/day |
| `POST /api/jd` | Bring a JD: analyse pasted text (<= 20 KB) | Ledger + 20/day |
| `GET /api/gaps?profile=` | Skill-gap analysis | |
| `GET/POST/PATCH/DELETE /api/tracker[/:id]` | Tracker | |
| `GET/PUT /api/alerts` | Alert settings | |
| `GET /api/unsubscribe?t=` | Signed one-action token, no session | |
| `GET /api/health` | Liveness and readiness | |
| `GET /api/admin/metrics` | Owner only | |

## Stack and versions

Selected under the 14-day rule (cutoff 2026-10-17). "Newer" names a release that was
too young on 2026-10-31.

| Part | Choice | Version | Newer | Why |
|---|---|---|---|---|
| Runtime | Node.js LTS | 24.22.0 (2026-10-14) | 24.22.1, 24.22.2; 26 became LTS on 2026-10-28 | Supported until 2028-04-30 |
| Language | TypeScript | **6.0.3** (2026-04-16) | 7.0.2 fails: see below | Newest 6.x under the 14-day rule |
| API | Express | 5.3.0 | | Same as CodeJudge |
| Validation | Zod | 4.6.7 | | Shared schemas for API and LLM output |
| DB driver | pg | 8.23.1 | | No ORM |
| Rate limits | rate-limiter-flexible | 11.2.2 | 11.2.3 | On Valkey, as in CodeJudge |
| Headers | helmet | 8.3.0 | | |
| Logging | pino | 10.4.0 | | Field allowlist |
| Valkey client (TS) | iovalkey | 0.4.0 | | |
| Magic bytes | file-type | 22.2.0 | 22.3.0 | Plus our own checks |
| PDF text | pdfjs-dist | 6.4.299 | | Inside the sandbox |
| DOCX text | mammoth | 1.13.0 | | Inside the sandbox, after zip checks |
| LLM SDK | @anthropic-ai/sdk (if Q5 = A) | 0.135.0 | 1.3.0 (2026-10-23), a new major | |
| AWS SDK | @aws-sdk/client-* | 3.1152.0 | 3.1161.0 | S3, SSM, SES |
| Web | React / React DOM | 19.3.1 | | |
| Build | Vite | 8.3.4 | 8.3.5 | |
| Routing | react-router | 8.4.0 | 8.5.1 | |
| Data fetching | @tanstack/react-query | 5.104.3 | 5.105.0 | |
| CSS | tailwindcss | 4.3.3 | | |
| Unit tests | Vitest | 5.0.5 | | |
| e2e | @playwright/test, @axe-core/playwright | 1.64.1, 4.14.0 | | |
| API tests | supertest, msw | 7.3.1, 3.0.3 | | |
| Lint | eslint, typescript-eslint, prettier | 10.13.0, 8.71.2, 3.9.10 | 10.14.0, 8.72.1 | |
| Workers | Go | go1.27.2 toolchain in `go.mod` | go1.27.3 (2026-10-19) | Same as CodeJudge |
| PG driver (Go) | jackc/pgx/v5 | v5.11.0 | | |
| Valkey (Go) | valkey-io/valkey-go | v1.0.79 | | |
| Rate (Go) | golang.org/x/time | v0.16.0 | | |
| Metrics (Go) | prometheus/client_golang | v1.25.0 | | |
| ONNX (Go) | yalue/onnxruntime_go | v1.36.0 | v1.38.0 | Runtime: onnxruntime v1.31.0 |
| robots.txt (Go) | temoto/robotstxt | v1.1.2 (2021) | | Old; P2 decides between it and a small RFC 9309 parser of our own, with tests |
| Database | PostgreSQL | 18.6 | | Supported until 2030-11 |
| Vectors | pgvector | 0.8.7 (`pgvector/pgvector:0.8.7-pg18-trixie`, 2026-10-01) | | Iterative index scans, halfvec |
| Queue and cache | Valkey | 9.1.2 (2026-09-01) | 9.1.3 (2026-10-26) | Supported until 2031-05 |
| Migrations | dbmate | v2.36.0 | v2.37.0 (2026-10-18) | |
| Edge | Caddy | v2.11.7 | | |
| Sandbox | gVisor | release-20261012.0 | release-20261026.0 | Same model as CodeJudge |
| Load tests | k6 | v2.3.0 (2026-09-21) | | |
| Backup encryption | age | v1.3.2 | | |
| Secret scanning | gitleaks | v8.30.1 | | |
| Host OS | Ubuntu 26.04 LTS | 26.04.1 | | Supported until 2031-05 |
| Infra | aws-cdk-lib / aws-cdk | 2.273.0 / 2.1145.0 | 2.275.0 / 2.1147.0 | Same as CodeJudge |

Base images are pinned by digest, chosen from tags at least 14 days old. ClamAV is not
used, for the reason given in THREAT_MODEL.md.

**Why TypeScript 6, not 7.** P1 tested TS 7.0.2 and 6.0.3 on 2026-10-31 in one project
with typescript-eslint 8.71.2, Vitest 5.0.5 and `tsc`:

| | `tsc` build | Vitest | ESLint with type-aware rules |
|---|---|---|---|
| TS 7.0.2 | pass | pass | **fails**: "typescript-eslint does not support TS 7.0." |
| TS 6.0.3 | pass | pass | pass |

typescript-eslint declares `typescript >=4.8.4 <6.1.0`, including its newest release
8.72.1, and TS 7's package exports no classic compiler API. Revisit when a
typescript-eslint release older than 14 days supports TS 7.

**Host:** m7g.xlarge (Q9), with x86 t3.xlarge as the fallback if gVisor or any image
fails on arm64 in P1.

## Measured in Phase 0

### Corpus spike (local)

A throwaway spike on 2026-10-31, kept out of this repo. Setup: 5 lakh synthetic rows,
`halfvec(384)`, 120-word Zipf documents, in `pgvector/pgvector:0.8.7-pg18-trixie` on
Docker Desktop (Windows, i5-1235U, container limited to 6 CPUs and 5 GiB,
`shared_buffers` 1.5 GB). **This is a laptop and synthetic data: it sizes the design
and exposes risks. It proves no production latency.**

Solid numbers:

| Measure | Result |
|---|---|
| HNSW build (`m=16, ef_construction=64`, 5 parallel workers) | **403 s** |
| GIN build on `tsvector` | 132 s |
| Sizes | table 1,090 MB, HNSW 558 MB, GIN 135 MB, database 1.8 GB |
| `pg_dump -Fc -Z 1` | 76 s, **1.25 GB** |
| `pg_restore -j 4`, including all index builds | **497 s** |
| Container memory after queries | ~3 GiB |

Latency, from `EXPLAIN ANALYZE` on warm data:

| Query | Rows involved | Samples | Execution time |
|---|---|---|---|
| HNSW, no filter, top 100 | | 3 | ~10 ms |
| HNSW, filter keeping 4.5% of rows, top 100 | 22,667 match | 3 | **129 to 209 ms** |
| Exact vector search over the filtered subset, top 200, one core | 22,667 | 20 | p50 98 ms, p95 259 ms |
| `ts_rank_cd` over the filtered subset, 8-term OR, top 200, one core | ~13,000 match | 20 | p50 402 ms, **p95 3.8 s** |

Filtered HNSW recall@20 against exact search was **0.57 to 0.62**, far below the 0.95
target.

What went wrong, and what can't be trusted:

- A first harness that timed batches through `psql \timing` reported 0.3 to 2 s for
  queries that `EXPLAIN` put at 10 to 200 ms. A warm-cache rerun didn't fix it, and the
  cause wasn't found. Those numbers are discarded. The P3 harness must be validated
  against `EXPLAIN` before its results count.
- The generator's noise was too large, so the vectors were nearly uniform on the
  sphere. That's HNSW's worst case and real embeddings are more clustered, so the
  recall above is pessimistic by an unknown amount.
- Half of the documents matched the 8-term OR, which is a heavy case for ranking.
- The samples are small: 3 and 20 queries.

What the spike changes:

1. **p95 < 200 ms is not proven.** It's the top technical risk in P3.
2. Selective filters are the normal case here, since our users want India or remote
   roles. So filtered search uses **exact search over the small subset**, not HNSW:
   recall is 1.0 by definition, and pgvector's README recommends it for selective
   filters. HNSW serves only broad, unfiltered queries.
3. **`ts_rank_cd` over all matches is too slow.** Lexical retrieval becomes
   skill-array overlap, with `tsvector` kept for typed keyword search only.
4. Search data moves to a **narrow table** (key, filters, skills, embedding), so a
   scan doesn't drag text and `tsvector` through memory.
5. **Matches are computed once per profile version and stored.** Paging and filtering
   read stored results, so the expensive path runs on profile changes and new postings.
6. Memory isn't the limit (1.8 GB for 5 lakh rows). CPU is, so the host choice favours
   vCPUs (Q9).
7. A restore with indexes took about 8 minutes at 5 lakh rows on this laptop. The real
   in-scope corpus is about a quarter of that, so dump-and-restore fits the up budget
   (Q11).

### Sources

Coverage, board yields, 304 behaviour and closed-posting status codes are in SOURCES.md.

## Bench harness and load tests

Claims about speed are measured, with commands anyone can rerun.

- **Synthetic corpus generator** (from the Phase 0 spike, formalised): N postings with
  clustered unit vectors, a Zipf vocabulary, and filter columns whose shares match the
  measured ones (engineering, India, remote-open, level). Seeded, so runs are comparable.
- **`bench` CLI (Go):** runs the real query plans against `db-corpus` with profiles from
  D1, at fixed concurrency, and reports p50/p95/p99, histogram, rows scanned, and
  filtered recall against exact search.
- **k6:** end-to-end HTTP on `/api/matches` with real sessions, 20 virtual users, while
  `ingest` writes at full rate. Pass: p95 < 200 ms, error rate < 0.1%.
- **Fake source (Go):** thousands of boards with ETags, 304s, 429s with `Retry-After`,
  slow and huge responses. It proves the limiter never exceeds the configured rate,
  however many workers run.
- Each run writes `bench/reports/<date>-<sha>.md` with hardware, versions and config.

## Deployment design (built in P8)

Follows CodeJudge: AWS CDK, ap-south-1, GitHub OIDC, images by digest, SSM, deploy with
smoke test and automatic rollback.

**Two stacks:**

| Stack | Contents | Lifetime |
|---|---|---|
| `RoleRankProd` (persistent) | S3 backup bucket (versioned, Object Lock on `backups/`, lifecycle), ECR repositories keeping 2 images each, SSM parameters, SNS topic and `BackupMissing` alarm, OIDC deploy role, SES domain identity, monthly budget | Always |
| `RoleRankHost` (ephemeral) | One EC2 host (Q9) with a gp3 volume, security group, instance role, public IP | Created by up, destroyed by down |

The host runs Caddy, `api`, `aiworker`, Postgres and Valkey under Docker
Compose, and `ingest` and `extractor` under systemd (gVisor needs the host runtime).

**`rolerank-down.sh`:**
1. Final `pg_dump` of `db-app`, encrypted with `age`, then a restore drill into a scratch
   database comparing every table's row count. Any failure stops here; nothing is removed.
2. `pg_dump` of `db-corpus` without indexes (Q11), and the Caddy data.
3. Upload to S3 with checksums; keep the newest 2 finals.
4. Disable the `BackupMissing` alarm's actions; park DNS at 192.0.2.1.
5. Save the release manifest; destroy `RoleRankHost`.
6. Audit every billable resource in every Region; print `STILL BILLING` and exit 1 on
   anything beyond ECR and S3 within their documented size.

**`rolerank-up.sh`:**
1. Preflight: CLIs, SSO login, the right account, budget present.
2. Deploy `RoleRankHost`; wait for bootstrap.
3. Restore Caddy data, then deploy the saved release by digest.
4. Restore `db-app`, apply tombstones, check counts. Restore `db-corpus`, build indexes.
5. Point DNS at the host; start the catch-up ingestion cycle.
6. Checks: `/api/health` by IP and by domain; Google and GitHub authorize pages name the
   app; one real LLM call; one embedding; one SES send to the owner; a real
   `/api/matches` request under 200 ms.
7. Box: URL, fallback hosts-file line, cost per hour, "run rolerank-down.sh when done".

**Backups:** nightly while up, plus the final one at down. The host role can only
`s3:PutObject` under its prefix; Object Lock blocks deletion within retention. Freshness
alarm after 26 hours without a success while up. All timers started with `enable --now`
and verified active by the deploy.

## Cost (ap-south-1, on-demand, USD; Rs at 94.95)

Prices from the AWS public price files on 2026-10-31: m7g.xlarge $0.1166/h, gp3
$0.0912/GB-month, S3 Standard $0.025/GB-month, EBS snapshots $0.05/GB-month. Public IPv4
$0.005/h is from CodeJudge's measured bill, not re-checked. ECR storage at ~$0.10/GB-month
is inferred from CodeJudge's measured cost, not from a price file.

| State | Cost | Detail |
|---|---|---|
| Up, per hour | **~$0.127 (~Rs 12)** | m7g.xlarge $0.1166 + 40 GB gp3 $0.0050 + IPv4 $0.005; plus LLM use, capped at $1/day |
| Up for a month (down forgotten) | **~$93 (~Rs 8,800)** | The Rs 500 budget's 50% email arrives within about a day |
| Down, per month | **~$0.30 (~Rs 28)**, to confirm in P8 | ECR ~2 GB ~$0.20; S3 finals (2 dumps of ~1.25 GB at most) ~$0.06; SSM, SNS, alarm, budget ~$0 |

Per-user LLM cost: ~$0.001 per parse, explanation or tailoring run with the Q5
recommendation; bounded by the $0.05 per user per day cap.
