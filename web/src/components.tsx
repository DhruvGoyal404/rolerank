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
  stretch: "One level up: a stretch",
  above: "Well above your level",
  unknown: "Level not stated",
};

export function SkillSummary({ e }: { e: Explanation }) {
  return (
    <div className="stack">
      {e.matched.length > 0 ? (
        <div>
          <p className="eyebrow">You have</p>
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
        <div>
          <p className="eyebrow">Your resume doesn&apos;t show</p>
          <ul className="chips" aria-label="Required skills your resume doesn't show">
            {e.missing.map((s) => (
              <li className="chip missing" key={s}>
                {s}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      {e.niceMatched.length > 0 ? <p className="hint">Nice to have, and you have: {e.niceMatched.join(", ")}</p> : null}
      <p className="hint">{LEVEL[e.level.fit]}</p>
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

export function JobHeading({ job, link }: { job: JobCard; link?: boolean }) {
  return (
    <div>
      <h3>{link ? <Link to={`/jobs/${job.key}`}>{job.title}</Link> : job.title}</h3>
      <p className="meta">
        {job.company}
        {job.location ? ` · ${job.location}` : ""}
        {job.closed ? " · closed" : ""}
      </p>
    </div>
  );
}

/** The three grounded outputs of tailoring. Every line shown is the user's own, verbatim. */
export function TailoringView({ t }: { t: Tailoring }) {
  const nothing = t.evidence.length === 0 && t.alignments.length === 0 && t.emphasis.top.length === 0;
  return (
    <div className="stack">
      {nothing ? <p className="notice">Your resume doesn&apos;t show what this job asks for. Nothing honest to suggest.</p> : null}
      {t.evidence.length > 0 || t.notShown.length > 0 ? (
        <section className="card" aria-labelledby="ev">
          <h3 id="ev">Where your resume shows each skill</h3>
          <ul className="evidence">
            {t.evidence.map((e) => (
              <li key={e.skill}>
                <strong>{e.skill}</strong>
                {e.lines.map((l) => (
                  <p className="quote" key={l.n}>
                    {l.text}
                  </p>
                ))}
              </li>
            ))}
            {t.notShown.map((s) => (
              <li key={s}>
                <strong>{s}</strong> <span className="hint">· not shown in your resume</span>
              </li>
            ))}
          </ul>
        </section>
      ) : null}
      {t.alignments.length > 0 ? (
        <section className="card" aria-labelledby="al">
          <h3 id="al">Use the job&apos;s words</h3>
          <p className="hint">The same skill, named the way this job names it. Nothing else in the line changes.</p>
          <ul className="evidence">
            {t.alignments.map((a) => (
              <li key={`${String(a.line)}-${a.from}`} className="swap">
                <del>{a.from}</del> → <ins>{a.to}</ins>
                <p className="quote">{a.rewrite}</p>
              </li>
            ))}
          </ul>
        </section>
      ) : null}
      {t.emphasis.top.length > 0 ? (
        <section className="card" aria-labelledby="em">
          <h3 id="em">Put these first</h3>
          <ol className="evidence">
            {t.emphasis.top.map((l) => (
              <li key={l.n}>
                <p className="quote">{l.text}</p>
              </li>
            ))}
          </ol>
          {t.emphasis.addMetric.length > 0 ? (
            <>
              <h3>Add a number, if you have one</h3>
              <p className="hint">Users, time saved, scale, accuracy: only if it&apos;s true. RoleRank never suggests a number.</p>
              {t.emphasis.addMetric.map((l) => (
                <p className="quote" key={l.n}>
                  {l.text}
                </p>
              ))}
            </>
          ) : null}
        </section>
      ) : null}
    </div>
  );
}

export function Loading({ what }: { what: string }) {
  return (
    <p className="hint" role="status">
      Loading {what}…
    </p>
  );
}
