# Job sources

Every source RoleRank may fetch from, what it allows, and what it actually contains.
Everything here was checked on **2026-10-30 and 2026-10-31 (IST)** against the
provider's own documentation, its robots.txt, and live calls from a script that
identified itself honestly (`RoleRank-research/0.1 (student project; contact ...)`)
and made about 1 request per second per host. From now on, research scripts use
`bot@dhruvgoyal.tech` as the contact, never a personal address. Nothing was fetched through a browser,
proxy or faked identity.

This is an engineering record, not legal advice. Where a provider publishes no terms
for its public endpoint, this document says **unknown** rather than guessing.

## Summary

| Source | Status | Auth | Coverage for us | Main condition |
|---|---|---|---|---|
| Greenhouse job board API | **Approved** | None for GET | Large; ~0.9% India engineering | No published API terms; be gentle |
| Lever postings API | **Approved** | None for GET | Large; ~1.4% India engineering | `Crawl-delay: 1` on the API host |
| Ashby posting API | **Approved** | None | Medium; ~1.2% India engineering | Skip `isListed: false` jobs |
| Himalayas API | **Approved** | Optional free key | **Best for India-eligible remote** | Attribution, `applicationLink`, no redistribution |
| RemoteOK API | **Approved, small** | None | ~100 jobs per snapshot, ~1 India | Followable link back, name credit, no logo |
| Workable public widget | **Not in v1 (Q7)** | None | Medium; ~3% India | Endpoint not officially documented |
| Remotive, Jobicy | **Not in v1 (Q7)** | None | Small, remote only | Strict rate and link rules |
| SmartRecruiters | **Excluded** | | | robots.txt on the API host disallows us |
| Internshala | **Excluded** | | | Terms forbid automated access |
| LinkedIn, Naukri, Indeed | **Excluded** | | | Owner's rule; terms forbid it |
| Adzuna | **Excluded for v1** | Key | Unknown for India | Commercial licence needed after a 14-day trial |
| Arbeitnow | **Excluded** | | Germany-focused | Thin terms; out of market |
| National Career Service | **Excluded** | | | No public job-listing API found |
| HN "Who is hiring", community GitHub lists | **Excluded** | | | Personal posts or no licence found |

## Measured coverage

Random boards per ATS, taken from board names seen in Common Crawl (see
"Discovery"). Classification used simple title and location patterns, so treat the
shares as rough. The real classifiers come in P2 with their own eval (EVALS.md, D6).

| | Greenhouse | Lever | Ashby |
|---|---|---|---|
| Boards tried | 300 | 200 | 200 |
| Boards live (not 404) | 239 (80%) | 160 (80%) | 174 (87%) |
| Postings | 11,055 | 22,352 | 2,990 |
| Mean / median per live board | 46 / 14 | 140 / 23 | 17 / 8 |
| Engineering titles | 3,315 (30%) | 3,901 (17%) | 1,025 (34%) |
| India location | 230 (2.1%) | 947 (4.2%) | 52 (1.7%) |
| **India + engineering** | **102 (0.9%)** | **302 (1.4%)** | **36 (1.2%)** |
| India or open-remote, engineering | 208 (1.9%) | 1,152 (5.2%)* | 66 (2.2%) |
| Intern titles, any location | 208 | 274 | 56 |
| **India intern, any role** | **3** | **10** | **1** |
| **India early-career engineering** | **1** | **4** | **1** |
| Median description length (chars) | 10,186 (HTML) | 603 (`descriptionPlain` only)** | 5,365 |

\* Lever's `workplaceType: remote` matched the remote pattern even when a location
restricts it, so this row overcounts for Lever.
\*\* Lever splits the text into `descriptionPlain`, `lists` and `additionalPlain`.
The full text is their sum.

Himalayas, by its own search API on 2026-10-31:

| Query | Jobs open to India |
|---|---|
| `country=IN` | 6,249 |
| `country=IN&q=software` | 1,692 |
| `country=IN&q=intern` | 160 |
| `country=IN&q=engineer&seniority=Entry-level` | 98 |

**What this means.** Combined, 440 of 36,397 sampled ATS postings (1.2%) are India
engineering roles, 14 are India internships of any kind, and 6 are India early-career
engineering roles. The legal ATS APIs are a strong
source of remote and multinational roles, and a weak one for Indian freshers. That
finding drives three changes, proposed in SPEC.md and QUESTIONS.md: Himalayas as a
first-class source, the **Bring a JD** feature, and an optional allowlist of
first-party career pages (Q4).

### Known Indian employers

68 company names were tried as board tokens on Greenhouse, Lever and Ashby. A hit means
"a board with that name exists", **not** that it belongs to that company: board tokens
aren't verified identities.

- Hits (12): `eternal`, `cred`, `meesho`, `paytm`, `freshworks` (0 jobs), `fi`,
  `mindtickle` on Lever; `groww`, `inmobi`, `slice`, `digit` (0 jobs) on Greenhouse;
  `navi` on Ashby.
- No board under the obvious name (sample): Razorpay, Swiggy, Zomato, PhonePe,
  Zerodha, Flipkart, Myntra, Zoho, Postman, BrowserStack, Dream11, Nykaa, Ola,
  Zepto, Juspay, Cashfree.

Several of these may use another ATS, another token, or their own career site.
**Unknown** until discovery runs properly in P2.

## Scale

Projection from the measured board yield, using only board names found in partial
Common Crawl reads (the index server returned many 504s, so these counts are floors):

| Source | Board names found | Live share | Mean postings | Projected postings |
|---|---|---|---|---|
| Greenhouse | 2,578 | 80% | 46 | ~95,000 |
| Lever | 285 (index read mostly failed) | 80% | 140 (skewed by large boards) | ~32,000 |
| Ashby | 1,868 | 87% | 17 | ~28,000 |
| Workable (if Q7 = yes) | 3,474 | 97% of 60 tried | 18 | ~60,000 |
| Himalayas | | | | 6,249 India-eligible; full feed size unknown |
| RemoteOK | | | | ~100 |
| **Total** | | | | **~1.6 lakh without Workable, ~2.2 lakh with it** |

So **2 lakh active postings is reachable**; 5 lakh needs much better discovery, and
nothing measured yet shows it. Of those 2 lakh, roughly 25 to 30% are engineering roles,
and roughly 2 to 5% are India or open-remote engineering roles: about 4,000 to 10,000.
The target is proposed in SPEC.md: ingest at least 2 lakh, search 50,000 to 1.25 lakh
in-scope postings, and benchmark latency on a 5 lakh corpus so the claim holds at the top.

## Discovery: finding boards legally

No ATS publishes a list of its customers. These methods stay within the rules:

1. **Common Crawl URL index.** Query the CDX index for `boards.greenhouse.io/*`,
   `job-boards.greenhouse.io/*`, `jobs.lever.co/*`, `jobs.ashbyhq.com/*` and
   `apply.workable.com/*`, and keep only the first path segment, the board name. We
   read URLs from the index, not page content.
   Common Crawl's [terms of use](https://commoncrawl.org/terms-of-use) grant a limited
   licence and ask users to respect third-party rights. Its index server timed out
   often (HTTP 502 and 504) during this research, so the P2 tool must retry slowly and
   resume. Index used: `CC-MAIN-2026-43` (October 2026) and `CC-MAIN-2026-39`.
2. **Company lists → token guesses.** For each name, try the obvious tokens once on
   each API. A 404 costs one cheap request. A hit is verified by its `company_name`
   field or its careers link before it's trusted.
3. **Links on companies' own career pages.** A careers page linking to
   `jobs.lever.co/<token>` names its token. Only on hosts whose robots.txt allows it.
4. **User and employer submissions.** "Add a company" form, reviewed by hand.
5. **Re-validation.** A board that returns 404 three cycles in a row is retired.

Measured on 2026-10-31 from the two newest indexes: 4,900 Greenhouse, 2,984 Ashby and 284
Lever boards. **Lever is badly undercounted.** `jobs.lever.co`'s robots.txt blocks Common
Crawl's bot, so the index barely sees it. Lever needs methods 2 to 4.

**Lever step 1 (2026-11-01).** Names and official sites come from Wikipedia's ordinary
category and article pages, read through the fetcher, with one try per name on Lever.
Wikipedia's text is CC BY-SA; names and URLs are facts, and we credit the source. Wikidata
would be the cleaner source (CC0), but `query.wikidata.org/robots.txt` disallows `/sparql`,
so it isn't used. Lever's response doesn't name the company, so a hit counts only if the
company's own site links to that exact board, on a page robots.txt allows.

Not used: search engine scraping, guessing at scale beyond one try per name, or any
host whose robots.txt disallows us.

## Common rules for every source

- User-Agent: `RoleRankBot/<version> (+https://rolerank.dhruvgoyal.tech/about/bot; bot@dhruvgoyal.tech)`.
- robots.txt fetched per host, cached for 24 h, obeyed including `Crawl-delay`
  (RFC 9309: a 4xx means no restrictions; a 5xx or network error means assume
  complete disallow, and retry later).
- Content Signals (`Content-Signal:` lines, seen on Lever's, RemoteOK's and
  Workable's hosts): we use postings for **search** and pass a job's text to an LLM
  only when a user asks about that job (**ai-input**). We never use them for
  **ai-train**. Where `ai-input` isn't stated, the signal "neither grants nor
  restricts" by its own definition.
- Default limit: **1 request per second per host**, shared by every worker, with
  `Retry-After` honoured on 429 and 503, exponential backoff, and a circuit breaker
  after repeated 5xx.
- Conditional GET: store the `ETag`; send `If-None-Match`. **All three ATS APIs returned
  `304` with an empty body** (Greenhouse list and content, Lever, Ashby).
- Response size cap of 25 MB, gzip ratio cap, 120 s timeout.
- We store the text we need for search and show an excerpt plus a link to the
  original. Job pages are `noindex` and emit no `JobPosting` structured data. That
  keeps us from republishing the postings, and it's an explicit condition of
  Himalayas and Remotive.
- Removal: an employer can ask to be removed; the board is denylisted within 7 days.

---

## Greenhouse job board API

- **Docs:** https://developers.greenhouse.io/job-board.html (now redirects to
  https://docs.greenhouse.io/job-board.html)
- **Endpoints** (base `https://boards-api.greenhouse.io/v1/boards/{board_token}`):
  - `GET /jobs`: all published jobs. `?content=true` adds description HTML,
    departments and offices.
  - `GET /jobs/{id}`: one job. `?pay_transparency=true` adds `pay_input_ranges`.
  - `GET /`, `/offices`, `/departments`, `/sections`.
- **Auth:** none for GET. The docs say: "Job Board data is publicly available, so
  authentication is not required for any GET endpoints." POST (applications) needs a
  customer key; we never POST.
- **Rate limits:** not stated in the docs. **Unknown.** We use 1 request per second.
- **Caching:** responses carry `ETag` and `cache-control: max-age=0, private,
  must-revalidate`. `If-None-Match` returned 304 (tested).
- **Terms:** the docs state no terms for the public API, and the Greenhouse legal page
  lists customer agreements, not API or website terms. **Unknown.** robots.txt on
  `boards-api.greenhouse.io` and `boards.greenhouse.io` disallows only `/embed/`;
  `job-boards.greenhouse.io` disallows nothing.
- **Attribution:** none required. We link to `absolute_url` (the employer's posting)
  and show "via Greenhouse" as honest provenance.
- **Fields (list, verified live):** `id`, `internal_job_id`, `title`, `updated_at`,
  `requisition_id`, `location.name`, `absolute_url`, `language`, `metadata`,
  `first_published`, `company_name`, `application_deadline`, `education`,
  `data_compliance`; `meta.total`. Per job adds `content` (HTML-escaped),
  `departments`, `offices`, AI-disclaimer fields.
- **Stable ID:** `greenhouse:{board_token}:{id}`. Note `id` is the post;
  `internal_job_id` is the underlying job, shared by several posts of the same job.
  That helps dedupe.
- **Refresh:** `?content=true` once per cycle, conditional. That's one request per board
  instead of one per job, and an unchanged board returns 304 with no body.
- **Closed postings:** gone from the list. A single closed job returns **404**
  (tested with a missing ID). `application_deadline` in the past also counts as closed.
  A whole board returning 404 means the board was renamed or dropped.

## Lever postings API

- **Docs:** https://github.com/lever/postings-api (README)
- **Endpoints:** `GET https://api.lever.co/v0/postings/{site}?mode=json` with
  `skip` and `limit`; filters `location`, `commitment`, `team`, `department`,
  `level`, `group`. One posting: `GET /v0/postings/{site}/{id}`. EU-hosted
  accounts use `https://api.eu.lever.co/v0/postings/`; discovery tries both.
- **Auth:** none for GET.
- **Rate limits:** the README states one for application POSTs ("more than 2
  application POST requests per second" returns 429) and says limits may change
  without warning. None stated for GET. robots.txt on `api.lever.co` says
  `Allow: /` and **`Crawl-delay: 1`**, which we obey.
- **Caching:** `ETag`; 304 on `If-None-Match` (tested).
- **Terms:** the README tells customers "These jobs may be scraped by third parties."
  That describes what happens; it isn't a licence. Lever's
  [terms of service](https://lever.co/legal/terms-of-service) (updated 2023-08-25) cover
  customers and bar Lever's direct competitors; they say nothing on scraping.
  `jobs.lever.co` robots.txt sets `Content-Signal: search=yes, ai-train=no,
  use=reference` and blocks named AI training crawlers. We never train on postings.
- **Attribution:** none required. Link to `hostedUrl`.
- **Fields (verified live):** `id`, `text` (title), `categories` (`location`,
  `commitment`, `team`, `department`, `allLocations`), `country` (ISO alpha-2),
  `workplaceType`, `createdAt` (epoch ms), `opening[Plain]`, `description[Plain]`,
  `descriptionBody[Plain]`, `lists`, `additional[Plain]`, `hostedUrl`, `applyUrl`,
  `salaryRange`, `salaryDescription[Plain]`.
- **Stable ID:** `lever:{site}:{id}` (UUID).
- **Refresh:** the whole list per cycle, conditional. No `updated_at` field, so a
  change is detected by a content hash.
- **Closed postings:** gone from the list; a single closed posting returns **404**
  (tested).

## Ashby public job posting API

- **Docs:** https://developers.ashbyhq.com/docs/public-job-posting-api
- **Endpoint:** `GET https://api.ashbyhq.com/posting-api/job-board/{JOB_BOARD_NAME}`,
  `?includeCompensation=true` for pay data.
- **Auth:** none stated; works without any (tested).
- **Rate limits:** not stated. **Unknown.** 1 request per second.
- **Caching:** `cache-control: public, max-age=60, stale-while-revalidate=60` and
  `ETag`; 304 tested. Never poll a board more than once a minute.
- **robots.txt:** `api.ashbyhq.com/robots.txt` returns **401**. Under RFC 9309 a 4xx
  means no restrictions. `jobs.ashbyhq.com` disallows `/api/` on that host, which we
  don't use.
- **Terms:** none for this API. Ashby's [terms](https://www.ashbyhq.com/terms)
  (updated 2025-09-29) are customer terms; 5.1(g) bars use that could "overburden"
  the service. **Unknown** beyond that.
- **Attribution:** none required. Link to `jobUrl`.
- **Fields (verified live):** `id`, `title`, `department`, `team`, `location`,
  `secondaryLocations`, `address`, `isListed`, `isRemote`, `workplaceType`,
  `employmentType` (includes `Intern`), `descriptionHtml`, `descriptionPlain`,
  `publishedAt`, `jobUrl`, `applyUrl`, `compensation`,
  `shouldDisplayCompensationOnJobPostings`.
- **Rule:** jobs with `isListed: false` are "available only by direct link". We skip
  them, because the employer chose not to list them.
- **Stable ID:** `ashby:{board}:{id}`.
- **Closed postings:** gone from the board response.

## Himalayas

- **Docs:** https://himalayas.app/api; terms: https://himalayas.app/terms (clause 113,
  "Terms and Conditions for API Users")
- **What we read:** the browse feed by cursor, filtered locally with their India rule,
  which is what their docs say to do to collect one country. Their search can't be used
  for this: it pages with `&page=`, and robots.txt disallows `/jobs*&page=`. A page can
  hold under 20 jobs, so only an empty page or a missing cursor ends a walk.
- **Endpoints:** `GET https://himalayas.app/jobs/api` (browse feed, `limit` up to 20,
  cursor paging with `nextCursor`); `GET /jobs/api/search` (`q`, `country`,
  `worldwide`, `seniority`, `employment_type`, `timezone`, `sort`, `page`, 20 a page);
  `GET /jobs/api/companies/{company}/jobs/{slug}`: 200 open, **410 closed** (with
  `expiryDate`), 404 unknown.
- **Auth:** optional free key, `Authorization: Bearer`. Without one, callers share a
  public allowance. We use a key, `HIMALAYAS_API`, read from the environment on the server
  and sent to `himalayas.app` only.
- **Rate limits:** with a key, **60 requests per minute and 10,000 per UTC day**; 429
  with `Retry-After`. Data refreshes daily, so we poll at most a few times a day.
- **Terms (quoted):** personal and commercial use allowed without asking. "Every
  Apply action for a job from Himalayas must use the applicationLink supplied by the
  API." "Clearly identify Himalayas as the source of the jobs, with a visible link."
  "Show the jobs only on sites and apps you operate." "Do not pass them on to third
  parties, such as other job boards, aggregators, job search engines, ... Google Jobs
  or LinkedIn Jobs, including through feeds or JobPosting structured data." "Cache job
  data where you can, stay within your request limits, and remove jobs once they
  expire." Keys server-side only.
- **India:** a job is open to India when `locationRestrictions` is empty or includes
  India and `timezoneRestrictions` includes UTC+05:30 (Himalayas' own rule).
- **Fields (verified):** `guid`, `title`, `excerpt`, `description`, `companyName`,
  `companySlug`, `companyLogo`, `employmentType`, `seniority`, `minSalary`,
  `maxSalary`, `currency`, `salaryPeriod`, `locationRestrictions`,
  `timezoneRestrictions`, `categories`, `parentCategories`, `pubDate`, `expiryDate`,
  `applicationLink`.
- **Stable ID:** `himalayas:{guid}`.
- **Closed:** `expiryDate` passed, or the per-job check returns 410.
- **Dedupe note:** Himalayas aggregates jobs that may also be on an ATS. When both
  exist we keep both records and link them; the UI shows the source the user reached
  it through, and Himalayas' apply link stays on the Himalayas record.

## RemoteOK

- **Endpoint:** `GET https://remoteok.com/api` (JSON array; element 0 is a legal notice).
- **Auth:** none.
- **Terms (quoted from the response itself):** "Please link back (with follow, and
  without nofollow!) to the URL on Remote OK and mention Remote OK as a source, so we
  get traffic back from your site. If you do not we'll have to suspend API access.
  Please don't use the Remote OK logo without written permission as it's a registered
  trademark, please DO use our name Remote OK though."
- **robots.txt:** `Content-Signal: search=yes,ai-train=no,use=reference`,
  `Crawl-delay: 1`; disallows AJAX query paths and named AI crawlers. We call only `/api`.
- **Measured:** 99 jobs in the snapshot, dated 2026-08-15 to 2026-10-29; 1 with an
  India location; 62 with engineering tags. Small, but harmless and well-defined.
- **Fields:** `id`, `slug`, `epoch`, `date`, `company`, `company_logo`, `position`,
  `tags`, `description`, `location`, `salary_min`, `salary_max`, `url`, `apply_url`.
- **Refresh:** a few times a day. Closed: absent from the feed.
- **UI rule:** the job card links to `url` with a normal followable link and says
  "via Remote OK". No logo.

## Workable public widget (not in v1, Q7)

- **Official docs** cover only the customer API (`{subdomain}.workable.com/spi/v3`,
  bearer token, scope `r_jobs`). That isn't usable by us.
- **The public endpoint** used by Workable's own embeddable widget,
  `https://apply.workable.com/api/v1/widget/accounts/{slug}`, appears only in
  third-party write-ups. **It is not documented by Workable.**
- **robots.txt** on `apply.workable.com`: `Content-Signal: search=yes, ai-input=yes,
  ai-train=no`, empty `Disallow`. So robots permits us.
- **Terms:** Workable's [terms](https://www.workable.com/terms) (updated 2026-03-02)
  are customer terms with no scraping clause.
- **Measured:** 58 of 60 random boards answered; 1,027 jobs; 32 in India. Fields:
  `title`, `shortcode`, `code`, `employment_type`, `telecommuting`, `department`,
  `url`, `shortlink`, `application_url`, `published_on`, `created_at`, `country`,
  `city`, `state`, `education`, `experience`, `function`, `industry`, `locations`.
- **Risk:** an undocumented endpoint can change or close with no notice, and its
  permission is implied only by robots.txt. Decision (Q7): left out of v1;
  revisit after the P2 coverage report and any reply from Workable.

## Remotive, Jobicy (not in v1, Q7)

- **Remotive** (https://github.com/remotive-com/remote-jobs-api): `GET
  https://remotive.com/api/remote-jobs`. Advised at most 4 requests a day; more than
  2 a minute is blocked. Must link back to the Remotive URL and name Remotive. Must
  not submit its jobs to third-party sites, including Google Jobs and LinkedIn.
  Jobs are delayed by 24 hours.
- **Jobicy** (https://jobicy.com/jobs-rss-feed): `GET https://jobicy.com/api/jobs`,
  automated passes at most once an hour. Credit Jobicy; apply links must go to Jobicy's
  pages. No India geo slug is documented.
- Both are remote-only and small. Decision (Q7): revisit only if they add in-scope
  jobs the others don't have.

---

## Excluded, with reasons

| Source | Evidence |
|---|---|
| **SmartRecruiters** | `api.smartrecruiters.com/robots.txt` has `User-agent: *` / `Disallow: /`, allowing only LinkedInBot on `/v1/companies/`. Its posting API docs also describe API-key or OAuth access with the `internal_postings_read` scope. Robots disallows us, so we don't use it. |
| **Internshala** | Its [terms](https://internshala.com/terms) prohibit "using automated systems, software, bots, spiders, scripts, or manual processes to access, extract, scrape, crawl" and "any data mining". robots.txt also disallows `/api/` and every query string. This is the site Indian interns use most; we still don't touch it. |
| **LinkedIn, Naukri, Indeed** | Excluded by the owner's rule. No further checking needed. |
| **Adzuna** | Needs `app_id` and `app_key` from registration. Its terms require a written agreement for "commercial analysis, resale of data", a licence after a 14-day trial for commercial use, and an "Adzuna" label linked to Adzuna on every advert. India support isn't shown on the overview page: **unknown**. Not worth the licence question in v1. |
| **Arbeitnow** | Germany-focused, terms not stated beyond "free and public". Out of market. |
| **National Career Service (India)** | No public job-listing API found. Only state-level integrations are mentioned in government sources. |
| **SimplifyJobs and similar GitHub lists** | No licence visible on the repository page. Without a licence there's no permission to reuse. |
| **HN "Who is hiring"** | Posts are personal comments, not employer feeds. HN API terms for this use: **unknown**. |
| **Google Jobs, job search engines** | Their results are other sites' content. Not a source. |

## Compliance record

On 2026-10-31, during P2, two manual `curl` test requests went to
`himalayas.app/jobs/api/search?country=IN&page=...`, which the rule `/jobs*&page=` in
Himalayas' robots.txt disallows. They were sent by hand, outside the fetcher. The fetcher
enforces robots.txt and later refused the same URL on its own. One-off research requests
now check robots.txt first.

## Open items

- Rate limits for Greenhouse and Ashby GETs are **unknown**. Drafts to both providers,
  and to Workable about its widget, are in OUTREACH.md for the owner to send. Replies are
  recorded here. Until then, 1 request per second per host.
- Greenhouse's website terms for the public API: **unknown**. Same email.
- Re-check every source's terms and robots.txt quarterly; a CI job diffs each robots.txt
  weekly while the project is active and opens an issue on change.
