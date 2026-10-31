# Product spec

RoleRank ranks real job postings from legal, public sources against your resume and
tells you why each one fits: which skills match, which are missing, and whether the
role suits your level and location.

Status: proposal for Phase 0. Items marked **(Qn)** wait on a decision in
[QUESTIONS.md](QUESTIONS.md). The defaults here assume the recommended answer.

## Who it is for

| User | Situation | What they need most |
|---|---|---|
| **Fresher** (primary) | Final-year student or recent graduate in India, projects instead of experience | Honest roles for 0 to 1 year, no "senior" noise, what to learn next |
| **Intern seeker** (primary) | Student looking for a 2 to 6 month internship | Internships filterable by duration and start, in India or remote from India |
| **Early-career engineer** | 1 to 4 years, India | Better matches than keyword search, skill gaps for the next role |
| **Remote seeker** | Based in India, wants global remote work | To know a "remote" job is actually open to India |

Not for: recruiters, employers or anyone screening candidates.

## The problem

Keyword search gives a fresher hundreds of results, mostly senior. Match-score tools
give a percentage with no reason. "Remote" often means "remote in the US". Tailoring
tools happily invent experience. RoleRank's angle: **every match is explained,
every claim is traceable, and every number is measured.**

## What we learned before writing this

Phase 0 measured the sources on 2026-10-31 ([SOURCES.md](SOURCES.md)). Three findings
shape the product:

1. **The legal ATS APIs barely reach Indian fresher roles.** In a random sample of
   36,397 Greenhouse, Lever and Ashby postings: 440 (1.2%) were India engineering
   roles, 14 were India internships of any kind, and 6 were India early-career
   engineering roles.
2. **Most well-known Indian employers aren't reachable by obvious board name** on
   Greenhouse, Lever or Ashby. Many run their own career sites.
3. **Himalayas is the exception:** 6,249 India-eligible remote jobs, 1,692 software,
   160 intern, with a public API that allows commercial use on stated conditions.

So "ingest more" alone won't serve the primary user. The product adds two things
that were not in the original brief: **Bring a JD** (below) and an allowlisted
first-party career-page source (Q4).

## User stories

Priority: **M** must have for v1, **S** should, **C** could.

### Account

- **M** Sign in with Google or GitHub, with no password.
- **M** Delete my account and everything with it, in one action, and see what that covers.
- **M** Export my data as JSON.
- **M** Read a plain /privacy page that matches what the system does.

### Resume and profile

- **M** Upload a PDF or DOCX and get a structured profile within 30 seconds.
- **M** See exactly what was read from my resume, and correct it. The confirmed
  profile, not the raw parse, drives matching.
- **M** Be told clearly if a file can't be read (scanned, password-protected, too
  large), with what to do instead.
- **S** Keep more than one profile, for different target roles.
- **M** Set preferences: role type (intern, full-time), locations, remote, target
  roles, graduation year.

### Matches

- **M** See a ranked list of open roles for my profile in under a second.
- **M** Filter by level, location, remote eligibility, internship, company, freshness.
- **M** For every match, see matched skills, missing skills, level fit and location
  fit, with no LLM call needed.
- **M** See a fit band (Strong, Good, Stretch) with reasons, not a fake precise
  percentage.
- **M** Open the original posting at its source, with the source credited.
- **M** See how fresh a posting is, and never be sent to a closed one knowingly.
- **S** Get a short written explanation on demand.
- **S** Hide a company, or mark a match as not relevant. This feeds the eval set
  with consent, never a model.

### Bring a JD (added in Phase 0)

- **M** Paste a job description or upload its text, and get the same match
  breakdown, gaps and tailoring as for an indexed job.

This serves the primary user directly. A fresher hears about a role through campus,
a friend or LinkedIn, and needs help with that role. The user supplies the text, so
there's no sourcing problem, and the job isn't added to the shared index.

### Skill gaps

- **M** See which skills recur among the roles I'm a near-match for, ranked by how
  many more roles each would unlock.
- **S** See that gap for one target role title.

### Tailoring

- **M** For a chosen job, get resume-line suggestions. Each cites the line it
  rewrites and shows it side by side.
- **M** Be told plainly when the honest answer is "your resume doesn't show this".
  Never a suggestion that invents experience.
- **S** Copy accepted suggestions out. RoleRank doesn't generate or edit the resume file.

### Tracker

- **M** Save a job, and move it through saved, applied, interview, offer, rejected.
- **M** Keep a job's details in my tracker after the posting closes.
- **S** Add notes and dates.

### Alerts

- **S** Opt in to an email digest of new strong matches, at a chosen frequency, with
  one-click unsubscribe. Depends on Q1: while the site is down, nothing is sent.

### Trust and operations

- **M** See which sources RoleRank uses and how to get a company removed (/sources).
- **M** Public page explaining the bot: User-Agent, rate limits, contact (/about/bot).
- **M** Admin-only metrics page: ingestion lag, source health, LLM spend, eval status.

## Scope for v1

### In

Resume upload and confirmed profile; ingestion from the approved sources; dedupe;
hybrid search with deterministic ranking; explainable matches; Bring a JD; skill-gap
view; tailoring with the grounding gate; tracker; cost caps; deletion and export;
/privacy, /sources and /about/bot; evals, bench harness, backups; up and down scripts.

### Conditional

Email alerts (Q1). First-party career pages (Q4). Public signup (Q2).

### Out of v1

| Out | Why |
|---|---|
| Auto-apply, form filling | Against many employers' terms, and harms the user |
| Writing whole resumes or cover letters | Raises invention risk; tailoring by line is the honest scope |
| Scanned resumes (OCR) | Adds an attack surface and errors; ask for a text PDF |
| Non-English resumes and postings | Evals don't cover them yet. Filtered by `language` |
| Non-engineering roles in search | Focus. Stored as metadata only, for dedupe and stats |
| Employer accounts, paid plans | Different product |
| Mobile apps | Responsive web is enough |
| Salary prediction | Sparse data, easy to mislead. Show only what a source states |
| Company reviews, referrals, chat | Scope |

## Non-goals (ever, unless this document changes)

- Scraping LinkedIn, Naukri, Indeed, Internshala or any site that forbids it.
- Screening or ranking candidates for employers.
- Selling or sharing user data. No ads, no third-party trackers.
- Training models on user resumes or on ingested postings.
- Claims with no measurement behind them.

## Success measures

| Measure | Target | Measured by |
|---|---|---|
| Relevance on real data | D3 nDCG@10 >= 0.75 | EVALS.md |
| Search and rank latency | p95 < 200 ms on the full bench corpus | Bench harness |
| Freshness | 95% of in-scope postings checked within 6 h while up | Ingestion metrics |
| Closed-link rate | < 3% of clicked links are dead | Sampled weekly while up |
| Hallucination | 0 invented skills on pressure cases | EVALS.md |
| Cost | Mean LLM cost per active user-day < $0.01 | Ledger |
| Personal outcome | The owner uses it for a real job search | Honest report at the end of P7 |

## Scale targets

Reasoning and measurements are in SOURCES.md, "Scale". Summary:

| Target | Number | Status |
|---|---|---|
| Postings ingested and deduplicated (all roles) | **>= 2 lakh active** as the v1 goal, 5 lakh as a stretch that depends on discovery | Boards found so far project to ~1.6 lakh, ~2.2 lakh with Workable |
| In-scope postings embedded and searched | **50,000 to 1.25 lakh** | 20 to 34% of ingested postings are engineering roles |
| Bench corpus for latency claims | **5 lakh** synthetic rows | Measured so the claim holds at the top of the range |
| Refresh | Every 6 h while up, with conditional GET | 304 confirmed on all three main ATS APIs |

The original target, 2 to 5 lakh postings, is kept as an ingestion and engineering
goal. It isn't a measure of value: at the measured shares, 5 lakh ATS postings would
hold roughly 6,000 India engineering roles, and roughly 15,000 to 25,000 counting remote
roles open to India. What counts for the primary user is
in-scope coverage, tracked as its own metric on the admin page.
