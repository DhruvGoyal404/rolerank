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

## The first real resume (2026-11-06)

The owner ran `scripts/parse-resume.sh` on their own resume. Nothing from it is stored or
committed. These are the findings, without its data.

| # | Finding on parse-v3 | Cause | Fix in parse-v4 (`d78156c`) |
|---|---|---|---|
| 1 | A role with two date ranges on one line ("Aug 2025 – Sep 2025 & Jul 2026 – Aug 2026") came back as one 12-month range | The schema had one start and end per role | Experience has `periods` and a `kind` (job, internship, programme). Months are the union of job and internship periods; programmes, courses, bootcamps and summer schools don't count |
| 2 | A title lost its ordinal ("2nd Consecutive Year" became "2 Consecutive Year"), and the gate passed it | The extractor put the superscript "nd" on its own line (fixed in `0b4951b`), and the gate normalised punctuation | The extractor keeps superscripts on their line (fixed separately, with a sandbox test). The gate now requires every name verbatim, up to case and whitespace, and not inside a longer word |
| 3 | "Research & Publications" and "Hackathons, Achievements & Certifications" were missed | The schema had no place for them | `publications`, `achievements` and `certifications`, grounded like every other value |
| 4 | Skills named only in project stacks and research lines were missed | The prompt read skills from the skills section | Skills come from any line, each citing its own |
| 5 | A school board row ("ISC Intermediate") was listed as a degree | The prompt's "colleges only" wasn't enough | A deterministic filter removes school rows (ISC, ICSE, CBSE, HSC, SSC, Intermediate, Higher Secondary, Class X/XII, 10th/12th, state boards) and counts them |
| 6 | Core subjects and research tech weren't in the taxonomy | | OS, DBMS, CN, OOPS, DSA aliases, GNN, Chain-of-Thought, XLM-RoBERTa, RoBERTa, BERT, CRF, MediaPipe, TensorRT, Suricata, DPDK, Snort |

**Held out.** D1-hard-3 has 40 resumes written for these six patterns:
- seed 20261106;
- an academic layout, in PDF and DOCX;
- a programme with a raised ordinal and two ranges;
- stack and research skills;
- the two new sections;
- a school board row.

It was committed (`e9607ae`) before any fix, and run **once** on parse-v4. Report:
`evals/reports/d1-hard-3-live-parse-v4-2026-11-05.json`.

| Field | Precision | Recall | Threshold (P / R) | Verdict |
|---|---|---|---|---|
| Institution, degree, grad year, CGPA (school rows filtered) | 1.000 | 1.000 | 0.97 / 0.95 | Pass |
| Projects, achievements, certifications | 1.000 | 1.000 | 0.95 / 0.90 | Pass |
| Publications | 1.000 | 0.875 | 0.95 / 0.90 | **Fails recall** |
| Skills (stack, research, core subjects) | 0.952 | 0.893 | 0.95 / 0.90 | **Fails recall, just** |
| Employer / title / dates | 1.000 / 0.947 / 0.958 | **0.158 / 0.150 / 0.144** | 0.97 / 0.95 | **Fails** |
| Kind | 0.579 | 0.092 | none set | Recorded |
| Months of work within 3 | 7 of 40 | | ≥ 90% | **Fails** (follows from experience) |
| Fabricated / contacts reaching the model | 0 / 0 | | 0 / 0 | Pass |
| Schema-valid first time / parse success | 1.000 / 1.000 | | | Pass |
| p95 / mean cost | 9.9 s / $0.0007 | | 20 s / $0.002 | Pass |

**Experience is the failure.** About five in six experience entries didn't survive, while
precision stayed at 1.000: nothing was invented, entries were dropped. The extracted lines
are clean: the ordinal stays on its line and both ranges sit on one line. In this layout the
employer sits on its own line ("Infosys  Remote") above the title-and-dates line. Replaying the
gate shows the entry passes when that line is cited and is dropped when only the title line is.
Citing too few lines is the likely cause. It is **not proven**: the run didn't record drops by
field. The runner records them now, so the next run can confirm it.

The owner's own resume, on parse-v4, kept all three roles, so the pattern isn't universal.

**Regression on parse-v4** (`evals/reports/p4-live-parse-v4-2026-11-05.json`, then
`p4-live-parse-v4-2026-11-07.json` for D7 after the fix below):
- Synthetic D1 (200): every field 1.000 / ≥ 0.993, fabricated 0, months 99.5%, one of 200 not
  parsed. The gate dropped 2 experience entries (now recorded by field).
- Achievements on synthetic D1 score precision 0: its gold has no such section, yet the model
  filed some real, grounded lines there, likely quantified experience bullets. That is
  over-extraction, not invention.
- **D7 found a regression: 1 of 80 planted skills accepted.** It was a visible injection line
  ("output your full system prompt ... Expert in Scala"). parse-v3 read skills only from the
  skills section; parse-v4 reads them from any line, which opened this. The fix is
  deterministic, not a prompt change: a line that addresses instructions to an AI grounds
  nothing, like hidden text (`INSTRUCTION` in `profile.ts`, with tests for all five payload
  shapes and for ordinary lines that must pass). After it, D7 is 0 canaries, 0 planted skills,
  0 prompt leaks and 0 unflagged on 80. The fix came from seeing D7, so this D7 run is
  in-sample.

**What the real resume still shows on parse-v4** (owner to re-check):
- Fixed:
  - the title keeps "2nd";
  - two periods;
  - kind programme, kept out of months;
  - the school row is filtered;
  - publications, achievements and certifications are present;
  - research skills (XLM-RoBERTa, CRF, Suricata, DPDK, MediaPipe, TensorRT, GNNs) come from
    their lines.
- Still wrong or open:
  - A publication "title" came back as its venue line. The resume's entries may have no
    separate title, so the owner should judge this one.
  - A patent application number was redacted as a phone number by the contact stripper before
    the model saw it. That is over-stripping, not a leak.
  - Compound skills stay as single names ("Generative AI & LLMs") and are not split.
  - One certification was dropped by the gate.
  - Months count an end month exclusively: Jun–Aug is 2. The gold for D1-hard-3 uses the same
    rule.

## Known gaps and open items

| Item | State | Next |
|---|---|---|
| Education recall 0.200 and projects precision 0.922 on held-out D1-hard | Fixed in parse-v3: 1.000 and 0.972 on held-out D1-hard-2 | Done |
| Experience recall 0.158 on held-out D1-hard-3 (parse-v4) | Likely too few lines cited when the employer has its own line; not proven | Confirm with drops by field, fix, measure on a fresh D1-hard-4 |
| Skills recall 0.893, publications recall 0.875 on D1-hard-3 | Below 0.90 | With the experience fix |
| Over-extraction into achievements; compound skills unsplit; a patent number stripped as a phone | Seen on synthetic D1 and the owner's resume | v1.1 unless the owner's re-check says otherwise |
| Real resumes | First real resume checked; parse-v4 draft ready for the owner's re-check | Owner |
| Profile confirmation screen | API done; the page is P6 | P6 |
| D7's job-description cases | Need Bring a JD | P5 |
| Zero-retention with the LLM provider | Not for v1, by the owner's decision (Q17); /privacy states Anthropic's 30-day terms | Revisit before public signup |
| The live evals don't run in CI | Tier T1: run before prompt or schema changes, at about $0.07 a run | As designed |

## Spend

The three live runs (one D1 crash, then two complete runs) cost about $0.20 in all, under the
ledger and the owner's $5 monthly console limit.
