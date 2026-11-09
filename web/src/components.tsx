import { Link } from "react-router";
import type { Explanation, JobCard, Tailoring } from "./api";
import { applyHref, ruleFor } from "./attribution";

/** The fit band as the logo's three bars. A band, never a percentage (CLAUDE.md). */
export function Band({ band }: { band: string }) {
  const name = band === "Strong" || band === "Good" || band === "Stretch" ? band : "Stretch";
  return (
    <span className={`band band-${name.toLowerCase()}`}>
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

/** Skills had and missing as chips: missing ones are outlined and muted, calm but distinct. */
export function SkillSummary({ e, compact }: { e: Explanation; compact?: boolean }) {
  return (
    <div className={compact ? "skills skills-compact" : "skills"}>
      {e.matched.length > 0 ? (
        <ul className="chips" aria-label="Required skills you have">
          {e.matched.map((s) => (
            <li className="chip have" key={s}>
              {s}
            </li>
          ))}
        </ul>
      ) : null}
      {e.missing.length > 0 ? (
        <ul className="chips" aria-label="Required skills your resume doesn't show">
          {(compact ? e.missing.slice(0, MISSING_ON_CARDS) : e.missing).map((s) => (
            <li className="chip missing" key={s}>
              {s}
            </li>
          ))}
          {compact && e.missing.length > MISSING_ON_CARDS ? <li className="chip more">{`+${String(e.missing.length - MISSING_ON_CARDS)} more`}</li> : null}
        </ul>
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

export function ApplyLink({ job }: { job: JobCard }) {
  return (
    <a className="button primary" href={applyHref(job)} rel={ruleFor(job.source).rel} target="_blank">
      Apply on the employer&apos;s site
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

export function JobTitle({ job }: { job: JobCard }) {
  return (
    <h3 className="job-title">
      <Link to={`/jobs/${job.key}`}>{job.title}</Link>
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
export function TailoringView({ t }: { t: Tailoring }) {
  const nothing = t.evidence.length === 0 && t.alignments.length === 0 && t.emphasis.top.length === 0;
  return (
    <div className="tailoring">
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
                      <q key={l.n}>{l.text.replace(/^\s*[-–—•*]\s*/, "")}</q>
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
                <span className="swap-terms">
                  <del>{a.from}</del>
                  <span aria-hidden="true"> to </span>
                  <ins>{a.to}</ins>
                </span>
                <q>{a.rewrite.replace(/^\s*[-–—•*]\s*/, "")}</q>
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
              <li key={l.n}>{l.text.replace(/^\s*[-–—•*]\s*/, "")}</li>
            ))}
          </ol>
          {t.emphasis.addMetric.length > 0 ? (
            <>
              <h4>Add a number, if you have one</h4>
              <p className="fine">Users, time saved, scale or accuracy: only if it&apos;s true. RoleRank never suggests a number.</p>
              <ul className="plain">
                {t.emphasis.addMetric.map((l) => (
                  <li key={l.n}>
                    <q>{l.text.replace(/^\s*[-–—•*]\s*/, "")}</q>
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

export function Loading({ what }: { what: string }) {
  return (
    <p className="fine" role="status">
      Loading {what}…
    </p>
  );
}

/** Placeholder cards while matches load, so the page doesn't jump when they arrive. */
export function MatchSkeleton() {
  return (
    <ul className="match-list" aria-hidden="true">
      {[0, 1, 2].map((i) => (
        <li className="match skeleton" key={i}>
          <span className="sk sk-title" />
          <span className="sk sk-line" />
          <span className="sk sk-chips" />
        </li>
      ))}
    </ul>
  );
}
