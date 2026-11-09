# RoleRank design system

The visual system for every RoleRank screen. Product context is in [PRODUCT.md](PRODUCT.md).
Tokens live in `web/src/styles.css` under `:root`; nothing past the token block uses a literal
colour or font family.

## The idea

**Evidence, not percentages.** RoleRank's promise is that every match is explained and every
claim is traceable. The interface shows that literally:

- **The fit mark is the logo.** The logo's three rising bars are the fit band: Strong lights all
  three and the third in lime (the logo's check), Good lights two, Stretch one. It is the only
  place a band is drawn, and it never becomes a percentage, a ring or a gauge.
- **Claims carry their source.** A matched skill points to the resume line that shows it (`L12`),
  a posting shows its source and age, a refresh shows when. Citations are set like footnote
  references: small, tabular, in the body face, never monospace.

Everything else is quiet so these two things read first.

## Colour

From the logo: navy ground, indigo bars, a lime check. Lime is a signal, used sparingly: the
Strong bar, the dark theme's primary action, and the focus ring on dark. It is never text on a
light ground (it fails contrast) and never a fill behind body text.

| Role | Light | Dark | Use |
|---|---|---|---|
| `--bg` | `#F6F7FB` | `#0B1022` | Page ground |
| `--surface` | `#FFFFFF` | `#121834` | Cards, panels, inputs |
| `--sunk` | `#EDEFF6` | `#080C1B` | Wells, table heads, skeleton base |
| `--ink` | `#0D1226` | `#E8EBF6` | Text, headings |
| `--muted` | `#4A5170` | `#A6ADC8` | Secondary text (≥ 4.5:1 on bg and surface) |
| `--line` | `#DCE0EA` | `#252D52` | Borders at rest |
| `--line-strong` | `#C3C9DA` | `#364074` | Control borders on hover, dividers that matter |
| `--control` | `#7D84A0` | `#6670A3` | Input edges (≥ 3:1, WCAG 1.4.11) |
| `--indigo` | `#2A2385` | `#B3AEFF` | Links, active nav, chips you have |
| `--primary` / `--primary-ink` | `#2A2385` / `#FFFFFF` | `#B2F12B` / `#0D1226` | The one primary action per screen |
| `--lime` | `#B2F12B` | `#B2F12B` | The Strong bar; never text on light |
| `--have-bg` / `--have-ink` | `#ECEBFA` / `#241E73` | `#221D5C` / `#D9D6FF` | A skill the resume shows |
| `--miss-ink` | `#4A5170` | `#A6ADC8` | A skill it doesn't (dashed outline, no fill) |
| `--warn-bg` / `--warn-ink` | `#FFF4D6` / `#5C3D00` | `#3A2E0C` / `#FFE3A3` | Stale postings, hidden-text notices |
| `--danger` | `#A0182B` | `#FF8A98` | Destructive actions, errors |
| `--focus` | `#2A2385` | `#B2F12B` | Focus ring |

Rules: one primary fill per screen; surfaces separate by a tone step and a 1px `--line`; no
gradients, no glass, no glow. Both themes are designed, not inverted: dark's primary is lime on
navy, the logo's own pairing.

## Type

Two self-hosted variable faces (OFL), latin subset, `font-display: swap`, the display face
preloaded:

- **Display: Schibsted Grotesk** (600 to 800). Headings, the wordmark, band labels. A newspaper
  grotesk: sturdy and plain-spoken, the voice of a ledger, not a startup.
- **Text: Atkinson Hyperlegible Next** (400 and 600). Everything else. Drawn for legibility at
  small sizes on cheap screens, which is where most of RoleRank's users read.

| Token | Size / line height | Use |
|---|---|---|
| `--t-xs` | 13 / 18 | Citations, captions, fine print |
| `--t-sm` | 14 / 20 | Labels, chips, facts, table text |
| `--t-md` | 16 / 24 | Body (never smaller) |
| `--t-lg` | 18 / 28 | Lede |
| `--t-xl` | 22 / 28 | Card titles, h3 |
| `--t-2xl` | 28 / 34 | h2 |
| `--t-3xl` | clamp(32px, 4vw, 44px) / 1.1 | Page h1 |
| `--t-hero` | clamp(36px, 6vw, 68px) / 1.02 | Landing h1 only |

Display tracking -0.025em (hero -0.035em); body 0. Measure 65ch for prose. Headings use
`text-wrap: balance`. Numbers that align (tracker, counts, citations) use tabular figures. Labels
are sentence case, never tracked capitals; no eyebrows above headings.

## Space and layout

4-point scale: `--s1` 4, `--s2` 8, `--s3` 12, `--s4` 16, `--s5` 24, `--s6` 32, `--s7` 48,
`--s8` 64, `--s9` 96.

- One container, `--width` 72rem, centred; gutters `--s4` on phones, `--s6` from 768 px.
- Prose pages (privacy, sources, bot) use a 42rem reading column with a sticky contents rail on
  wide screens.
- App screens are left-aligned: the page head (h1, one line of context, the page's one action)
  sits above the content, never centred.
- Gaps: inside a group `--s2`/`--s3`, between groups `--s5`, between sections `--s8`.
- Breakpoints: 760 px (phone to tablet), 1080 px (two-column app layouts).

## Shape and depth

- Radius: `--r-sm` 4px for controls, chips and bars (the logo's bar corners); `--r-md` 10px for
  cards and panels; `--r-full` for the avatar and the toggle only. One radius per nesting level.
- Depth: at rest nothing has a shadow; cards are a surface with a 1px line. Only floating things
  (the filter sheet on phones, toasts, menus) get `--shadow-float: 0 12px 32px rgb(13 18 38 / 0.16)`.
  Dark mode uses a lighter line instead of shadow.

## Motion

Meaningful and small. Every rule below has a `prefers-reduced-motion: reduce` fallback that keeps
the content and drops the travel.

| Token | Value | Use |
|---|---|---|
| `--m-fast` | 120ms | Press feedback (scale 0.98), hover colour |
| `--m-base` | 200ms | State changes, route change fade |
| `--m-slow` | 320ms | List reveal, parsing stage advance |
| `--ease` | `cubic-bezier(0.2, 0.8, 0.2, 1)` | Everything (exponential ease-out) |

- Route change: the main region fades from 0.6 and rises 4px.
- Match list: the first eight cards reveal with a 30ms stagger, once per load.
- Fit bars fill bottom-up once when a card first appears (transform only).
- Only `transform` and `opacity` animate. Never `transition: all`.

## Components

**Buttons.** Height 44px (40px compact). `primary` (one per screen), `secondary` (surface, 1px
line), `quiet` (text only, underline on hover), `danger` (outline in danger, fills on confirm).
States: hover, active (scale 0.98), focus-visible (2px `--focus` ring, 2px offset), disabled, and
busy (label ends in an ellipsis, `aria-busy`). Labels name the action: "Upload resume", not
"Submit".

**Inputs.** 44px, `--surface`, 1px `--control` edge, label above, help text below tied with
`aria-describedby`, error text in `--danger` below with the field's edge in `--danger`.
Checkboxes 20px with the label as one hit area.

**Chips.** Skills only. Have: `--have-bg` fill, `--have-ink` text, a citation (`L12`) when known.
Missing: no fill, 1px dashed `--line-strong`, `--miss-ink` text. Never colour alone: missing chips
also read "missing" to screen readers.

**Fit band.** Three 6px bars rising 8/12/16px, `--r-sm` corners, beside the label "Strong fit",
"Good fit" or "Stretch". Unlit bars are `--line-strong`.

**Match card.** A surface with a 1px line. Row 1: title (link, `--t-xl`) and the fit band at the
right. Row 2: facts (company, place, age; stale ones in the warn tone), source shown with its
required attribution. Row 3: skills you have, then missing. Footer: Save, and Apply at the source.
On phones the band moves under the title. Cards are a list (`<ol>`, ranked), not a grid.

**Tables.** Evidence is a two-column table: skill | where your resume shows it. Long resume lines
clamp to two lines with a "Show all" button. Header row in `--sunk`, rows separated by `--line`.
On phones each row stacks.

**Contents bar.** The job page's three tailoring outputs (Evidence, The job's words, What to put
first) stay on one page as labelled regions, with a row of jump links above them; each is
reachable by URL hash. Tabs were considered and dropped: they hide two of the three outputs.

**Toasts.** Bottom centre, `aria-live="polite"`, auto-dismiss after 4s except errors; one at a time.
Used for optimistic actions (saved, status changed) with Undo where it is safe.

**Skeletons.** The shape of the real content (a card's title bar, facts, chips) in `--sunk`, no
shimmer under reduced motion. Every data view shows one while loading.

**Progress.** Resume parsing shows four named stages, Uploading, Reading, Understanding, Done, as a
list whose current stage is marked with `aria-current="step"` and announced politely.

**First-run steps.** Sign in, Upload, Confirm, Matches: a short ordered list in the page head on
the resume and matches screens until the profile is confirmed.

**Empty and error states.** A heading that says what is true, one sentence that says why, one
action that fixes it. Errors never apologise and never blame the user.

## Pages

| Page | Composition |
|---|---|
| Landing | Left: the h1, one sentence, Sign in. Right: a real match card built from the components, its evidence visible. Below: how it works as an ordered list beside a sources band, then the promise (what is never invented). |
| Sign in | One panel: the h1, the 18+ box, two provider buttons, what is kept; beside it on wide screens, the three facts about data. |
| Resume | Upload drop zone with the staged progress; review as an editable profile with each value's cited line; one Confirm action. |
| Matches | Page head with refresh time; filter bar (a sheet on phones); ranked list of match cards. |
| Job / Bring a JD | Head with the band and Apply; a contents bar over the evidence table, wording swaps (before and after) and what to put first (a numbered list). |
| Tracker | Columns by status on wide screens, a list grouped by status on phones. |
| Account | Preferences form; export; deletion in its own danger panel with a confirm step. |
| Privacy, Sources, About the bot | Reading column, contents rail, plain headings. |

## SEO for the public pages

Landing, `/sources`, `/about/bot` and `/privacy` are prerendered to static HTML at build time,
each with a unique title, meta description, canonical URL, Open Graph and Twitter card (image
generated from the logo), and Organization and WebSite structured data. Every other route is
`noindex` and gets no structured data; job pages never emit `JobPosting`.
