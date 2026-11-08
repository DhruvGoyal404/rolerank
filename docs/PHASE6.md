# Phase 6: minimal product UI

## What was built

- **The ranker service** (`worker/cmd/ranker`). P3's hybrid search, served to the API on the
  internal network behind a shared token, like the extractor.
  - It embeds the profile text with bge's query prefix, as P3 chose, and searches as `corpus_ro`.
  - The handler is unit-tested without ONNX. The binary builds with `-tags onnx` in its compose
    service, `ranker`.
- **`GET /api/matches`.** It sends the ranker only canonical skills, titles, level, place and a
  short text: never resume lines or contact details. It returns each posting's card, its fit band
  and its deterministic explanation. Saved preferences decide the place, remote and target roles;
  the query string can override them for one search.
- **Tracker** (`tracker_items`). Save a job, move it through saved, applied, interviewing, offer and
  not this time, or remove it. The title, company and link are copied from the public posting, so
  they outlive it.
- **Preferences** (`user_preferences`) and **data export** (`GET /api/me/export`, 3 a day):
  everything RoleRank keeps about the user, decrypted for them alone, as one JSON file.
- **`/api/me`** now says where the user is (no resume, a draft profile or a confirmed one, and why
  a file failed), so the UI knows what to show.
- **A pipeline fix.** An unexpected error while reading a resume left it "parsing" forever. It now
  fails visibly with the code `internal`, and the page says so (integration test).
- **The UI.**
  - Pages: Home, Sign in, Resume (upload, live status, review, correct and confirm), Matches
    (filters, fit bands, skills had and missing, credit, freshness, save), Job (apply link,
    explanation, explanation in words, tailoring, save), Bring a JD, Tracker, Account
    (preferences, export, deletion), Privacy, Sources and About our bot.
  - **The look** comes from the logo: navy ink, indigo structure, and lime as a sparing signal.
    The fit band is drawn as the logo's three bars: Strong lights all three, Good two, Stretch one.
    Resume lines show in monospace, as the user's own words. Display type is a condensed system
    face; there are no web fonts, since the CSP allows fonts only from self.
  - **Light and dark themes.** The system setting decides until the user picks one; the pick is
    remembered.
  - **Icons** from `web/brand/logo.png`: favicons (16, 32, 48 and an .ico), a 180 apple-touch icon,
    192 and 512 app icons, and 192 and 512 maskable icons with the mark inside the safe zone. The
    logo's white corners are made transparent for the favicons, and app icons are full-bleed.
- **Attribution per source** (`web/src/attribution.ts`), as SOURCES.md requires:
  - "via Greenhouse" and "via Lever", linking to the employer's posting;
  - Himalayas: credit, and "Apply" goes to their `applicationLink`, unchanged;
  - Remote OK: a followable link (no nofollow), the name credited, never their logo.

## Results

| Criterion (ROADMAP.md, P6) | Measured | Verdict |
|---|---|---|
| Playwright e2e across every M story, on an isolated DB | 20 tests, all passing: sign-in, upload to profile (under 30 s), correction and confirmation, the three unreadable-file cases, matches with bands, explanations, credits, freshness and filters, job detail with apply link, explanation in words and tailoring, Bring a JD, the tracker, preferences, export, deletion, sign-out, /privacy, /sources, /about/bot. The API runs on throwaway `_test` databases (`api/test/e2e-server.ts`) | Pass |
| axe clean at WCAG 2.1 AA on every page, light and dark | Every signed-out and signed-in page, both themes, no violations | Pass |
| Usable on a 360 px phone | No horizontal overflow on any page; screenshots reviewed | Pass |
| LCP under 2.5 s on throttled 4G | Median of 3: 1.00 s on `/`, 1.01 s on `/signin` on the final run; 1.66 s and 1.23 s on an earlier run while the machine was busy (slow 4G: 150 ms RTT, 1.6 Mbps, CPU 4x) | Pass (see deviations) |
| Strict CSP with no violations in e2e | Every e2e page fails on any console error, which includes CSP violations; the CSP's script block is tested | Pass |
| Attribution visible per source, tested against each rule | `attribution.test.tsx` per source, and e2e on the rendered pages | Pass |
| /privacy reviewed line by line against the code | Reviewed; added the tracker, preferences, export, the search service, explanations, tailoring without a model, and pasted JDs | Pass |

**Live check of the ranker service** (2026-11-08): the ONNX build in its compose container, against
the real corpus.
- Ten ranked hits for an entry-level data profile; 0.88 s for the first, cold call, then 0.084 s
  and 0.071 s. A request without the token got 401.
- It caught a real bug the fakes had hidden: the ranker names bands in lower case ("strong"), and
  the UI would have shown every real match as Stretch. The API now normalises band names, and
  both fakes use the real service's lower case, so the tests cover it.

## Stories not in v1

- The skill-gap page and the admin metrics page: v1.1, by the owner's decision of 2026-11-04.
- Keeping a job in the tracker after its posting closes is built (the tracker copies the job's
  details), but no e2e closes a posting.

## Deviations, recorded

- **LCP is measured with Playwright and Chrome's DevTools protocol, not Lighthouse.** The
  throttling is Lighthouse's slow-4G profile; this needs no new dependency. One warm-up load runs
  first, then three loads with the cache off, and the median counts.
- **Preferences cover where the user can work, remote, and target roles.** Role type is the
  internship filter on Matches, and the graduation year is read from the resume.
- **Sign-out reloads the page**, so nothing from the session stays in memory and no page's
  redirect can race it.

## What surprised us

- **The e2e suite found two real bugs that unit and integration tests had not.**
  - A resume that hit an unexpected error stayed "parsing" forever.
  - The ranker's lower-case band names would have shown every real match as Stretch; the live
    check found that one.
- **Two UI races:** deleting an account, and signing out, each raced a page's own redirect to
  /signin. Deletion now clears the session and goes home in one render, and sign-out reloads home.
- **The logo's corners are white, not transparent,** which would have put white wedges on dark
  browser tabs. The favicons derive a proper alpha from the tile's edge.

## Open items

| Item | Next |
|---|---|
| A walkthrough by someone who hasn't seen it (ROADMAP demo) | Owner |
| The ranker image for deployment | P8 |
| Countries other than India in preferences | v1.1 |
