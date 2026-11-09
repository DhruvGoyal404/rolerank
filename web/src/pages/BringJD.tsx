import { useEffect, useRef, useState, type SyntheticEvent } from "react";
import { Navigate } from "react-router";
import { api, ApiError, type Explanation, type Tailoring } from "../api";
import { Loading, SkillSummary, TailoringView } from "../components";
import { useSession } from "../session";

const MAX_BYTES = 20 * 1024;

/** Paste a job description: the same breakdown as an indexed job. Nothing pasted is stored. */
export function BringJD() {
  const { me, loading } = useSession();
  const [text, setText] = useState("");
  const [result, setResult] = useState<(Explanation & { title: string; required: string[] }) | null>(null);
  const [tailoring, setTailoring] = useState<Tailoring | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const resultHeading = useRef<HTMLHeadingElement>(null);
  // The result can land below the fold on a phone: bring it into view and give it focus.
  useEffect(() => {
    if (result) resultHeading.current?.focus();
  }, [result]);
  if (loading) return <Loading what="your account" />;
  if (!me) return <Navigate to="/signin" replace />;
  if (!me.profile?.confirmed) return <Navigate to="/resume" replace />;

  const bytes = new TextEncoder().encode(text).length;
  const analyse = async (event: SyntheticEvent) => {
    event.preventDefault();
    setBusy(true);
    setError(null);
    setTailoring(null);
    try {
      setResult(await api(`/api/jd`, { method: "POST", body: { text } }));
    } catch (e) {
      setError(e instanceof ApiError && e.status === 429 ? "You've used today's allowance. Please come back tomorrow." : "We couldn't read that. Please check the text and try again.");
    } finally {
      setBusy(false);
    }
  };
  const tailor = async () => {
    setBusy(true);
    setError(null);
    try {
      setTailoring(await api<Tailoring>(`/api/jd/tailor`, { method: "POST", body: { text } }));
    } catch (e) {
      setError(e instanceof ApiError && e.status === 429 ? "You've used today's allowance. Please come back tomorrow." : "That didn't work. Please try again.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="stack">
      <header className="page-head">
        <h1>How do you fit a job you found?</h1>
        <p className="lede">
          Paste a job description from anywhere: you get the same breakdown as a ranked job. It&apos;s read in memory, never stored or
          shared, and our AI model never sees its text.
        </p>
      </header>
      <form className="panel" onSubmit={(e) => void analyse(e)}>
        <label htmlFor="jd">Job description</label>
        <textarea
          id="jd"
          value={text}
          placeholder="Paste the whole description, including the requirements…"
          spellCheck={false}
          aria-invalid={bytes > MAX_BYTES ? true : undefined}
          onChange={(e) => { setText(e.target.value); }}
          aria-describedby="jd-size"
        />
        <p id="jd-size" className={bytes > MAX_BYTES ? "error" : "hint"}>
          {bytes > MAX_BYTES ? `${String(Math.ceil(bytes / 1024))} KB: over the 20 KB limit. Paste just the role's description.` : `${String(Math.ceil(bytes / 1024))} KB of 20 KB`}
        </p>
        <button className="primary" type="submit" aria-busy={busy && !result} disabled={busy || text.trim() === "" || bytes > MAX_BYTES}>
          {busy && !result ? "Reading…" : "See how I fit"}
        </button>
      </form>
      {error ? (
        <p className="error" role="alert">
          {error}
        </p>
      ) : null}
      {result ? (
        <section className="panel fit-panel" aria-labelledby="jd-fit">
          <h2 id="jd-fit" ref={resultHeading} tabIndex={-1}>
            {result.title || "This job"}
          </h2>
          {result.required.length === 0 ? <p className="notice">We couldn&apos;t find any skills we know in this text.</p> : <SkillSummary e={result} />}
        </section>
      ) : null}
      {result && result.required.length > 0 ? (
        tailoring ? (
          <TailoringView t={tailoring} />
        ) : (
          <button type="button" disabled={busy} onClick={() => void tailor()}>
            {busy ? "Working…" : "Tailor my resume for it"}
          </button>
        )
      ) : null}
    </div>
  );
}
