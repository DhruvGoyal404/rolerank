# Phase 2: ingestion

Status: **exit criteria not met** (2026-11-03). See "Exit report" below.

## Second cycle (2026-11-01, 01:41 to 03:52 IST)

Ran on the owner's laptop at the normal 6-hour spacing, with the 1 request per second
per host limit unchanged. Queries are against `fetch_log` from the start of the cycle.

| Host | Requests | 200 | 304 | Other | MB |
|---|---|---:|---:|---:|---:|
| boards-api.greenhouse.io | 3,841 | 85 | 3,756 | 0 | 454.2 |
| api.ashbyhq.com | 2,670 | 35 | 2,635 | 0 | 76.3 |
| api.lever.co | 240 | 5 | 234 | 1 | 9.2 |
| remoteok.com | 1 | 1 | 0 | 0 | 0.6 |
| Total | 6,752 | 126 | 6,625 | 1 | 540.3 |

- **304 rate:** 98.1% across the ATS hosts. Himalayas sent no requests, as it refreshes daily.
- **Bytes:** 540 MB, against about 3.7 GB for the first cycle. This run includes the first full
  fetch of nine large boards, so it's above a normal cycle. A normal cycle is unknown until
  the next run.
- **Duration:** 131 minutes, the same as the first cycle, but that isn't fetch time. Boards
  come due 6 hours after their last check, so the first cycle's spread repeats. Requests per
  15 minutes rose from 105 to 1,501 and fell to 33, a peak of 1.7 requests per second
  across all hosts.
- **Postings:** 27,093 new, 109 closed. Nine boards the first cycle lost to the size limit
  account for about 27,000. Closed-posting accuracy is still unmeasured.
- **Failures:** 1. `jobgether` on Lever still exceeds the size limit and stays failing, with
  one active posting. The other nine boards came through.
- **Corpus:** 1,168 MB. C: has 84 GB free.

## Coverage after dedupe

Dedupe placed 4,367 new postings and merged 94, in about a minute. In-scope counts rest on
classifiers D6 hasn't validated yet.

| Source | Postings | In scope | India eng | India early | Interns | Remote open |
|---|---:|---:|---:|---:|---:|---:|
| Greenhouse | 157,980 | 31,963 (57%) | 1,769 (41%) | 25 (24%) | 4 | 213 (8%) |
| Ashby | 55,030 | 18,056 (32%) | 552 (12%) | 7 (6%) | 0 | 168 (7%) |
| Lever | 22,134 | 3,563 (6%) | 255 (5%) | 4 (3%) | 0 | 41 (1%) |
| Himalayas | 5,896 | 1,964 (3%) | 1,729 (40%) | 67 (65%) | 23 | 1,964 (82%) |
| RemoteOK | 98 | 25 (0%) | 0 | 0 | 0 | 0 |
| Total | 241,138 | 55,571 | 4,305 | 103 | 27 | 2,386 |

### Without Ashby (2026-11-02)

Ashby declined, and its data was deleted the same day (SOURCES.md). `bash scripts/ingest.sh
coverage`, run after the purge, no new fetch:

| Source | Postings | In scope | India eng | India early | Interns | Remote open |
|---|---:|---:|---:|---:|---:|---:|
| Greenhouse | 157,985 | 31,968 (85%) | 1,769 (47%) | 25 (26%) | 4 | 213 (9%) |
| Lever | 22,134 | 3,563 (9%) | 255 (6%) | 4 (4%) | 0 | 41 (1%) |
| Himalayas | 5,913 | 1,981 (5%) | 1,737 (46%) | 67 (69%) | 23 | 1,981 (88%) |
| RemoteOK | 99 | 26 (0%) | 0 | 0 | 0 | 0 |
| Total | 186,131 | 37,538 | 3,761 | 96 | 27 | 2,235 |

Against the table above: in scope down 32%, India engineering down 13%, India early-career
down 7%, interns unchanged, remote-open down 6%. The table above stays as measured.

**Concentration risk (figures before Ashby's removal):** India early-career hangs on Himalayas at 65% and remote-open at 82%.
The recovered boards added about 4,300 in-scope postings but only 24 in India and no India
early-career posting. Five of them hold no engineering role at all.

## Close report (2026-11-04)

**P2 is closed by the owner's decision, with one known v1 gap:** entry-level F1. Every other
eval passes. No threshold was changed for this report. The exit report of 2026-11-03 below is
kept as history.

| Criterion | Status | Evidence |
|---|---|---|
| 1.5 lakh active postings, per source | Met | 1.83 lakh on 2026-11-03 |
| Full cycle inside 6 h, 304 rate reported | Met | 131 minutes, 98.1% 304; `ingest status` shows lag and 304% |
| Never above the rate, 4 workers | Met | `TestWorkersNeverExceedTheRate` |
| 429, 5xx, huge bodies, gzip bombs, slow responses | Met | `fetch` tests, `TestSlowResponseTimesOut` |
| Kill mid-batch loses nothing | Met by test | `TestStopAtEveryPageResumesWithoutLossOrDuplicates` |
| Closed postings leave within a cycle, error under 1% | Met | 0.045% and 0.057% against a fresh fetch of 300 boards (`closecheck`) |
| Dedupe eval D5 | Met | Precision 298 of 300, 0.993 (0.976 to 0.998); recall 0.826 against 0.75 |
| Classifier eval D6 | **Partly** | Remote-open 59 of 60, India 60 of 60, intern 47 of 47 pass; **entry F1 0.70 misses 0.90** |
| Bot obeys a disallowing robots.txt | Met | `TestRobotsIsObeyedBeforeTheRequest` |
| Attribution stored per posting | Met | `TestEveryPostingReachesItsSourcesAttribution` |
| Bytes per cycle, split into 200 and 304 | Partly | Requests are split; bytes are only totalled |
| In-scope coverage report | Met | 20,957 in scope, 3,461 India engineering (2026-11-03) |

**How dedupe got there.** Six rounds, each scored on pairs the fix before it hadn't seen.
Rounds 4 and 5 still merged jobs that differed in one line: contract against permanent,
"required" against "preferred", a dropped on-call duty, a clearance level, a shift. The merge
now also needs matching employment terms and matching counts of requirement words, and text
similarity of 0.97 instead of 0.95. Round 6 was the first to pass. No posting-fresh pairs were
left by then, so round 6 used pairs never labelled before; some postings had appeared in other
pairs. Labels: sonnet for all pairs, opus for the 68 low-confidence ones.

### Known v1 gaps

| Gap | Measured | Threshold | Cause | v1.1 item |
|---|---|---|---|---|
| Entry-level F1 | 0.70: precision 58 of 67 (0.866), recall 0.58 | 0.90 | A posting that states no years can't be told from a mid or senior one by rules. 5 of 110 postings with an entry signal were entry, and each stands for about 115 postings. "1.5+ years" also reads as entry where the labels say mid | An entry signal that reads the whole posting, scored on a fresh entry sample |

Users feel it as entry roles shown as unknown or mid level. P3's ranking must not drop
unknown-level roles for a fresher, or the gap becomes a hidden job.

### Final audits (owner, 2026-11-04)

Scored with `bash scripts/label.sh audit <name>`. Gold needs kappa 0.8.

| Audit | Question | Agreed | Kappa (95% interval) | Verdict |
|---|---|---|---|---|
| d5-final-audit, 20 pairs | same job | 19 of 20 | 0.894 (0.667 to 1.000) | Gold |
| d6-final-audit, 10 postings | engineering | 9 of 10 | 0.615 (0.000 to 1.000) | Below 0.8 |
| | level | 5 of 8 | 0.564 (0.226 to 0.830) | Below 0.8 |
| | India | 8 of 10 | 0.643 (0.167 to 1.000) | Below 0.8 |

The D6 kappas miss 0.8, but only one disagreement is about the answer. A senior pricing data
scientist was called out of scope by the model and in scope by the owner, a data-analyst
rule-1 judgement. The other five are the owner answering "unsure" (three senior levels and one
India) or "region not stated" where the model said "no" for an on-site US role. Neither of
those makes a role India-open. Ten items can't size kappa: one disagreement in 10 gives 0.615.
The larger D6 audit of 2026-11-03, with the owner's corrections, stands as the gold check
(engineering 1.0, level 0.865, India 1.0). D6 stays gold on that audit, and this one is
recorded as a pass on the answers it rated, not as proof. Rule 1 may need a sentence on pricing
and analytics data scientists in v1.1.

P3 starts now.

## Exit report (2026-11-03, history)

**P2 hasn't passed.** The ingestion machinery works, and most criteria are met. Dedupe merges
different jobs far too often, two classifier thresholds aren't met, and closed-posting accuracy
was never measured. Numbers and method for the evals are in EVALS.md.

| Criterion | Status | Evidence |
|---|---|---|
| 1.5 lakh active postings, per source | Met | 186,131 canonical postings without Ashby, table above |
| Full cycle inside 6 h, 304 rate reported | Met | 131 minutes, 98.1% 304. Measured with Ashby, not re-run since |
| Never above the rate, 4 workers | Met | `TestWorkersNeverExceedTheRate`, 4 workers |
| 429, 5xx, huge bodies, gzip bombs | Met | `fetch` tests |
| Slow responses handled | Not shown | No test by that name was found; a 120 s timeout is configured |
| Kill mid-batch loses nothing | Met by test | `TestStopAtEveryPageResumesWithoutLossOrDuplicates`, `TestRedeliveryAfterCommitDoesNoWork` |
| Closed postings leave within a cycle, error under 1% | **Not measured** | Tests prove the behaviour; the comparison with a fresh fetch was never run |
| Dedupe eval D5 passes | **Fails** | Precision 0.763 against 0.99. Recall 0.83 against 0.90, not shown |
| Classifier eval D6 passes | **Fails** | Remote-open 9 of 17; entry level too few to size; India passes unproven |
| Bot obeys a disallowing robots.txt | Met | `TestRobotsIsObeyedBeforeTheRequest` |
| Attribution stored per posting, tested per source | Partly | Stored per source, and `TestEveryAdapterDeclaresHostsAndCredit` covers each adapter; not per posting |
| Bytes per cycle, split into 200 and 304 | Partly | Requests are split; bytes are only totalled, 540 MB |
| First run resumable, proved by killing it | Met by test | The same stop tests. No live kill is on record |
| In-scope coverage report | Met, overstated | 37,538 in scope; classifier precision makes this a ceiling |
| Per-source share recorded | Met | `coverage_snapshots`. The admin page is P6 |
| /sources and /about/bot pages | Met | Pages and tests, 2026-11-02 |

**Demo:** `ingest status` and `ingest coverage` exist. The dashboard with lag and 304 rate, and the
live worker kill, haven't been built or run.

**Against SPEC's targets:** 186,131 active against 2 lakh, and 37,538 in scope against 50,000.
Both are missed and stay as written.

**To reach the exit, in order:**

1. The owner reviews the D5 and level disagreements and rules on the precedents in EVALS.md.
2. Fix the dedupe key and the text comparison, then re-run dedupe and score on fresh pairs.
3. Tighten remote-open and the engineering classifier to the owner's scope rules, then re-score.
4. Measure closed-posting accuracy with a fresh fetch.
5. Split bytes by status, add the slow-response test, and add per-posting attribution or change
   that criterion with a written reason.

No exit criterion or threshold was changed for this report.
