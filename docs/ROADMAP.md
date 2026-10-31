# Roadmap

Each phase ends with something you can demo and numbers you can rerun. A phase is
done when every exit criterion is met and written up in `docs/PHASE<n>.md`: what
was built, how it was measured, what surprised us, what's left. Moving a criterion
takes a written reason. Time estimates are unknown until Q12 is answered.

Every phase also has these standing criteria:

- CI green: typecheck, lint, unit, integration, e2e, build, gitleaks, `npm audit`,
  `govulncheck`.
- One commit per logical piece, each passing the gate.
- Dependencies pass the 14-day rule (a CI script checks publish dates).
- The doc that owns a decision is updated in the same PR.

## P0: research and design (this phase)

**Done when:** the eight docs exist, every claim has a source or says "unknown",
and the owner has answered QUESTIONS.md.

## P1: foundation and CI

Monorepo: `api/`, `web/`, `worker/` (Go), `db/`, `evals/`, `infra/`. Compose with two
Postgres clusters and Valkey. Express API with config validation, health, structured
logs. Google and GitHub sign-in, sessions, Origin check, helmet CSP, rate limits. Empty
React shell. dbmate on both clusters. CI from the first commit.

**Exit criteria**

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
Lever, Ashby, Himalayas and RemoteOK, each behind a contract test recorded from the
real API. robots.txt handling, shared per-host limiter, ETag cache, closed-posting
detection, board registry, discovery tool, source health metrics, /sources and
/about/bot pages.

**Exit criteria**

- At least 1.5 lakh active postings ingested from real sources, on the local machine
  or a temporary worker. The count is reported per source.
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
- **In-scope coverage report:** how many India, remote-India, intern and entry-level
  engineering roles we really have. This is the number that matters most.

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

## P5: explanations, gaps and tailoring

Deterministic explanation; on-demand LLM prose; skill-gap analysis; Bring a JD;
tailoring with the grounding gate. Dataset D4.

**Exit criteria**

- All tailoring thresholds pass. Zero invented skills on pressure cases.
- A test proves that a suggestion failing the gate can't reach the API response.
- Human audit of 50 suggestions recorded in the phase report.
- Mean cost per explanation and per tailoring run reported, within budget.
- Explanation p95 <= 3 s.

**Demo:** a JD demanding Kubernetes, a resume without it, and RoleRank saying so honestly.

## P6: product UI

The full React app: onboarding, matches, filters, job detail, tracker view, gap view,
tailoring view, settings, /privacy. Design pass and a real accessibility pass.

**Exit criteria**

- Playwright e2e across every M story, on an isolated DB.
- axe clean at WCAG 2.1 AA on every page, light and dark.
- Usable on a 360 px phone.
- Largest Contentful Paint under 2.5 s on throttled 4G (measured with Lighthouse in CI).
- Strict CSP with no violations in e2e.
- Attribution visible per source; checked by a test against each source's rule.
- /privacy reviewed line by line against the code.

**Demo:** an end-to-end walkthrough by someone who hasn't seen it.

## P7: alerts and tracker

Tracker states and persistence past posting closure. Email digests through SES:
opt-in, unsubscribe, bounces, complaints. Depends on Q1.

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

## Later (not committed)

- Scheduled ingestion window, so the corpus and alerts stay fresh while the site is
  down (Q1, option B).
- First-party career pages (Q4), if P2's coverage report shows the gap remains.
- More ATS sources: Recruitee, Teamtailor, Workable (Q7).
- Rerankers, if evals show room above the deterministic ranker.
- Hindi or other language postings.
