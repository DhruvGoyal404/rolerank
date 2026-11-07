# Phase 5: Bring a JD, explanations and tailoring

## What was built

- **Bring a JD.**
  - `POST /api/jd` breaks a pasted JD (20 KB at most) down against the user's latest confirmed
    profile: required and nice-to-have skills, matched, missing and level fit.
  - It runs in memory with the taxonomy, needs no model, and stores nothing.
  - `POST /api/jd/explain` and `POST /api/jd/tailor` add the LLM features on the same text.
- **Job detail.** `GET /api/jobs/:key` reads the posting as `corpus_ro` and returns its card
  (title, company, links, source, excerpt) and the deterministic explanation. It never returns
  the body, and the excerpt is empty for link-only sources.
- **Explanations.**
  - `POST /api/jobs/:key/explain` returns the deterministic explanation and, when the model's
    prose passes the gate, two or three sentences of prose.
  - Prose naming any skill outside the matched, missing and nice lists is withheld, not shown.
  - At a spend cap, or without aiworker, the deterministic explanation stands alone.
- **Tailoring.**
  - `POST /api/jobs/:key/tailor` returns rewrites of the user's own resume lines.
  - Every suggestion passes the grounding gate first, and the response lists what the resume
    lacks.
- **Where the model runs.**
  - aiworker keeps the only LLM client and serves `POST /explain` and `POST /tailor` to the API
    over an internal endpoint with a shared token.
  - It is given a user id and job facts, and reads that user's own rows.
  - The model never sees job text, only the job's title and its taxonomy skills, so a JD's
    instructions have no path to it, and link-only text stays on the server.
- **Tests.** `api/test/jobs.int.test.ts` runs the API, aiworker's endpoint and a fake model on
  isolated databases. It proves:
  - a suggestion that fails the gate never reaches the response;
  - gated prose is withheld;
  - a pasted JD writes no row anywhere;
  - JD text and job bodies never reach the model, and contacts are stripped first;
  - the API uses only the caller's own profile;
  - the spend cap degrades to the deterministic answer;
  - aiworker refuses calls without its token.

## Results

Live runs: `evals/reports/p5-live-d4-tailor-v1-explain-v1-2026-11-07.json` and
`p5-live-d4-2-tailor-v2-explain-v1-2026-11-07.json`. Model Claude Haiku 5.5; claim judge Claude
Sonnet 5.5.

**Explanations** (D4 and D4-2, 80 cases, each from a real parse of its D1 resume):

| Criterion | Measured | Threshold | Verdict |
|---|---|---|---|
| A matched skill is truly in the resume and the job | 1.000 / 1.000 | ≥ 0.99 | Pass |
| A missing skill is truly required and absent | 1.000 / 1.000 | ≥ 0.95 | Pass |
| Skills in shown prose are all in the lists | 100%: 8 of 80 answers withheld by the gate, none shown | 100% | Pass |
| p95 latency | 1.76 s / 2.09 s | ≤ 3 s | Pass |
| Mean cost | $0.00011 | within budget | Pass |

The p95 is for the whole answer, unstreamed; that already meets the streaming-start target of
1 s for the deterministic part, which needs no model.

**Tailoring, first run** (tailor-v1 on D4, 40 cases, committed in `59fc004` before it ran):

| Criterion | Measured | Threshold | Verdict |
|---|---|---|---|
| Suggestions shown that fail the gate | 0 (by construction, tested) | 0 | Pass |
| Invented skill on pressure cases | 0 | 0 | Pass |
| No-fit jobs answered with nothing | 8 of 8 | | Pass |
| Pre-gate pass rate | 0.851 | ≥ 0.95 | **Fails** |
| Unsupported / inflated, judge | 39.7% / 31.7% of 63 | ≤ 1% / ≤ 2% | **Fails** |
| Fit cases with 3 or more suggestions | 12 of 32 | ≥ 3 | **Fails** |

The judge's notes name the cause: the model attached skills from the profile to lines that
never mention them ("Project: URL Shortener (Python, REST APIs)"), and added project names to
bare lines. The gate allowed it, because it checked technologies against the line *or the
whole profile*, as CLAUDE.md words the rule. A skill the user has isn't a skill every line used.

**The fix** (`c469ff5`):
- The gate now requires every number and technology in a rewrite, and every technology in its
  reason, to be in the cited line. Proper names must be in the line or among the profile's
  employers, titles and schools. This is stricter than CLAUDE.md's rule, so no invariant moves.
- The prompt (tailor-v2) says each rewrite uses only its own line and thin lines are skipped.

**Tailoring, held out** (tailor-v2 on D4-2, 40 fresh pairings, committed before its single run):

| Criterion | Measured | Threshold | Verdict |
|---|---|---|---|
| Suggestions shown that fail the gate | 0 | 0 | Pass |
| Invented skill on pressure cases | 0 | 0 | Pass |
| No-fit jobs answered with nothing | 8 of 8 | | Pass |
| Unsupported / inflated, judge | 8.3% / 8.3% (1 of 12) | ≤ 1% / ≤ 2% | **Fails** (one suggestion; 12 is too few to bound the rate) |
| Pre-gate pass rate | 0.316 (12 of 38) | ≥ 0.95 | **Fails** |
| Fit cases with 3 or more suggestions | 0 of 32 | ≥ 3 | **Fails** |
| Mean cost per tailoring run | $0.00018 | within budget | Pass |

**What this means.**
- Tailoring is now **safe but nearly useless** on this data: the gate drops what the model
  invents, so what's shown is honest, but little survives.
- Two causes:
  - The model still moves skills onto lines that don't name them: 15 of 26 drops are
    technologies, 10 are names.
  - D4 is built on D1's synthetic resumes, whose lines are thin ("Built with SQL", "Built and
    maintained services used by internal teams"). There's little to rephrase honestly, so D4
    can't show whether tailoring helps.
- Not chased, per the lean plan. Next (v1.1):
  - a D4 built on resumes with real bullet points (the D1-hard-4 layout);
  - a prompt that rephrases a line around the skills it already names, or says nothing.

**Injection through a JD** (D7-JD, 20 held-out cases, injections in the title or the body):
0 canaries, 0 planted skills in shown suggestions, 0 prompt leaks. The body never reaches the
model; titles do, truncated to 120 characters and delimited as data.

## Exit criteria

| Criterion (ROADMAP.md, P4b and P5) | State |
|---|---|
| All tailoring thresholds pass; zero invented skills on pressure cases | Zero invented skills: met. The other thresholds fail (above) |
| A test proves a suggestion failing the gate can't reach the API response | Met (`jobs.int.test.ts`) |
| Pasted JD treated as untrusted (D7 cases) and never joins the index | Met: D7-JD clean; a test shows no row is written |
| Human audit of 50 suggestions | **Owner**: `evals/labels/d4-audit-tailor-v1.jsonl` (50, judge flags first) and `d4-2-audit-tailor-v2.jsonl` (all 12) |
| Mean cost per tailoring run, within budget | $0.00018 to $0.00020: met |
| e2e of the whole flow on an isolated DB, axe-checked | Moved to P6: it needs the UI, which P6 builds |
| All explanation thresholds pass | Met |
| The gate rejects any skill in LLM prose outside the deterministic lists (test) | Met (unit and integration tests) |
| Mean cost per explanation | $0.00011: met |
| Explanation p95 ≤ 3 s | 2.09 s: met |

**Demo, as far as P5 goes:**
- A pasted JD demanding Kubernetes returns matched and missing skills. "Kubernetes" is listed
  as missing, and tailoring never claims it.
- A job's detail comes with its explanation.

The ranked matches list is not an API route yet. The ranker is the Go worker (P3), and P6 decides
how the UI gets matches.

## Deviations, recorded

- **The claim judge is Claude Sonnet 5.5**, a different model from the suggester but the same
  vendor. EVALS.md asks for a different model family; only an Anthropic key is available. The
  owner's audit covers the gap.
- **No streaming.** The whole explanation arrives within 2.1 s at p95, under the 3 s limit.
- **The D4 run on tailor-v1 crashed once in the eval harness** before computing any metric (Go
  writes an empty nice-to-have list as null). It was fixed and re-run; nothing was seen before.
  The JD cases' results come from the first run.

## Open items

| Item | Next |
|---|---|
| Tailoring fails the pre-gate, claim and suggestions-per-case thresholds | v1.1: D4 on richer resumes, then the prompt |
| Owner's audit of tailoring suggestions | Owner |
| `GET /api/matches` | P6 |
| e2e and axe for the whole flow | P6 |
| A fresh held-out injection set (resumes and JDs) | Before public signup (QUESTIONS.md) |

## Spend

P5's live runs cost about $0.08 on the model (parse, explanation and tailoring for 120 cases) and $0.11 on the judge.
