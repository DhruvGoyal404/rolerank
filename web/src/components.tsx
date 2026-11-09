import { useState } from "react";
import { Link } from "react-router";
import type { Explanation, JobCard, Tailoring } from "./api";
import { applyHref, ruleFor } from "./attribution";

/** The fit band as the logo's three bars. A band, never a percentage (CLAUDE.md). */
export function Band({ band, large }: { band: string; large?: boolean }) {
  const name = band === "Strong" || band === "Good" || band === "Stretch" ? band : "Stretch";
  return (
    <span className={`band band-${name.toLowerCase()}${large ? " band-large" : ""}`}>
      <span className="bars" aria-hidden="true">
        <i />
        <i />
        <i />
      </span>
      {`${name} fit`}
    </span>
  );
}

const LEVEL: Record<Explanation["level"]["fit"], string> = {
  fits: "Your level fits",
  stretch: "One level up",
  above: "Well above your level",
  unknown: "Level not stated",
};

/** On a card, the first few missing skills; the job page lists them all. */
const MISSING_ON_CARDS = 6;

/** Skills had and missing as chips in labelled rows: had ones filled, missing ones outlined. */
export function SkillSummary({ e, compact }: { e: Explanation; compact?: boolean }) {
  return (
    <div className={compact ? "skills skills-compact" : "skills"}>
      {e.matched.length > 0 ? (
        <div className="skill-row">
          <span className="label" aria-hidden="true">
            You have
          </span>
          <ul className="chips" aria-label="Required skills you have">
            {e.matched.map((s) => (
              <li className="chip have" key={s}>
                {s}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      {e.missing.length > 0 ? (
        <div className="skill-row">
          <span className="label" aria-hidden="true">
            Not shown
          </span>
          <ul className="chips" aria-label="Required skills your resume doesn't show">
            {(compact ? e.missing.slice(0, MISSING_ON_CARDS) : e.missing).map((s) => (
              <li className="chip missing" key={s}>
                {s}
              </li>
            ))}
            {compact && e.missing.length > MISSING_ON_CARDS ? <li className="chip more">{`+${String(e.missing.length - MISSING_ON_CARDS)} more`}</li> : null}
          </ul>
        </div>
      ) : null}
      {compact ? null : (
        <p className="fine">
          {LEVEL[e.level.fit]}
          {e.niceMatched.length > 0 ? `. Also nice to have, and you have: ${e.niceMatched.join(", ")}` : ""}
        </p>
      )}
    </div>
  );
}

/** Credit and links exactly as each source requires (attribution.ts). */
export function Via({ job }: { job: JobCard }) {
  const rule = ruleFor(job.source);
  return (
    <span className="via">
      via{" "}
      <a href={job.url} rel={rule.rel} target="_blank">
        {rule.credit}
      </a>
    </span>
  );
}

export function ApplyLink({ job, compact }: { job: JobCard; compact?: boolean }) {
  return (
    <a className={compact ? "button compact" : "button primary"} href={applyHref(job)} rel={ruleFor(job.source).rel} target="_blank">
      {compact ? "Apply" : "Apply on the employer's site"}
      <span className="sr-only"> (new tab)</span>
    </a>
  );
}

/** Postings older than this are marked: the role may already be filled. */
export const STALE_DAYS = 30;

export const daysSince = (at: string, now = Date.now()): number => Math.floor((now - Date.parse(at)) / 86_400_000);

/** "2 hours ago", "3 days ago": for the corpus refresh time. */
export function ago(at: string, now = Date.now()): string {
  const mins = Math.floor((now - Date.parse(at)) / 60_000);
  if (!Number.isFinite(mins)) return "at an unknown time";
  if (mins < 60) return mins <= 1 ? "just now" : `${String(mins)} minutes ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return hours === 1 ? "1 hour ago" : `${String(hours)} hours ago`;
  const days = Math.floor(hours / 24);
  return days === 1 ? "yesterday" : `${String(days)} days ago`;
}

/** "today", "3 days ago": how fresh a posting is. */
export function age(postedAt: string, now = Date.now()): string {
  const days = Math.floor((now - Date.parse(postedAt)) / 86_400_000);
  if (!Number.isFinite(days) || days < 1) return "posted today";
  return days === 1 ? "posted yesterday" : `posted ${String(days)} days ago`;
}

/**
 * A region-wide posting lists every country it accepts ("Afghanistan | Armenia | ... | Yemen",
 * found in the walkthrough, 2026-11-09). Past three places, name India (or the first) and count
 * the rest.
 */
export function place(location: string): string {
  const parts = location.split(" | ").map((p) => p.trim()).filter(Boolean);
  if (parts.length <= 3) return parts.join(", ");
  const lead = parts.find((p) => /^india$/i.test(p)) ?? parts[0] ?? "";
  return `${lead} and ${String(parts.length - 1)} other countries`;
}

/** Company, place and age as separate facts, not a dotted string. */
export function Facts({ job }: { job: JobCard }) {
  return (
    <ul className="facts">
      <li>{job.company}</li>
      {job.location ? <li>{place(job.location)}</li> : null}
      {daysSince(job.postedAt) > STALE_DAYS ? (
        <li className="stale">{age(job.postedAt)}: may already be filled</li>
      ) : (
        <li>{age(job.postedAt)}</li>
      )}
      {job.closed ? <li className="closed">Closed</li> : null}
    </ul>
  );
}

/** The job's title as a link; the band rides along so the job page can show it. */
export function JobTitle({ job, band }: { job: JobCard; band?: string }) {
  return (
    <h3 className="job-title">
      <Link to={`/jobs/${job.key}`} state={band ? { band } : undefined}>
        {job.title}
      </Link>
    </h3>
  );
}

/** Skills that cite exactly the same lines share one row: a skills list shouldn't repeat four times. */
export function byLines(evidence: Tailoring["evidence"]): Array<{ skills: string[]; lines: Tailoring["evidence"][number]["lines"] }> {
  const rows = new Map<string, { skills: string[]; lines: Tailoring["evidence"][number]["lines"] }>();
  for (const e of evidence) {
    const key = e.lines.map((l) => l.n).join(",");
    const row = rows.get(key);
    if (row) row.skills.push(e.skill);
    else rows.set(key, { skills: [e.skill], lines: e.lines });
  }
  return [...rows.values()];
}

/**
 * The three grounded outputs of tailoring, built to scan: a table from each job skill to the
 * resume line that shows it, swaps as before and after, and the bullets to lead with, in order.
 * Every line shown is the user's own, verbatim.
 */
/** A resume line, verbatim; long ones clamp to two lines with a toggle. */
export function ResumeLine({ text }: { text: string }) {
  const clean = text.replace(/^\s*[-\u2013\u2014\u2022*]\s*/, "");
  const [open, setOpen] = useState(false);
  const long = clean.length > 160;
  return (
    <div className="line">
      <q className={long && !open ? "clamp" : undefined}>{clean}</q>
      {long ? (
        <button
          type="button"
          className="quiet more-toggle"
          aria-expanded={open}
          onClick={() => {
            setOpen(!open);
          }}
        >
          {open ? "Show less" : "Show all"}
        </button>
      ) : null}
    </div>
  );
}

export function TailoringView({ t }: { t: Tailoring }) {
  const nothing = t.evidence.length === 0 && t.alignments.length === 0 && t.emphasis.top.length === 0;
  const parts = [
    t.evidence.length > 0 && ["ev", "Evidence"],
    t.alignments.length > 0 && ["al", "The job's words"],
    t.emphasis.top.length > 0 && ["em", "What to put first"],
  ].filter((x): x is [string, string] => Array.isArray(x));
  return (
    <div className="tailoring">
      {parts.length > 1 ? (
        <nav aria-label="Tailoring sections">
          <ul className="jump">
            {parts.map(([id, label]) => (
              <li key={id}>
                <a href={`#${id}`}>{label}</a>
              </li>
            ))}
          </ul>
        </nav>
      ) : null}
      {nothing ? <p className="notice">Your resume doesn&apos;t show what this job asks for, so there is nothing honest to suggest.</p> : null}
      {t.evidence.length > 0 ? (
        <section aria-labelledby="ev">
          <h3 id="ev">Where your resume shows each skill</h3>
          <table className="evidence-table">
            <thead>
              <tr>
                <th scope="col">Job asks for</th>
                <th scope="col">Your resume line</th>
              </tr>
            </thead>
            <tbody>
              {byLines(t.evidence).map((row) => (
                <tr key={row.skills.join()}>
                  <th scope="row">{row.skills.join(", ")}</th>
                  <td>
                    {row.lines.map((l) => (
                      <ResumeLine key={l.n} text={l.text} />
                    ))}
                  </td>
                </tr>
              ))}
              {t.notShown.length > 0 ? (
                <tr className="not-shown">
                  <th scope="row">Not shown in your resume</th>
                  <td>
                    <ul className="chips" aria-label="Skills the job asks for that your resume doesn't show">
                      {t.notShown.map((s) => (
                        <li className="chip missing" key={s}>
                          {s}
                        </li>
                      ))}
                    </ul>
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </section>
      ) : null}
      {t.alignments.length > 0 ? (
        <section aria-labelledby="al">
          <h3 id="al">Use the job&apos;s words</h3>
          <p className="fine">The same skill, named the way this job names it. Nothing else in the line changes.</p>
          <ul className="swaps">
            {t.alignments.map((a) => (
              <li key={`${String(a.line)}-${a.from}`}>
                <div className="before">
                  <span className="tag">Your resume says</span>
                  <del>{a.from}</del>
                </div>
                <div className="after">
                  <span className="tag">This job says</span>
                  <span>
                    <ins>{a.to}</ins>
                  </span>
                  <ResumeLine text={a.rewrite} />
                </div>
              </li>
            ))}
          </ul>
        </section>
      ) : null}
      {t.emphasis.top.length > 0 ? (
        <section aria-labelledby="em">
          <h3 id="em">Put these first</h3>
          <ol className="lead-with">
            {t.emphasis.top.map((l) => (
              <li key={l.n}>
                <ResumeLine text={l.text} />
              </li>
            ))}
          </ol>
          {t.emphasis.addMetric.length > 0 ? (
            <>
              <h4>Add a number, if you have one</h4>
              <p className="fine">Users, time saved, scale or accuracy: only if it&apos;s true. RoleRank never suggests a number.</p>
              <ul className="plain">
                {t.emphasis.addMetric.map((l) => (
                  <li key={l.n}>
                    <ResumeLine text={l.text} />
                  </li>
                ))}
              </ul>
            </>
          ) : null}
        </section>
      ) : null}
    </div>
  );
}

/** A loading state shaped like content, with the words for screen readers. */
export function Loading({ what }: { what: string }) {
  return (
    <div className="loading" role="status">
      <span className="sr-only">Loading {what}…</span>
      <span className="sk sk-title" aria-hidden="true" />
      <span className="sk sk-line" aria-hidden="true" />
      <span className="sk sk-block" aria-hidden="true" />
    </div>
  );
}

/** Placeholder cards while matches load, so the page doesn't jump when they arrive. */
export function MatchSkeleton() {
  return (
    <ul className="match-list" aria-hidden="true">
      {[0, 1, 2, 3].map((i) => (
        <li className="match skeleton" key={i}>
          <span className="sk sk-title" />
          <span className="sk sk-line" />
          <span className="sk sk-chips" />
        </li>
      ))}
    </ul>
  );
}

const JOURNEY = ["Sign in", "Upload your resume", "Confirm your profile", "See your matches"] as const;

/** The first-run journey, shown until the profile is confirmed. `at` is the current step, 0 to 3. */
export function Journey({ at }: { at: number }) {
  return (
    <ol className="journey" aria-label="Getting started">
      {JOURNEY.map((label, i) => (
        <li key={label} className={i < at ? "done" : undefined} aria-current={i === at ? "step" : undefined}>
          {label}
          {i < at ? <span className="sr-only"> (done)</span> : null}
        </li>
      ))}
    </ol>
  );
}
