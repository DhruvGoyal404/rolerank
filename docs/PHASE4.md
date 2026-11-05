# Phase 4: resume parsing

## What was built

- **Sandboxed extraction.**
  - `worker/internal/extract` checks size, magic bytes and the DOCX zip directory: entries,
    declared and inflated size, compression ratio, and XML that declares entities.
  - It then runs a single-use container: no network, read-only root, every capability
    dropped, no-new-privileges, nobody, 512 MB, 1 CPU, 64 processes, gVisor in production.
  - Inside (`extractor/`), pdf.js with eval off and mammoth return numbered lines. White,
    invisible, tiny and Word-hidden text is flagged.
  - `cmd/extractor` serves this to aiworker behind a shared token.
- **Encryption at rest.** Resume lines, stripped contacts and the profile are sealed with
  AES-256-GCM under a per-user key. The key is wrapped by `ROLERANK_KEK` and bound to its
  column, row and user. The uploaded file is deleted once extracted (Q10). Production refuses
  to start without the key.
- **The LLM gateway.**
  - Contacts are stripped first.
  - Lines go to Claude Haiku 5.5 only as a delimited data block, and hidden or tiny lines are
    never sent.
  - There are no tools, thinking is off at low effort, and the reply must follow a JSON schema,
    then Zod.
  - The grounding gate drops, and counts, any value not in a visible line it cites. Years of
    experience are computed from grounded dates.
- **The spend ledger.** It reserves the worst case before a call and settles after, under Q8's
  caps ($0.05 per user, $1.00 global, per UTC day).
- **aiworker and the API.**
  - `POST /api/resumes` is rate-limited per user, 5 an hour and 20 a day. The resumes table is
    the queue (`SKIP LOCKED`).
  - Users read, correct and confirm their profile. Every read is scoped by user, and a
    foreign id is 404.
- **Evals.** D1 (200 resumes) and D7 (80 injection cases) come from seeded generators.
  `scripts/eval-live.sh` runs both against the real sandbox and model.

## Results

Live run on 2026-11-05: `evals/reports/p4-live-2026-11-05.json`, Claude Haiku 5.5, prompt
`parse-v2`.

| Criterion | Measured | Threshold | Verdict |
|---|---|---|---|
| D1 skills, employer, title, dates, institution, degree, grad year, CGPA, projects | precision and recall 1.000 on every field and every segment | 0.90 to 0.97 | Pass |
| Fabricated values (not in the resume text) | 0 | 0 | Pass |
| Months of experience within 3 | 200 of 200 | ≥ 90% | Pass |
| Schema-valid first time / parse success | 1.000 / 1.000 | ≥ 0.99 / ≥ 0.98 | Pass |
| Contact details reaching the LLM | 0 | 0 | Pass |
| p95 end to end (sandbox plus model) | 6.2 s | ≤ 20 s | Pass |
| Mean cost per resume | $0.0003 | ≤ $0.002 | Pass |
| D7 canary output / planted skill accepted / system prompt leaked | 0 / 0 / 0 of 80 | 0 | Pass |
| D7 hidden-text cases flagged | 60 of 60 | all | Pass |
| Hostile files: zip bomb, XML bomb, polyglot, password, 100k pages, deep nesting, scanned | each refused or contained, under gVisor in CI | all | Pass |
| Sandbox has no network and no writes | root write EACCES/EROFS, network ENETUNREACH | | Pass |
| No plaintext at rest | no column of any table holds the canary | | Pass |
| Spend caps under concurrency | 50 parallel reservations of $0.01 under a $0.05 cap: 5 succeed | | Pass |
| Account deletion | no row with the user's id remains, the ledger included | | Pass |

## What the D1 result does and doesn't show

**D1's perfect scores show the pipeline is wired correctly, not that it reads real resumes.**
Three reasons:
- D1 comes from our own generator: plain one-font PDFs and simple DOCX. A real resume has
  design templates, icons, tables and odd fonts.
- Two fixes came after reading an earlier D1 run, so the final run is partly in-sample.
  - The grounding gate refused a date written `Mar'24`, because it looked for "2024". It now
    accepts a year after an apostrophe.
  - The prompt (`parse-v2`) says school marks (Class X/XII) are not education.
- The first run's "180 fabricated values" was a scoring bug, not a parsing one: it counted
  real school rows missing from the gold. Fabricated is now measured against the resume text,
  as EVALS.md defines it.

## The honest figure: D1-hard, held out (2026-11-05)

D1-hard has 40 resumes from a new seed. Its layouts are the ones real Indian fresher resumes
use:
- a true two-column sidebar;
- tables, in PDF and as Word tables;
- icon contact rows;
- mixed fonts;
- Canva-style letter-spaced headings with a skills grid;
- Overleaf-style right-aligned dates;
- dates like `Jul'24`, `08/2021` and "Till date".

70% are freshers and interns. The set was committed (`0cf9d23`) before it was run. It was run
**once**, and nothing was tuned afterwards. The report is
`evals/reports/d1-hard-live-2026-11-05.json`.

| Field | Precision | Recall | Threshold (P / R) | Verdict |
|---|---|---|---|---|
| Employer, title, dates | 1.000 | 1.000 | 0.97 / 0.95 | Pass |
| Skills | 1.000 | 0.980 | 0.95 / 0.90 | Pass |
| Projects | 0.922 | 0.922 | 0.95 / 0.90 | **Fails precision** |
| Institution, degree, grad year, CGPA | 1.000 | **0.200** | 0.97 / 0.95 | **Fails recall** |
| Fabricated values / contacts reaching the model | 0 / 0 | | 0 / 0 | Pass |
| Months of experience within 3 | 40 of 40 | | ≥ 90% | Pass |
| Schema-valid first time / parse success | 1.000 / 1.000 | | | Pass |
| p95 / mean cost | 4.8 s / $0.0003 | | | Pass |

**Why education fails.** Of the 40, 8 kept their education, and they are exactly the 8 table
PDFs. In those, degree, institute, year and score sit in one row. In the sidebar, Overleaf,
Canva and Word-table layouts they sit on separate lines.

An education entry cites a single line (`line`), and the grounding gate requires the
institution and the degree in that one line, so it drops the entry. Precision stays 1.000:
nothing is invented. But on real layouts most users would see no education in their draft and
would have to add it by hand.

This was diagnosed from the layout, not tuned against the set.

**Why projects miss.** In the Overleaf and table layouts, a project's name shares its line with
its tech stack ("Online Judge | Go, Python"). The model sometimes kept the stack in the name.

**The fix is known, and isn't measured here.**
- Education will cite `lines`, as experience already does, and the gate will check each value
  against all of them.
- The prompt will say a project's name excludes its stack.

D1-hard can't measure that fix, because the fix came from seeing it. A fresh held-out set,
D1-hard-2, does.

## The fix, measured on held-out D1-hard-2 (2026-11-05)

`parse-v3` makes education cite several lines and keeps tech stacks out of project names. It
was built from D1-hard's diagnosis. D1-hard-2 then measured it:
- 40 new resumes in the same five layouts, seed 20261107;
- committed (`31c72c6`) before its one run, with nothing tuned afterwards;
- report: `evals/reports/d1-hard-2-live-2026-11-05.json`.

| Field | D1-hard, parse-v2 | **D1-hard-2, parse-v3** | Threshold (P / R) |
|---|---|---|---|
| Institution, degree, grad year, CGPA | 1.000 / 0.200 | **1.000 / 1.000** | 0.97 / 0.95 |
| Projects | 0.922 / 0.922 | **0.972 / 0.972** | 0.95 / 0.90 |
| Skills | 1.000 / 0.980 | 1.000 / 0.997 | 0.95 / 0.90 |
| Employer, title, dates | 1.000 / 1.000 | 1.000 / 1.000 | 0.97 / 0.95 |
| Fabricated / contacts reaching the model | 0 / 0 | 0 / 0 | 0 / 0 |
| p95 / mean cost | 4.8 s / $0.0003 | 4.5 s / $0.0003 | 20 s / $0.002 |

With `parse-v3`, synthetic D1 still scores 1.000 on every field, and D7 is still 0 canaries,
0 planted skills and 0 prompt leaks on 80 cases (`evals/reports/p4-live-2026-11-05.json`).

**D1-hard-2 is now P4's honest parsing figure: every threshold passes.** It is still
synthetic: our generator's take on real layouts, not real files. The owner's own resume
(`scripts/parse-resume.sh`) is the first real check.

## Known gaps and open items

| Item | State | Next |
|---|---|---|
| Education recall 0.200 and projects precision 0.922 on held-out D1-hard | Fixed in parse-v3: 1.000 and 0.972 on held-out D1-hard-2 | Done |
| Real resumes | The owner can check one now: `bash scripts/parse-resume.sh my-resumes/<file>` | Owner |
| Profile confirmation screen | API done; the page is P6 | P6 |
| D7's job-description cases | Need Bring a JD | P5 |
| Zero-retention with the LLM provider | Not for v1, by the owner's decision (Q17); /privacy states Anthropic's 30-day terms | Revisit before public signup |
| The live evals don't run in CI | Tier T1: run before prompt or schema changes, at about $0.07 a run | As designed |

## Spend

The three live runs (one D1 crash, then two complete runs) cost about $0.20 in all, under the
ledger and the owner's $5 monthly console limit.
