import { useEffect, useState } from "react";
import { Navigate, useParams } from "react-router";
import { api, ApiError, type Explanation, type JobCard, type Tailoring } from "../api";
import { ApplyLink, JobHeading, Loading, SkillSummary, TailoringView, Via } from "../components";
import { useSession } from "../session";
import { SaveButton } from "./Tracker";

export function Job() {
  const { key = "" } = useParams();
  const { me, loading } = useSession();
  const [job, setJob] = useState<{ job: JobCard; explanation: Explanation | null } | null>(null);
  const [missing, setMissing] = useState(false);
  const [prose, setProse] = useState<{ text: string | null; outcome: string } | null>(null);
  const [tailoring, setTailoring] = useState<Tailoring | null>(null);
  const [busy, setBusy] = useState<"" | "explain" | "tailor">("");
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    api<{ job: JobCard; explanation: Explanation | null }>(`/api/jobs/${key}`).then(setJob, () => { setMissing(true); });
  }, [key]);
  if (loading) return <Loading what="your account" />;
  if (!me) return <Navigate to="/signin" replace />;
  if (missing) return <h1>Job not found</h1>;
  if (!job) return <Loading what="the job" />;

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
    <div className="stack">
      <p className="eyebrow">Job</p>
      <h1>{job.job.title}</h1>
      <JobHeading job={{ ...job.job, title: "" }} />
      <div className="row">
        <ApplyLink job={job.job} />
        <SaveButton jobKey={job.job.key} />
        <Via job={job.job} />
      </div>
      {job.job.excerpt ? <p className="hint">{job.job.excerpt}</p> : null}
      {job.explanation ? (
        <section className="card" aria-labelledby="fit">
          <h2 id="fit">How you fit</h2>
          <SkillSummary e={job.explanation} />
          {prose?.text ? <p>{prose.text}</p> : null}
          {prose && !prose.text ? <p className="hint">The skill lists above say it all for now.</p> : null}
          {!prose ? (
            <button type="button" disabled={busy !== ""} onClick={() => void run("explain")}>
              {busy === "explain" ? "Writing…" : "Explain this match in words"}
            </button>
          ) : null}
        </section>
      ) : (
        <p className="notice">Confirm your profile to see how you fit.</p>
      )}
      <section aria-labelledby="tailor">
        <h2 id="tailor">Tailor your resume for this job</h2>
        <p className="hint">Built only from your own resume lines: where each skill shows, the job&apos;s words for skills you have, and what to put first.</p>
        {tailoring ? (
          <TailoringView t={tailoring} />
        ) : (
          <button type="button" className="primary" disabled={busy !== ""} onClick={() => void run("tailor")}>
            {busy === "tailor" ? "Working…" : "Tailor my resume"}
          </button>
        )}
      </section>
      {error ? (
        <p className="error" role="alert">
          {error}
        </p>
      ) : null}
    </div>
  );
}
