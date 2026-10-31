# Provider outreach

Drafts for the owner to send from `bot@dhruvgoyal.tech`, once Cloudflare Email Routing
works. Replies go into SOURCES.md, with their date. RoleRank proceeds at 1 request per
second per host while waiting, and stops at once if a provider says no.

Recipient addresses aren't filled in: find each provider's current support or developer
contact on its own site, rather than trusting one recorded here.

---

## Greenhouse

**Subject:** Public Job Board API: rate limits and acceptable use for a student project

Hello,

I'm a student in India building RoleRank, a non-commercial project that ranks public
job postings against a person's resume and explains each match. It's open source:
https://github.com/DhruvGoyal404/rolerank

I'd like to read public job boards through the Job Board API
(`boards-api.greenhouse.io/v1/boards/{token}/jobs`). Your documentation says GET
endpoints need no authentication, but I couldn't find a rate limit or terms for this
API, so I'd rather ask than assume.

What I plan to do:

- Identify as `RoleRankBot`, with this email address as the contact.
- Make at most 1 request per second across all boards.
- Refresh each board about every 6 hours, using `If-None-Match`.
- Show a short excerpt and link to the original posting for applying.
- Never post applications, train models on postings, or republish full descriptions.
- Remove any employer's board on request.

Could you tell me:

1. Is this use acceptable?
2. Is there a rate limit, or a rate you'd prefer?
3. Are there terms for this API that I should follow?

Thank you,
Dhruv Goyal

---

## Ashby

**Subject:** Public Job Posting API: rate limits and acceptable use for a student project

Hello,

I'm a student in India building RoleRank, a non-commercial project that ranks public
job postings against a person's resume and explains each match. It's open source:
https://github.com/DhruvGoyal404/rolerank

I'd like to read public job boards through your Public Job Posting API
(`api.ashbyhq.com/posting-api/job-board/{name}`). The documentation doesn't state a rate
limit or terms of use, so I'd rather ask than assume.

What I plan to do:

- Identify as `RoleRankBot`, with this email address as the contact.
- Make at most 1 request per second across all boards.
- Refresh each board about every 6 hours, using `If-None-Match`, and never more often
  than your `max-age` of 60 seconds.
- Skip any job with `isListed: false`.
- Show a short excerpt and link to `jobUrl` for applying.
- Never train models on postings or republish full descriptions.
- Remove any employer's board on request.

Could you tell me:

1. Is this use acceptable?
2. Is there a rate limit, or a rate you'd prefer?
3. Are there terms for this API that I should follow?

Thank you,
Dhruv Goyal

---

## Workable

**Subject:** Reading public job listings: is the widget endpoint permitted?

Hello,

I'm a student in India building RoleRank, a non-commercial project that ranks public
job postings against a person's resume and explains each match. It's open source:
https://github.com/DhruvGoyal404/rolerank

Your documented API (`/spi/v3`) needs a customer token, so it isn't open to me. Your
career pages' widget appears to read public jobs from
`apply.workable.com/api/v1/widget/accounts/{slug}`. It isn't in your documentation, and
your `robots.txt` on that host doesn't disallow it, but I'd rather ask than rely on that.
I haven't added it to RoleRank, apart from a test of 60 boards at about 1 request per
second on 2026-10-31.

If permitted, I would:

- Identify as `RoleRankBot`, with this email address as the contact.
- Make at most 1 request per second across all accounts.
- Refresh each account about every 6 hours.
- Show a short excerpt and link to the original posting for applying.
- Never train models on postings, in line with your `ai-train=no` signal.
- Remove any employer on request.

Could you tell me:

1. May I use this endpoint for this purpose?
2. If not, is there a public feed you'd prefer I use?
3. Is there a rate limit I should follow?

Thank you,
Dhruv Goyal
