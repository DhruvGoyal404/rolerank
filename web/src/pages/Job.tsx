import { useEffect, useState } from "react";
import { Link, Navigate, useLocation, useParams } from "react-router";
import { api, ApiError, type Explanation, type JobCard, type Tailoring } from "../api";
import { ApplyLink, Band, Facts, Loading, SkillSummary, TailoringView, Via } from "../components";
import { useSession } from "../session";
import { SaveButton } from "./Tracker";

/** An excerpt cut mid-sentence ends at its last full sentence instead, when it has one. */
export function tidyExcerpt(text: string): string {
  const t = text.trim();
  if (!/(\.\.\.|\u2026)$/.test(t)) return t;
  const end = Math.max(t.lastIndexOf(". "), t.lastIndexOf("! "), t.lastIndexOf("? "));
  return end > 40 ? t.slice(0, end + 1) : t;
}

export function Job() {
  const { key = "" } = useParams();
  const { me, loading } = useSession();
  const fromList = (useLocation().state as { band?: string } | null)?.band;
  const [job, setJob] = useState<{ job: JobCard; band?: string | null; explanation: Explanation | null } | null>(null);
  const [missing, setMissing] = useState(false);
  const [prose, setProse] = useState<{ text: string | null; outcome: string } | null>(null);
  const [tailoring, setTailoring] = useState<Tailoring | null>(null);
  const [busy, setBusy] = useState<"" | "explain" | "tailor">("");
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    api<{ job: JobCard; band: string | null; explanation: Explanation | null }>(`/api/jobs/${key}`).then(setJob, () => { setMissing(true); });
  }, [key]);
  if (loading) return <Loading what="your account" />;
  if (!me) return <Navigate to="/signin" replace />;
  if (missing)
    return (
      <div className="notfound">
        <h1>Job not found</h1>
        <p className="lede">This posting isn&apos;t in RoleRank any more: it may have closed, or its employer asked to be removed.</p>
        <Link className="button primary" to="/matches">
          Back to your matches
        </Link>
      </div>
    );
  if (!job) return <Loading what="the job" />;
  // The server's band, scored for this user; the list's, if the server couldn't answer.
  const band = job.band ?? fromList;

  const run = async (what: "explain" | "tailor") => {
    setBusy(what);
    setError(null);
    try {
      if (what === "explain") {
        const r = await api<{ prose: string | null; proseOutcome: string }>(`/api/jobs/${key}/explain`, { method: "POST" });
        setProse({ text: r.prose, outcome: r.proseOutcome });
      } else setTailoring(await api<Tailoring>(`/api/jobs/${key}/tailor`, { method: "POST" }));
    } catch (e) {
      setError(e instanceof ApiError && e.status === 429 ? "You've used today's allowance for this. Please come back tomorrow." : "That didn't work. Please try again.");
    } finally {
      setBusy("");
    }
  };

  return (
    <article className="job">
      <header className="job-head">
        {band ? <Band band={band} large /> : null}
        <h1>{job.job.title}</h1>
        <Facts job={job.job} />
        <div className="actions">
          <ApplyLink job={job.job} />
          <SaveButton jobKey={job.job.key} compact={false} />
          <Via job={job.job} />
        </div>
        {job.job.excerpt ? <p className="excerpt">{tidyExcerpt(job.job.excerpt)}</p> : null}
      </header>
      {job.explanation ? (
        <section className="panel fit-panel" aria-labelledby="fit">
          <h2 id="fit">How you fit</h2>
          <SkillSummary e={job.explanation} />
          {prose?.text ? <p className="prose">{prose.text}</p> : null}
          {prose && !prose.text ? <p className="fine">The skill lists above say it all for now.</p> : null}
          {!prose ? (
            <div>
              <button type="button" disabled={busy !== ""} aria-busy={busy === "explain"} onClick={() => void run("explain")}>
                {busy === "explain" ? "Writing…" : "Explain this match in words"}
              </button>
            </div>
          ) : null}
        </section>
      ) : (
        <p className="notice">Confirm your profile to see how you fit.</p>
      )}
      <section className="section" aria-labelledby="tailor">
        <h2 id="tailor">Tailor your resume for this job</h2>
        <p className="lede">Built only from your own resume lines: where each skill shows, the job&apos;s words for skills you have, and what to put first.</p>
        {tailoring ? (
          <TailoringView t={tailoring} />
        ) : (
          <button type="button" disabled={busy !== ""} aria-busy={busy === "tailor"} onClick={() => void run("tailor")}>
            {busy === "tailor" ? "Working…" : "Tailor my resume"}
          </button>
        )}
      </section>
      {error ? (
        <p className="error" role="alert">
          {error}
        </p>
      ) : null}
    </article>
  );
}
