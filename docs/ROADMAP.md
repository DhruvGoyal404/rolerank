# Roadmap

Each phase ends with something you can demo and numbers you can rerun. A phase is
done when every exit criterion is met and written up in `docs/PHASE<n>.md`: what
was built, how it was measured, what surprised us, what's left. Moving a criterion
takes a written reason.

## Schedule

**Lean v1 (owner's decision, 2026-11-04).** v1 is P3, P4, P5 (with Bring a JD), a minimal
P6 and P8, **deployed by 2026-11-22**. P7's email alerts and everything listed under "v1.1"
below wait until after v1. Every dataset is model-labelled with a small human audit; the
owner's time goes only to audits. Dates are ends of phases.

| Phase | Done by | Owner's time needed |
|---|---|---|
| P1 Foundation and CI | 2026-11-02 (done) | |
| P2 Ingestion | 2026-11-04 (closed with one known gap) | Two final audits, 30 items |
| P3 Search and ranking | 2026-11-08 | Audit of the model-graded D3 sample; pick the embedding model |
| P4 Resume parsing | 2026-11-11 | Review the hostile-file results |
| P5 Bring a JD, explanations, tailoring | 2026-11-14 | Audit a sample of tailoring suggestions |
| P6 Minimal product UI | 2026-11-18 | One walkthrough |
| P8 Deploy, up and down | 2026-11-22 | AWS SSO login, budget and alarm emails |

Risks to these dates:

- P1 to P5 in 10 days is aggressive even at this pace, and the phases are sequential.
  A gate is never skipped to hold a date. If a date slips, it slips, and the report says
  so.
- The eval gates need the owner's hands only for audits now, not full labelling passes.
- Providers may not reply to the outreach emails in time. P2 proceeds at 1 request per
  second per host regardless.
- The first full ingestion cycle runs at polite rates, so it can't be rushed. Common
  Crawl's index server was also unreliable in Phase 0.
- **Quality comes before these dates.** If a date slips, it moves, with the reason written
  here. A gate is never skipped and the request rate is never raised to hold one.
- A new dependency release needs 14 days before it can be adopted, so a version bug
  found now can't always be fixed by upgrading.

Every phase also has these standing criteria:

- CI green: typecheck, lint, unit, integration, e2e, build, gitleaks, `npm audit`,
  `govulncheck`.
- One commit per logical piece, each passing the gate.
- Dependencies pass the 14-day rule (a CI script checks publish dates).
- The doc that owns a decision is updated in the same commit.

## P0: research and design (done 2026-10-31)

The eight docs exist, every claim has a source or says "unknown", and the owner has
answered QUESTIONS.md.

## P1: foundation and CI

Monorepo: `api/`, `web/`, `worker/` (Go), `db/`, `evals/`, `infra/`. Compose with one
Postgres cluster (databases `app` and `corpus`, separate roles) and Valkey. Express API
with config validation, health, structured logs. Google and GitHub sign-in, sessions,
invite codes, 18+ confirmation, Origin check, helmet CSP, rate limits. Empty React shell.
dbmate on both databases. CI from the first commit.

**Exit criteria**

- **TypeScript 7.0.2 was tested:** typescript-eslint doesn't support it, so TS 6.0.3 is
  pinned and the reason is recorded in ARCHITECTURE.md. Done 2026-10-31.
- **arm64 is proven:** every image runs under gVisor on arm64 and the integration suite
  passes there (Q9), or the host falls back to x86 t3.xlarge. Done 2026-10-31.
- **Database isolation is proven:** `app_rw` is refused by `corpus`, the corpus roles are
  refused by `app`, `PUBLIC` has no `CONNECT`, and a schema test finds no PII column in
  `corpus`.
- Sign in with both providers locally. Sign out and session expiry work.
- Negative auth tests pass: bad `state`, reused `state`, bad `nonce`, wrong `aud`,
  expired token, `alg=none`, unsigned token, foreign `next`, missing Origin.
- A test starts the real server with `TEST_AUTH_SHORTCUT=1` and `NODE_ENV=production`
  and proves it refuses to run.
- e2e refuses a database not ending in `_test`.
- Log allowlist test: a run with planted PII leaves none in the logs.
- `set -o pipefail` lint on workflow files. Actions pinned by SHA.
- The dependency-age check fails on a too-new package (proved with a fixture).

**Demo:** sign in on localhost; CI badge; a failed-on-purpose security test.

## P2: ingestion

Go workers: scheduler, fetcher, normaliser, deduper. Source adapters for Greenhouse,
Lever, Himalayas and RemoteOK, each behind a contract test recorded from the
real API. robots.txt handling, shared per-host limiter, ETag cache, closed-posting
detection, board registry, discovery tool, source health metrics, /sources and
/about/bot pages.

**Exit criteria**

- At least 1.5 lakh active postings ingested from real sources, on the local machine
  or a temporary worker. The count is reported per source. Met without Ashby: 186,131
  canonical postings on 2026-11-02.
- A full refresh cycle finishes inside 6 h at the configured rates. 304 rate reported.
- **Never above the configured rate:** a load test against a local fake source with 4
  workers proves the limiter holds across workers.
- A fake source returning 429 with `Retry-After`, 5xx, huge bodies, gzip bombs, or
  slow responses is handled correctly in tests.
- Kill a worker mid-batch: nothing is lost or duplicated (reclaim test).
- Closed postings leave the active set within one cycle: measured by comparing against
  a fresh full fetch, with error below 1%.
- Dedupe eval D5 passes. Classifier eval D6 passes.
- The bot obeys a local robots.txt that disallows it (test).
- Attribution data stored for every posting (test per source).
- **Bytes per cycle reported**, split into 200 and 304 responses, to confirm unchanged
  boards cost almost nothing with Greenhouse's `content=true`.
- **The first full run is resumable:** it can be stopped and restarted at any point with
  no duplicates and no lost progress, proved by killing it mid-run. The owner gets its
  expected duration before it starts.
- **In-scope coverage report:** how many India, remote-India, intern and entry-level
  engineering roles we really have. This is the number that matters most.
- **Per-source share of in-scope roles** in the P2 report, recorded in `coverage_snapshots`
  and shown on the admin metrics page in P6, so reliance on one source is visible over time.

**Exit status (2026-11-04): closed by the owner with one known v1 gap,** entry-level F1 (0.70
against 0.90). Dedupe passed on the last fresh round. The thresholds weren't changed. See
PHASE2.md, "Close report".

**Coverage after Ashby's exclusion (2026-11-02):** 37,538 in scope, 3,761 India engineering,
96 India early-career, 27 India interns, 2,235 remote-open. Greenhouse is 85% of in scope;
Himalayas is 69% of India early-career and 88% of remote-open. SPEC's targets became
1.5 lakh postings and about 21,000 in scope on 2026-11-04, with the reasons in SPEC.md.

**Demo:** dashboard of postings per source, lag, 304 rate, a live worker kill.

## P3: search, ranking and evals

Embedding pipeline; HNSW and full-text indexes on the corpus cluster; hybrid retrieval;
feature reranker; fit bands. Eval runner; datasets D2 and D3; embedding bake-off. Bench
harness with the 5 lakh synthetic corpus.

**Exit criteria**

- Bake-off report across the candidates in ARCHITECTURE.md, with D3 results, cost and
  latency, and a recommendation. The owner picks.
- D2 and D3 thresholds pass. Hybrid beats lexical-only and vector-only with a 95%
  bootstrap interval above zero, or the losing part is removed.
- **p95 search and rank < 200 ms** on the 5 lakh bench corpus at the target host size,
  with 20 concurrent users, while ingestion writes run. Reported as a full latency
  histogram, not one number.
- The bench harness's timings are checked against `EXPLAIN ANALYZE` first. The Phase 0
  harness overstated latency by up to 100x for a cause never found.
- Filtered recall@20 of at least 0.95 against exact search, on the real filter mix.
- Rebuild time for all indexes is measured and fits the up/down design.
- The rank of a given (profile, corpus) is reproducible from stored features.

- Run the bench with `ingest` on and off. If `/api/matches` p95 rises by more than 25% and
  misses 200 ms because of it, split `app` and `corpus` into two clusters, with numbers.

**Demo:** type a profile, get ranked jobs with matched and missing skills, then show the
bench report and eval report side by side.

## P4: resume parsing

Upload flow; magic-byte checks; sandboxed extraction under gVisor; contact stripping;
LLM parse with schema; validator; grounding check; profile confirmation screen;
encrypted storage; LLM ledger and caps. Datasets D1 and D7.

**Exit criteria**

- D1 passes all field thresholds, per segment. Zero fabricated values.
- D7 passes with zero canary output.
- Hostile file suite: zip bomb, XML bomb, PDF with deep nesting, polyglot, huge page
  count, password-protected, scanned. Each is rejected or contained, with no effect
  on the host, proved by a test that runs them.
- The sandbox has no network and no write access (isolation tests, as in CodeJudge P0).
- Encrypted at rest: a test reads the DB and object store and finds no plaintext.
- Spend cap: a test with a fake provider proves the per-user and global caps stop calls,
  including under concurrent requests.
- Delete account: the row-level test proves nothing with that `user_id` remains.

**Demo:** upload a synthetic resume, see the parse, correct a field, watch a zip bomb fail.

## P4b: Bring a JD (first user-facing milestone, done inside P5 for v1)

The first thing a real person can use. Paste a job description, get the same breakdown
as an indexed job: matched skills, missing skills, level fit, and tailoring with the
grounding gate. Includes a minimal real UI: upload, confirm profile, paste JD, results.
Dataset D4.

**Exit criteria**

- All tailoring thresholds pass. Zero invented skills on pressure cases.
- A test proves that a suggestion failing the gate can't reach the API response.
- Pasted JD text is treated as untrusted (D7 cases included) and never joins the shared
  index.
- Human audit of 50 suggestions recorded in the phase report.
- Mean cost per tailoring run reported, within budget.
- e2e of the whole flow on an isolated database, axe-checked.

**Demo:** upload a synthetic resume, paste a JD demanding Kubernetes, get matched and
missing skills and grounded tailoring, with RoleRank saying honestly that the resume
doesn't show Kubernetes.

## P5: explanations and gaps

Deterministic explanations on indexed jobs and on-demand LLM prose, with P4b's Bring a JD and
grounded tailoring. The skill-gap page is v1.1; the per-job matched and missing skills stay.

**Exit criteria**

- All explanation thresholds in EVALS.md pass.
- The gate rejects any skill in LLM prose that isn't in the deterministic lists (test).
- Mean cost per explanation reported, within budget.
- Explanation p95 <= 3 s.

**Demo:** a resume's ranked matches, each explained, and a pasted JD with grounded tailoring.

## P6: product UI (minimal for v1)

Sign in, upload and confirm the profile, matches, job detail, Bring a JD, tailoring, a simple
tracker (save a job, set its status), /privacy and /sources. Design pass and a real
accessibility pass. The gap view, tracker notes, multiple profiles and extra dashboards are
v1.1.

**Exit criteria**

- Playwright e2e across every M story, on an isolated DB.
- axe clean at WCAG 2.1 AA on every page, light and dark.
- Usable on a 360 px phone.
- Largest Contentful Paint under 2.5 s on throttled 4G (measured with Lighthouse in CI).
- Strict CSP with no violations in e2e.
- Attribution visible per source; checked by a test against each source's rule.
- /privacy reviewed line by line against the code.

**Demo:** an end-to-end walkthrough by someone who hasn't seen it.

## P7: alerts and tracker (v1.1; the simple tracker moved into P6)

Tracker states and persistence past posting closure. Email digests through SES:
opt-in, unsubscribe, bounces, complaints. Sent only while the site is up (Q1). GitHub users
grant `user:email` incrementally, only when they turn alerts on; sign-in stays `read:user`.

**Exit criteria**

- Tracker e2e, including a saved job that later closes.
- Alert job finds new strong matches in under 60 s for 1,000 test users.
- Unsubscribe works without a session; a suppressed address gets nothing.
- SPF, DKIM, DMARC pass on a real test mail.
- Honest report: has the owner actually used this for applications, and what failed?

**Demo:** a new posting arrives, and a real email goes to a real inbox.

## P8: deploy, up and down

CDK: persistent stack and host stack. GitHub OIDC, deploy by digest, SSM, auto-rollback.
`rolerank-up.sh`, `rolerank-down.sh`. Backups. Budget alert. Runbook in MAINTENANCE.md.

**Exit criteria**

- up from fully down in under 15 minutes, measured with a breakdown of each step.
- down leaves the audit clean, exit 0.
- A real cycle passes: up, real request, down (audit clean), up. Then left down.
- up ends with the box: URL, fallback, cost per hour, "run rolerank-down.sh when done".
- up makes a real call to each critical external API: LLM, embedding, email.
- Backup restore drill passes on every down: row counts equal, or nothing is removed.
- Backup freshness alarm fires in a drill, and the email actually arrives. Evidence
  recorded. All timers checked active, not just enabled.
- Deletion across restore: delete a test user, restore an older backup, prove the
  tombstone removes them before the app starts.
- Budget alert live at the Rs 500 equivalent in the billing currency, notifications
  at 50/80/100% and forecast, confirmed. Stated as an alert, not a cap.
- Cost table in MAINTENANCE.md from the Pricing API, plus the first real bill.
- Load test against the deployed host: p95 < 200 ms holds on production hardware.
- Focused security review of auth, upload and deletion written to
  `docs/SECURITY_REVIEW.md`, findings fixed or accepted in writing.

**Demo:** the full cycle, timed, from a terminal.

## v1.1 (after v1 is deployed)

- Email alerts: P7 as written above.
- Skill-gap page, multiple profiles, tracker notes, extra dashboards.
- Entry-level recall: v1 misses F1 0.90 (PHASE2.md, "Known v1 gaps").

## Later (not committed)

- Scheduled ingestion window, so the corpus and alerts stay fresh while the site is
  down (Q1, option B).
- First-party career pages (Q4), if P2's coverage report shows the gap remains.
- More ATS sources: Recruitee, Teamtailor, Workable (Q7).
- Ashby's Dedicated Partner Job Feed: employer opt-in, needs an application.
- Rerankers, if evals show room above the deterministic ranker.
- Hindi or other language postings.
