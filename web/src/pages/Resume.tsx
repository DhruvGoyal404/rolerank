import { useEffect, useState, type SyntheticEvent } from "react";
import { Navigate, useNavigate } from "react-router";
import { api, ApiError, type Profile } from "../api";
import { Loading } from "../components";
import { useSession } from "../session";

const MAX_BYTES = 5 * 1024 * 1024;
// Each says which check failed, in plain words, and what to do (walkthrough, 2026-11-08).
const ERRORS: Record<string, string> = {
  too_large: "File too large: the limit is 5 MB. Export a smaller PDF and try again.",
  pdf_or_docx_only: "Not a PDF or a Word file: the file's contents aren't a PDF or a .docx, whatever its name says.",
  rate_limited: "Too many uploads for now. Please try again in an hour.",
};
/** Why a file couldn't be read: the codes aiworker records, with what to do instead. */
export const READ_ERRORS: Record<string, string> = {
  no_text: "The PDF has no selectable text: it looks like a scanned image. Upload the PDF your word processor exports, or a .docx.",
  password_protected: "The PDF is password-protected. Remove the password and upload it again.",
  too_many_pages: "The file has too many pages to be a resume. A resume of a few pages works best.",
  rejected: "The file failed our safety checks: its structure looks unusual, so we didn't open it. Export it again and retry.",
  unreadable: "The file opened, but its text couldn't be read. Export it again as PDF or .docx and retry.",
  extract_startup: "Our file reader was slow to start, not your file. Please upload it again.",
  extract_timeout: "Reading this file took longer than our limit. Export a simpler PDF (fewer images or pages) and retry.",
  extract_too_much_text: "The file holds far more text than a resume. Upload just your resume.",
  extract_failed: "Our file reader failed on our side, not because of your file. Please upload it again.",
  file_missing: "The upload didn't arrive in full. Please upload it again.",
  internal: "Something went wrong on our side while reading it. Please try again.",
  parse_invalid: "We read the text but couldn't build your profile from it. Please try again.",
  parse_error: "We read the text but couldn't build your profile from it. Please try again.",
  parse_refused: "We read the text but couldn't build your profile from it. Please try again.",
};
/** A code without its own message still names itself, so it can be reported. */
export const readError = (code: string | null | undefined): string =>
  READ_ERRORS[code ?? ""] ?? `We couldn't read that file (code: ${code ?? "unknown"}). Please try again.`;

/** Upload, wait for the parse, then review and confirm. */
export function Resume() {
  const { me, loading, refresh } = useSession();
  const [replacing, setReplacing] = useState(false);
  if (loading) return <Loading what="your account" />;
  if (!me) return <Navigate to="/signin" replace />;
  const busy = me.resume && ["queued", "extracting", "parsing"].includes(me.resume.status);
  return (
    <div className="narrow">
      <p className="eyebrow">Your resume</p>
      {busy ? (
        <Waiting resumeId={me.resume?.id ?? ""} onDone={refresh} />
      ) : me.profile && !replacing ? (
        <Review profileId={me.profile.id} confirmed={me.profile.confirmed} onConfirmed={refresh} onReplace={() => { setReplacing(true); }} />
      ) : (
        <Upload
          failed={me.resume?.status === "failed" ? readError(me.resume.error) : null}
          onUploaded={async () => {
            setReplacing(false);
            await refresh();
          }}
        />
      )}
    </div>
  );
}

function Upload({ failed, onUploaded }: { failed: string | null; onUploaded: () => Promise<void> }) {
  const [error, setError] = useState<string | null>(failed);
  const [sending, setSending] = useState(false);
  const submit = async (event: SyntheticEvent<HTMLFormElement>) => {
    event.preventDefault();
    const file = new FormData(event.currentTarget).get("resume");
    if (!(file instanceof File) || file.size === 0) { setError("Choose a file first."); return; }
    if (file.size > MAX_BYTES) { setError(ERRORS.too_large ?? ""); return; }
    setSending(true);
    setError(null);
    try {
      await api("/api/resumes", { method: "POST", raw: file });
      await onUploaded();
    } catch (e) {
      setError(e instanceof ApiError ? (ERRORS[e.code] ?? `Upload failed (code: ${e.code}). Please try again.`) : "Upload failed: we couldn't reach RoleRank. Check your connection and retry.");
    } finally {
      setSending(false);
    }
  };
  return (
    <>
      <h1>Upload your resume</h1>
      <p>
        We read it in a sandbox with no network, strip your contact details before anything reaches our AI model, and store the text
        encrypted with a key of your own. The original file is deleted once it&apos;s read.
      </p>
      <form className="drop" onSubmit={(e) => void submit(e)}>
        <label htmlFor="resume">PDF or Word (.docx), up to 5 MB</label>
        <input id="resume" name="resume" type="file" accept=".pdf,.docx,application/pdf,application/vnd.openxmlformats-officedocument.wordprocessingml.document" />
        <p>
          <button className="primary" type="submit" disabled={sending}>
            {sending ? "Uploading…" : "Upload"}
          </button>
        </p>
        {error ? (
          <p className="error" role="alert">
            {error}
          </p>
        ) : null}
      </form>
    </>
  );
}

function Waiting({ resumeId, onDone }: { resumeId: string; onDone: () => Promise<void> }) {
  const [status, setStatus] = useState("queued");
  useEffect(() => {
    let stop = false;
    const tick = async () => {
      if (stop) return;
      try {
        const r = await api<{ status: string }>(`/api/resumes/${resumeId}`);
        setStatus(r.status);
        if (r.status === "ready" || r.status === "failed") {
          await onDone();
          return;
        }
      } catch {
        // keep polling; a passing network error isn't the end of the parse
      }
      setTimeout(() => void tick(), 2000);
    };
    void tick();
    return () => {
      stop = true;
    };
  }, [resumeId, onDone]);
  const words: Record<string, string> = { queued: "Waiting in line", extracting: "Reading the file in the sandbox", parsing: "Reading your profile" };
  return (
    <>
      <h1>Reading your resume</h1>
      <p role="status" aria-live="polite">
        {words[status] ?? "Working"}… this usually takes under a minute.
      </p>
    </>
  );
}

const LEVELS: Array<[Profile["level"], string]> = [
  ["intern", "Student, looking for an internship"],
  ["entry", "Fresher or under a year of work"],
  ["mid", "About 2 to 4 years"],
  ["senior", "5 years or more"],
];

function Review(props: { profileId: string; confirmed: boolean; onConfirmed: () => Promise<void>; onReplace: () => void }) {
  const navigate = useNavigate();
  const [profile, setProfile] = useState<Profile | null>(null);
  const [months, setMonths] = useState(0);
  const [skill, setSkill] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  useEffect(() => {
    api<{ profile: Profile; months: number }>(`/api/profiles/${props.profileId}`).then(
      (r) => {
        setProfile(r.profile);
        setMonths(r.months);
      },
      () => { setError("We couldn't load your profile."); },
    );
  }, [props.profileId]);
  if (!profile) return error ? <p className="error">{error}</p> : <Loading what="your profile" />;

  const update = (p: Profile) => { setProfile(p); };
  const addSkill = () => {
    const name = skill.trim();
    if (name && !profile.skills.some((s) => s.name.toLowerCase() === name.toLowerCase())) update({ ...profile, skills: [...profile.skills, { name, line: 0 }] });
    setSkill("");
  };
  const confirm = async () => {
    setSaving(true);
    setError(null);
    try {
      await api(`/api/profiles/${props.profileId}`, { method: "PUT", body: { profile } });
      await api(`/api/profiles/${props.profileId}/confirm`, { method: "POST" });
      await props.onConfirmed();
      await navigate("/matches");
    } catch {
      setError("Saving failed. Please try again.");
    } finally {
      setSaving(false);
    }
  };

  if (props.confirmed)
    return (
      <>
        <h1>Your confirmed profile</h1>
        <ProfileSummary profile={profile} months={months} />
        <div className="row">
          <button className="primary" type="button" onClick={() => void navigate("/matches")}>
            See matches
          </button>
          <button type="button" onClick={props.onReplace}>
            Upload a new resume
          </button>
        </div>
      </>
    );

  return (
    <>
      <h1>Check what we read</h1>
      <p>Correct anything that&apos;s wrong, then confirm. Matching uses only what you confirm here.</p>
      <label htmlFor="level">Career stage</label>
      <select id="level" value={profile.level} onChange={(e) => { update({ ...profile, level: e.target.value as Profile["level"] }); }}>
        {LEVELS.map(([v, label]) => (
          <option key={v} value={v}>
            {label}
          </option>
        ))}
      </select>
      <h2>Skills</h2>
      <ul className="chips" aria-label="Your skills">
        {profile.skills.map((s) => (
          <li className="chip" key={s.name}>
            {s.name}
            <button type="button" aria-label={`Remove ${s.name}`} onClick={() => { update({ ...profile, skills: profile.skills.filter((x) => x !== s) }); }}>
              ×
            </button>
          </li>
        ))}
      </ul>
      <label htmlFor="add-skill">Add a skill your resume shows</label>
      <div className="row">
        <input
          id="add-skill"
          value={skill}
          onChange={(e) => { setSkill(e.target.value); }}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              addSkill();
            }
          }}
        />
        <button type="button" onClick={addSkill}>
          Add
        </button>
      </div>
      <ProfileSummary profile={profile} months={months} />
      {error ? (
        <p className="error" role="alert">
          {error}
        </p>
      ) : null}
      <div className="row">
        <button className="primary" type="button" disabled={saving} onClick={() => void confirm()}>
          {saving ? "Saving…" : "Confirm and see matches"}
        </button>
        <button type="button" onClick={props.onReplace}>
          Upload a different file
        </button>
      </div>
    </>
  );
}

function ProfileSummary({ profile, months }: { profile: Profile; months: number }) {
  return (
    <div className="stack">
      <h2>Experience</h2>
      {profile.experience.length === 0 ? <p className="hint">None found.</p> : null}
      {profile.experience.map((e) => (
        <div className="card" key={`${e.employer}-${e.title}`}>
          <h3>{e.title}</h3>
          <p className="meta">
            {e.employer} · {e.kind} · {e.periods.map((p) => `${p.start || "?"} to ${p.end || "?"}`).join(", ")}
          </p>
        </div>
      ))}
      <p className="hint">Work experience counted: {months} months (jobs and internships; programmes don&apos;t count).</p>
      {profile.education.length > 0 ? (
        <>
          <h2>Education</h2>
          {profile.education.map((e) => (
            <p key={`${e.institution}-${e.degree}`}>
              {e.degree}, {e.institution}
              {e.grad_year ? `, ${String(e.grad_year)}` : ""}
              {e.score ? ` · ${e.score}` : ""}
            </p>
          ))}
        </>
      ) : null}
    </div>
  );
}
