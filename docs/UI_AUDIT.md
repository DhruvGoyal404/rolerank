# UI audit before the redesign

Date: 2026-11-09. Method: every page captured on the real local stack (synthetic user, a D1
eval resume) at 360, 768 and 1280 px in light and dark (78 screenshots, zero console or CSP
errors, no horizontal overflow), then read against DESIGN.md, the Impeccable craft floor, the
Hallmark floor and the Web Interface Guidelines. Problems only; what works is kept (listed last).

## Across the product

1. **No typographic voice.** One system font stack (`system-ui`, Segoe UI on Windows) for
   headings and body: nothing says RoleRank. Heading weights and the body differ only in size.
2. **The nav is a row of seven equal text links.** Matches, Bring a JD, Tracker, Resume, Account,
   Sign out and the theme switch all weigh the same; Sign out sits among destinations. At 360 px
   it wraps to three rows and pushes the h1 down by about 140 px.
3. **Unicode glyphs as icons.** The theme switch uses ☾ and ☀ (`App.tsx:42`).
4. **The footer is three links in a thin bar.** No brand close, no statement of what RoleRank is.
5. **Empty screens are mostly empty ground.** Tracker (empty), the confirmed profile and Bring a
   JD leave 60 to 70% of a 1280 × 900 screen blank, with the footer floating far below.
6. **No route-change feedback, no toasts.** Save and status changes give no confirmation beyond
   the button label; there is no undo.
7. **Lime, the logo's signal colour, is used only inside the band icon.** The two themes differ
   only in value; dark mode doesn't use the logo's lime-on-navy pairing at all.

## Landing (`Home.tsx`)

8. Steps are three equal columns under numbered circles (index numbering as decoration), followed
   by about 200 px of nothing above the footer at 1280 px.
9. Two actions of nearly equal weight ("Get started", "Where the jobs come from") compete.
10. The example card shows the claim but not the proof: the cited resume line sits in small grey
    text under the chips, the most distinctive thing about RoleRank set as fine print.
11. No sources, no "what is never invented" promise in its own place; the page ends after the steps.
12. The HTML is an empty SPA shell with `noindex`: crawlers see nothing, and there is no title per
    page, no canonical, no Open Graph card.

## Sign in (`SignIn.tsx`)

13. Provider links are plain underlined text, not buttons; the 18+ box sits between paragraphs;
    nothing says what is kept until the user reads /privacy.

## Resume (`Resume.tsx`)

14. Parsing shows one sentence ("Reading your profile… this usually takes under a minute.") with no
    stages and no sense of progress.
15. The upload is a bare file input inside a box; no drop zone, no chosen-file name, no limits
    shown until an error.
16. The confirmed profile is a short stack of headings with "None found." in grey; the actions sit
    flush under the last line with no separation.
17. No first-run journey: nothing shows that sign-in, upload, confirm and matches are four steps,
    or which one the user is on.

## Matches (`Matches.tsx`)

18. The fit band sits at the far right of each card, about 900 px from the title at 1280 px: the eye
    has to cross the card to pair them.
19. Have and missing chips differ only by fill and a dashed edge; missing chips usually outnumber
    the ones you have, so the card reads as a list of gaps. No label tells a sighted user which row
    is which.
20. The card's only actions are a small Save and a "via Himalayas" credit; there is no way to apply
    from the list.
21. The filter row is four equal inputs and two checkboxes at one level; nothing shows how many
    filters are on until the list changes.
22. All 50 cards render at once into a 9,350 px page with no grouping by band.
23. "Jobs last refreshed" and the stale marker exist (Part 1) but read as fine print.

## Job page (`Job.tsx`) and Bring a JD (`BringJD.tsx`)

24. The job page's header has no fit band: the one thing the user came from the list to confirm.
25. The excerpt is cut mid-sentence with "…" and set at the width of the page head.
26. The three tailoring outputs hide behind a "Tailor my resume" button and then stack as one long
    column; there is no way to jump between evidence, wording and order.
27. Long resume lines in the evidence table run to full length with no clamp.
28. Bring a JD: a single textarea and a "1 KB of 20 KB" counter; the result appears far below the
    form on a phone with nothing to bring it into view.

## Tracker (`Tracker.tsx`) and Account (`Account.tsx`)

29. The empty tracker is a yellow warning box, the tone used for real warnings, for a normal
    first state.
30. Account puts preferences, export and deletion in one column at one weight; deletion is not set
    apart as the one irreversible action.

## Public reading pages (`Sources.tsx`, `AboutBot.tsx`, `Privacy.tsx`)

31. Long single columns with no contents; /privacy is about 1,000 words with no way to jump to
    "Deleting your account".

## What works and stays

- The fit band as the logo's three bars, never a percentage.
- Missing skills distinct from had skills, and the cited resume line as evidence.
- The evidence table, before/after swaps and the numbered "put these first" list (structure kept,
  presentation redone).
- Contrast already passes AA in both themes; focus rings, the skip link and the 44 px targets.
- Every error message names the problem and the next step.
