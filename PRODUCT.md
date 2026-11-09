# Product

Inferred on 2026-11-09 from the owner's redesign brief and docs/SPEC.md, without an interview
(the owner asked for one report at the end). Assumptions are marked.

## What it is

RoleRank ranks real, legally sourced job postings against a resume and explains every match:
which skills fit, which are missing, and where the resume shows them. It never invents
experience. Free, sign in with Google or GitHub.

## Who uses it

- **Fresher** (primary): final-year student or recent graduate in India, projects instead of
  experience. Wants honest 0 to 1 year roles, no senior noise, what to learn next.
- **Intern seeker** (primary): a 2 to 6 month internship in India or remote from India.
- **Early-career engineer**: 1 to 4 years, wants better matches than keyword search.
- **Remote seeker**: in India, wants to know a "remote" job is really open to India.

Usage scene (assumption): a student on a mid-range Android phone or a laptop, often in the
evening, comparing a handful of roles before applying. Trust is the product: they have been
burned by vague match percentages and tools that make things up.

## Surfaces and modes

| Surface | Mode | Success |
|---|---|---|
| Landing `/` | Persuade | Understands the promise in one screen, signs in |
| `/sources`, `/about/bot`, `/privacy` | Read | Trusts where jobs come from and what happens to their data |
| `/signin` | Operate | Signs in with no doubt about what is kept |
| `/resume` (upload, review, confirm) | Operate | Uploads, sees progress, corrects and confirms the profile |
| `/matches` | Operate | Scans ranked roles fast, filters on a phone, opens the right ones |
| `/jobs/:key`, `/jd` | Operate | Sees the evidence and what to put first, applies at the source |
| `/tracker`, `/account` | Operate | Keeps track; exports or deletes everything |

## Voice

Plain, specific, calm. Says what happens and what is kept. Never hype, never a percentage,
never "AI-powered". Errors explain what went wrong and what to do next.

## Constraints

Strict CSP (`script-src 'self'`, no inline styles or scripts), WCAG 2.1 AA, 360 px phones to wide
desktops, light and dark both first-class, no heavy runtime dependencies, job pages `noindex`
with no JobPosting data, Greenhouse and Lever postings link-only.
