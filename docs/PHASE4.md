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

## Known gaps and open items

| Item | State | Next |
|---|---|---|
| No real resumes tested | D1 is synthetic and easy | Owner: upload one or two real resumes once P6's screen exists, and check the draft |
| Profile confirmation screen | API done; the page is P6 | P6 |
| D7's job-description cases | Need Bring a JD | P5 |
| Zero-retention with the LLM provider | An agreement with Anthropic, not an API setting; not arranged | Owner decision before public signup |
| The live evals don't run in CI | Tier T1: run before prompt or schema changes, at about $0.07 a run | As designed |

## Spend

The three live runs (one D1 crash, then two complete runs) cost about $0.20 in all, under the
ledger and the owner's $5 monthly console limit.
