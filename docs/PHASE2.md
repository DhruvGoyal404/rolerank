# Phase 2: ingestion

Status: in progress. This file gathers the measured results as they arrive, and closes when
the exit criteria in ROADMAP.md are met.

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

**Concentration risk:** India early-career hangs on Himalayas at 65% and remote-open at 82%.
The recovered boards added about 4,300 in-scope postings but only 24 in India and no India
early-career posting. Five of them hold no engineering role at all.
