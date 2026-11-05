# Decisions for the owner

**All answered on 2026-10-31.** This file keeps the reasoning behind each decision.

| # | Decision |
|---|---|
| Q1 | A: on-demand only |
| Q2 | A: invite-only until the security review |
| Q3 | A: `rolerank.dhruvgoyal.tech`. Owner accepts the trademark risk for a student project |
| Q4 | A: small reviewed allowlist, decided after P2 |
| Q5 | A: Claude Haiku 5.5 via the Anthropic API |
| Q6 | A: local bge-small by default, confirmed by bake-off in P3 |
| Q7 | A: not in v1 |
| Q8 | A: $5.27 monthly alert; $0.05 per user and $1.00 global per day |
| Q9 | A: m7g.xlarge. Confirmed in P1: the compose images run under gVisor on arm64 and 45 integration tests pass there (PHASE1.md). D (x86 t3.xlarge) stays the fallback if a later image fails |
| Q10 | A: delete the original after extraction |
| Q11 | A: `pg_dump` to S3 |
| Q12 | 140 hours a week. Since 2026-11-04: lean v1 (P3, P4, P5, a minimal P6, P8) deployed by 2026-11-22; alerts and extras in v1.1 (ROADMAP.md) |
| Q13 | A: public. Repository created 2026-10-31 |
| Q14 | A: 18+ |
| Q15 | No decision needed: the account is already on the PAID plan (`codejudge-down.sh` printed "plan: PAID ACTIVE" on 2026-10-30). Same account, separated by tags and stack names. Both budgets move to the current rate in P8 |
| Q16 | A: `bot@dhruvgoyal.tech` through Cloudflare Email Routing; drafts in OUTREACH.md |
| Q17 | No zero-data-retention (ZDR) agreement with Anthropic for v1, which is invite-only. **Revisit ZDR before public signup** (2026-11-05) |

Later changes by the owner (2026-10-31): one Postgres cluster with two databases instead
of two clusters; D3, D5 and D6 reduced; Bring a JD moved to P4b. The docs reflect these.

Prices: USD, checked 2026-10-30/31; Rs at the ECB rate of 94.95 per USD (2026-10-30).

---

## Q1. What happens while the site is down?

Your rule is that a deployed project runs only on demand. Three features don't fit
that cleanly: email alerts (nothing to send while down), freshness (the corpus goes
stale), and account deletion (a user can't press the button while the site is off).

- **A. On-demand only (recommended).** Alerts are sent only while up, and the
  settings page says so. On up, a catch-up refresh runs first and the UI shows the
  corpus age until it ends. Deletion while down is by email to the contact address,
  done within 7 days, and /privacy says this. Cost while down stays near zero.
- B. Daily ingestion and alert window: a scheduled host runs for about an hour a day,
  refreshes, sends alerts, processes deletion requests, and stops. Roughly $2 to $4 a
  month. Alerts become real. More moving parts in up/down.
- C. Always on, small. A t3.medium (too small for this corpus, by the way) 24x7 is
  over $33 a month before storage: about seven times the budget.

Why A: it honours your standing rule and costs nothing while idle. B is in the roadmap
under "Later", once A works.

## Q2. Who can sign up?

- **A. Allowlist at first (recommended).** The system is built multi-user, but signups
  need an invite code until P8's security review passes. Friends, TIET classmates,
  interview panels.
- B. Open public signup from the first deploy.

Why A: you'd hold real people's resumes, and the DPDP duties (THREAT_MODEL.md) bind
whoever holds them. A small known group limits the harm of an early bug.

## Q3. Domain and name

- **A. `rolerank.dhruvgoyal.tech` (recommended).** Consistent with CodeJudge, free,
  DNS already through Cloudflare.
- B. Buy a domain. Checked by DNS on 2026-10-30: `rolerank.com` resolves to a parked
  page (taken); `rolerank.ai`, `.io`, `.in`, `.dev`, `.app` returned NXDOMAIN, which
  suggests but doesn't prove they're unregistered.

On the name itself: npm `rolerank` is free, a web search found no product called
RoleRank, and GitHub has 4 small repos with the name. **A trademark search was not
done.** Please search the IP India registry (and USPTO if you care) before launch.

## Q4. First-party career pages

The legal ATS APIs hold very few Indian fresher roles (SOURCES.md, "Measured coverage").
Many Indian employers run their own career sites.

- **A. Yes, later, by allowlist (recommended).** Hand-reviewed list of at most 50 sites,
  only where robots.txt allows us and the terms don't ban automated access, reading
  only their published `JobPosting` structured data or an official feed. Decided after
  P2's coverage report.
- B. No. ATS APIs, Himalayas and RemoteOK only, plus Bring a JD.
- C. Yes, broadly, from P2.

Why A: it's the only legal route to many Indian employers, but each site needs a check,
so it must stay small and reviewed.

## Q5. LLM provider for parsing, explanations and tailoring

Expected use per call: parse ~3k tokens in, ~1.5k out; explanation ~2k in, ~300 out;
tailoring ~4k in, ~1.5k out.

| Option | Price per 1M in / out | Cost per parse | Structured output | Retention |
|---|---|---|---|---|
| **A. Claude Haiku 5.5, Anthropic API (recommended)** | $0.10 / $0.50 | ~$0.001 | JSON schema (`output_config.format`), verified in docs | Prompts and outputs not retained by default for non-"covered" models; not used for training without permission; flagged content may be kept up to 2 years |
| B. Amazon Bedrock | Bedrock prices not verified | unknown | Model-dependent | Bedrock: model providers have no access to prompts; ZDR mode available. Haiku 5.5 from Mumbai is **global cross-Region only** (data may leave India); the India-only profile offers Claude Haiku 4.5, Sonnet 5, Opus 5 |
| C. OpenAI gpt-5-nano / gpt-5-mini | $0.05 / $0.40; $0.25 / $2.00 | ~$0.0008 / ~$0.004 | Not re-verified in Phase 0 | Abuse logs kept up to 30 days by default; ZDR needs approval |
| D. Gemini 2.5 Flash-Lite, paid tier | $0.10 / $0.40 | ~$0.0009 | Not re-verified in Phase 0 | Paid tier not used to improve products; **free tier is**, so never the free tier |

Why A: cheapest class with structured outputs I could verify, and the cleanest default
retention. B is the alternative if you want resume text never to leave AWS: then use the
India geographic profile with Haiku 4.5, at a higher price. Either way the P4 eval
decides; this question is what we start with.

## Q6. Embeddings

Corpus: ~1 lakh in-scope postings at ~512 tokens = ~5 crore tokens a full re-embed.

| Option | Cost of a full re-embed | Data leaves our host? | Notes |
|---|---|---|---|
| **A. Local `BAAI/bge-small-en-v1.5` in the Go worker via ONNX Runtime (recommended default)** | $0 | No | MIT licence; 384 dims. CPU time on the host: to be measured in P3 |
| B. Titan Text Embeddings V2 on Bedrock, in-region Mumbai | ~$1 at $0.02/1M (AWS launch blog; pricing page not rendered) | Stays in AWS Mumbai | 256/512/1024 dims |
| C. OpenAI `text-embedding-3-small` | ~$1 at $0.02/1M | Yes | |
| D. Voyage `voyage-4-lite` | ~$1, 200M free tokens | Yes | |

Why A as the default: resume embeddings are personal data, and a local model means
they never leave the host. Cost is small for all of them. P3 runs a bake-off of A, B and C
on the real matching eval (D3) and the winner is your call then.

## Q7. Extra sources

- **A. Not in v1; decide after P2's coverage report (recommended).** Workable's
  public widget is undocumented, and Remotive and Jobicy are small and remote-only.
- B. Add Workable now (~60,000 postings projected).
- C. Add all three now.

## Q8. Spend limits

- **A (recommended):** monthly AWS budget alert at **$5.27 (Rs 500)**, emails at
  50/80/100% and on forecast; an alert, not a cap. LLM caps in code: **$0.05 per user
  per day, $1.00 global per day**, $2 per eval run, $10 per phase-gate eval run.
- B. Different numbers: tell me.

At ~$0.001 a parse, $0.05 per user per day allows ~40 LLM actions per user per day.

## Q9. Host size while up

The spike (ARCHITECTURE.md, "Measured") found that memory isn't the limit: 5 lakh rows
took 1.8 GB. Ranking is CPU-bound, so vCPUs matter more. Mumbai on-demand:

| Option | vCPU / RAM | Per hour | Per month if left up |
|---|---|---|---|
| **A. m7g.xlarge, Graviton (recommended, pending checks)** | 4 / 16 GiB | $0.1166 | ~$85 |
| B. t4g.xlarge, Graviton, burstable | 4 / 16 GiB | $0.0896 | ~$65 |
| C. r7g.large, Graviton | 2 / 16 GiB | $0.0751 | ~$55 |
| D. x86 t3.xlarge (if arm64 fails) | 4 / 16 GiB | $0.1792 | ~$130 |

A needs two checks in P1: gVisor and every image on arm64. If either fails, D. Your
rule prefers fast tiers while up, and at a few hours per demo the difference is a few
rupees. The P8 load test confirms the size.

## Q10. Keep the original resume file?

- **A. Delete it after extraction (recommended).** Keep only the extracted text and
  the parsed profile, both encrypted. A re-upload is needed to re-parse.
- B. Keep the file encrypted, for re-parsing when the parser improves.

Why A: less to protect, nothing to serve, and the user always has their own file.

## Q11. How the corpus survives `down`

Rebuilding the corpus from the sources takes hours, which breaks the up budget.

- **A. `pg_dump` of the corpus to S3 at down, restore and reindex at up (recommended).**
  About $0.03 a month per GB-sized dump. In the spike, 5 lakh rows dumped in 76 s to 1.25
  GB and restored with all indexes in 497 s on a laptop. The in-scope corpus is about a
  quarter of that, so this should take 2 to 3 minutes of the 15-minute budget.
- B. Snapshot the data volume. Fast restore, ~$0.05 per GB-month while down, so about
  $1 a month for 20 GB, more than everything else combined.
- C. Rebuild from the sources on every up: hours of work, and hard on the sources.

## Q12. Time budget

How many hours a week can you give this, and is there a date (placement season,
an interview) the first demo must hit? The roadmap has no dates until you answer.

## Q13. Repository visibility

- **A. Public from P1 (recommended)**, like CodeJudge. Real postings, eval labels from
  real data and any resume never enter the repo (EVALS.md, "Data rules").
- B. Private until launch.

## Q14. Minimum age

- **A. Signups confirm they are 18 or over (recommended).** The DPDP Act treats
  children's data separately, and checking guardian consent isn't feasible.
- B. Allow 16 to 18 with a guardian consent flow. Much more work.

## Q15. AWS account

CodeJudge's DEPLOY.md says its account is on the AWS Free plan. Secondary sources say
that plan ends after 6 months or when credits run out, and that the account then closes
unless upgraded. One forum report shows Bedrock at zero quota on the Free plan.
**I couldn't check your account: the AWS credentials here had expired, so the plan and
its end date are unknown.**

- **A. Same account, upgraded to the paid plan, tagged per project (recommended).**
  Remaining credits carry over. One SSO login, one bill, and the 2 projects are separated
  by tags and stack names.
- B. A new, separate AWS account for RoleRank.
- C. Stay on the Free plan and accept the risk.

Whichever you pick, both budgets need the current rate: Rs 500 is now **$5.27**, not
the $5.70 CodeJudge's budget uses.

## Q16. Public contact address, and emails only you can send

The bot's User-Agent and /about/bot must show a contact email, so it becomes public.

- **A. `bot@dhruvgoyal.tech` forwarding to you (recommended).** Your personal Gmail
  stays out of every server log on the internet.
- B. Your personal Gmail.

Before P2, please also send these short emails, because the answers go in SOURCES.md:

- **Greenhouse and Ashby support:** we will read public job boards at 1 request per
  second as `RoleRankBot`, showing excerpts and links. Is that acceptable, and what's the
  rate limit? Their docs state none.
- **Workable support:** is `apply.workable.com/api/v1/widget/accounts/{slug}` permitted
  for this use? (Decides Q7.)

## Q17. Zero data retention with Anthropic

**Decided 2026-11-05: not for v1. Revisit before public signup.**

Anthropic's terms were checked on 2026-11-05 in the API data-retention docs and the commercial
retention article:
- Without ZDR, API inputs and outputs are deleted within 30 days.
- They can be kept up to 2 years if automated safety systems flag them, and longer where the
  law requires.
- API data isn't used for training without permission.
- ZDR is a separate agreement with Anthropic, not an API setting. It doesn't cover Covered
  Models (the Fable and Mythos families); Claude Haiku 5.5, which RoleRank uses, isn't one.

v1 is invite-only, so the 30-day window is accepted and stated on /privacy. Before public signup:
ask Anthropic for ZDR, or say plainly on /privacy why not. Either way, /privacy changes in the
same commit as the decision.

