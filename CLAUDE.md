# RoleRank

Ranks real, legally sourced job postings against a resume and explains every
match. Focus: software and engineering roles in India and remote roles open to
India, with freshers and interns as first-class users.

This file is the project's constitution. It binds every change, human or AI.
To weaken a rule, change this file in its own PR and say why.

**Status: Phase 0 (research and design docs). There is no application code yet.**
Read these before changing anything:

| Doc | What it holds |
|---|---|
| [docs/SPEC.md](docs/SPEC.md) | Users, stories, scope, non-goals |
| [docs/SOURCES.md](docs/SOURCES.md) | Every job source: endpoints, terms, limits, measured coverage |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Components, data model, pipelines, stack, up/down design |
| [docs/EVALS.md](docs/EVALS.md) | Datasets, metrics, pass thresholds |
| [docs/THREAT_MODEL.md](docs/THREAT_MODEL.md) | STRIDE per component, mitigations, residual risks |
| [docs/ROADMAP.md](docs/ROADMAP.md) | Phases and exit criteria |
| [docs/QUESTIONS.md](docs/QUESTIONS.md) | Decisions that belong to the owner |

## How we work

- **Verify, never guess.** Every version, API and claim is checked against an
  official doc, a registry or a live call. If it can't be checked, write
  "unknown". A doc is a claim until behaviour confirms it.
- **Evidence in reports.** A report states what ran, the command, the numbers
  and what failed. "Should work" is not a result. "Measured" means there is a
  command someone else can rerun.
- **Small PRs, one commit per logical piece.** Every piece passes the full gate
  before the next one starts: typecheck, lint, unit, integration, e2e, build.
- **A red gate stops the line.** No skipping hooks, no `--no-verify`, no
  disabling a test to get green. Fix the cause.
- **Each phase ends with something demoable and measured.** The exit criteria in
  ROADMAP.md are binding. Moving a threshold needs a written reason.
- **Upstream bugs:** write a minimal repro, report it, link it in the code.
- **Record incidents.** Anything that broke production or a backup goes into
  MAINTENANCE.md with cause and fix, as CodeJudge does.
- **Say what's unfinished.** Open risks stay visible in the doc that owns them.

## Dependencies

- **14-day rule.** Adopt only a release published at least 14 days ago. If the
  newest is younger, take the newest that is older, even if that crosses a major
  version. Check with `npm view <pkg> time --json` or the Go module proxy.
- **Exact pins.** No `^` or `~`. Lockfiles are committed. `npm ci` only in CI.
- **`npm audit` clean** and `govulncheck` clean on every PR.
- **Security exception:** a fix for a published advisory may be younger than 14
  days. Record it in `docs/DEPENDENCY_EXCEPTIONS.md` with the advisory ID, the
  date and an expiry date. Remove the entry when the rule is met again.
- **Same spirit elsewhere:** Docker base images pinned by digest and at least 14
  days old; GitHub Actions pinned by commit SHA; Go toolchain pinned in `go.mod`;
  embedding models pinned by revision hash.
- Add a dependency only if the alternative is clearly worse. Fewer is better.
- Dependabot PRs are never merged directly; recreate them as our own PR, as in
  CodeJudge's MAINTENANCE.md.

## Testing and CI

- Piped CI steps use `set -o pipefail`, so a failing command can't hide behind
  `tee` or `grep`.
- e2e tests run on an isolated database created for the run. They refuse to start
  against a database whose name doesn't end in `_test`. Never the dev DB.
- Dev-only switches (for example `TEST_AUTH_SHORTCUT`) exist only with
  `NODE_ENV=test`. The server refuses to start otherwise, and a test starts the
  real process to prove it.
- Security behaviour is tested by running it, not by reading it: the Origin
  check, rate limits, ownership checks and file rejection all have tests that
  send the hostile request.
- gitleaks runs in CI on the full history. CodeQL and Dependabot alerts stay on.
- Evals are part of the gate: see docs/EVALS.md for which run on which trigger.

## Database

- dbmate migrations, forward-only. Expand first, contract in a later release.
  Every migration works with the previous release too.
- No ORM. SQL is reviewed like code. Queries are parameterised.
- Two Postgres clusters, kept apart on purpose: `app` holds user data, `corpus`
  holds postings. PII never enters `corpus` (ARCHITECTURE.md).

## Source and ingestion invariants (never weaken without explicit approval)

- Only sources listed as approved in docs/SOURCES.md. A new source needs a
  SOURCES.md entry first: endpoint, terms link, robots result, attribution, tests.
- Never scrape LinkedIn, Naukri, Indeed, Internshala or any site whose terms or
  robots.txt forbid it. No stealth browsers, no proxy rotation, no CAPTCHA
  solving, no faked User-Agent.
- Every request carries `RoleRankBot/<version> (+https://<site>/about/bot; <contact email>)`.
- robots.txt is fetched, cached for 24 hours and obeyed per host, including
  `Crawl-delay`. A 5xx on robots.txt means stop for that host; a 4xx means no rules
  apply (RFC 9309).
- Per-host rate limits are enforced in one shared limiter, so adding workers can
  never raise the rate. On 429 or 503, honour `Retry-After` and back off.
- Conditional GET (`If-None-Match`) on every board that sends an ETag.
- Follow each source's attribution rule in the UI. Apply links go exactly where
  that source requires.
- We show an excerpt and a link, not a copy of the posting. Job pages are
  `noindex` and emit no `JobPosting` structured data.
- Ingested postings are never used to train or fine-tune any model.
- Any employer can ask to be removed. The board goes on a denylist within 7 days.

## AI invariants (never weaken without explicit approval)

- Resumes and job descriptions are **untrusted input**. They never go into a
  system prompt. They are delimited as data, and the model is told so.
- LLM calls have **no tools and no side effects**. Output is schema-constrained,
  then validated with Zod on our side. Invalid output is rejected, not repaired
  by hand.
- **Grounding gate:** a tailoring suggestion must cite a resume line that exists,
  and every number, employer, title, date and technology in it must appear in that
  line or in the user's confirmed profile. A suggestion that fails is dropped
  and counted. Tailoring never invents experience.
- The LLM never ranks the whole corpus. Ranking is deterministic and reproducible
  from stored features. The score shown is a fit band, not a percentage.
- Every match has an explanation without any LLM: matched and missing skills come
  from the taxonomy. LLM prose is an on-demand addition.
- **Cost caps are enforced in code**, per user and globally, in USD per UTC day,
  against a ledger written before the call (reserve) and settled after it. At the
  cap the feature degrades to the deterministic version. A cap is never bypassed.
- Contact details are stripped before any text reaches an LLM.
- Only providers and modes marked approved in ARCHITECTURE.md may receive resume
  text. Zero-retention where the provider offers it. Prompts and resume text are
  never logged.

## Auth invariants (never weaken without explicit approval)

- Hand-written OAuth 2.0 with no auth library: Google and GitHub.
- PKCE S256 and `state` on both; `nonce` on Google, checked in the ID token.
  `state` is random, held in a short-lived httpOnly cookie, compared in constant
  time, and single-use (Valkey GETDEL).
- Google ID token: `alg` allowlisted to RS256, signature checked against JWKS,
  `iss`, `aud`, `exp` and `nonce` verified. Users are keyed on `(provider, sub)`,
  never on email. Accounts are not auto-linked by email.
- No JWT sessions. The cookie holds a random 32-byte token; the DB stores only its
  SHA-256. Cookie is httpOnly, SameSite=Lax, Secure, `__Host-` in production.
- Provider access tokens are used once to read the profile, then discarded.
- Every non-GET/HEAD/OPTIONS route passes the Origin check against `PUBLIC_ORIGIN`.
  Post-login `next` accepts same-origin paths only.
- Every row of user data is read through a query scoped by `user_id`. A missing or
  foreign ID returns 404, never 403.

## Privacy and resume invariants (never weaken without explicit approval)

- Upload: size limit, magic-byte check, then parsing only inside a no-network,
  read-only, resource-limited sandbox under gVisor (the CodeJudge sandbox pattern).
- Resume text is encrypted at the application layer with a per-user key before it
  reaches a disk. Plaintext exists only in process memory.
- Resume content, profile fields, prompts and model output are never logged, never
  in error reports, never in metrics labels.
- User-initiated deletion works, is tested end to end, and takes effect
  immediately in live systems and within the window stated on /privacy in
  backups. Restores re-apply the deletion tombstones.
- /privacy says what actually happens. If behaviour changes, it changes in the
  same PR.

## Web baseline

- helmet with a strict CSP: `script-src 'self'`, Trusted Types, HSTS.
- Rate limits with rate-limiter-flexible on the existing datastore; 429 with
  `Retry-After`. No new infrastructure for limits.
- Job text and all model output render as text, never as HTML.
- Accessibility: axe-checked WCAG 2.1 AA, usable on a phone.

## Secrets

- Never print a secret. Never put one in chat, logs, commit messages, screenshots
  or CI output. Scripts read them straight from the cloud secret store.
- Locally, secrets live in gitignored `.env` files. `.env.example` holds names only.
- GitHub reaches AWS through OIDC only. No stored AWS keys anywhere.

## Deployment (normally DOWN)

The site runs only when needed. Standing rules:

- `rolerank-up.sh`: fully down to live, target under 15 minutes. `rolerank-down.sh`
  stops every recurring cost and ends with an audit that prints `STILL BILLING` and
  exits non-zero if anything bills beyond the documented allowance.
- Both: Bash for Git Bash on Windows, `set -euo pipefail`, `MSYS_NO_PATHCONV=1`,
  idempotent, preflight, retries on transient errors, no local Docker, images
  built in the cloud from `git archive HEAD`.
- Windows tools from Git Bash take Windows-form paths (`E:/...`), never `/tmp`,
  `/e/...` or `/dev/null`. No `curl | grep -q` under pipefail. `PYTHONUTF8=1` for
  aws.exe. These are CodeJudge's real bugs; don't repeat them.
- up ends with a box: URL, fallback URL, cost per hour, "run rolerank-down.sh when done".
- Always leave the site down after testing.
- Monthly budget alert of about Rs 500, checked against the billing currency,
  emails at 50/80/100% plus forecast. It is an alert, not a cap.
- A cost table and a real test of up, real request, down (audit clean), up before
  deployment is called done.
- Backups from day one: create-only credentials, encrypted, restore drill on every
  down, freshness alarm, and alert delivery proven end to end. Every timer is
  started with `enable --now` and checked, because CodeJudge once ran two days
  without a backup when one wasn't.

## Environment

Windows 11, Docker Desktop (shared with other work: never change its settings, and
never touch containers, images or volumes this project didn't create; ours carry the
label `rolerank.managed=true`). System Go is 1.22.5; modules pin their toolchain in
`go.mod`. Browser origin in dev is `http://localhost:5173`, never 127.0.0.1.
