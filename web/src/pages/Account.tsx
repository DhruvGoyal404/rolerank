import { useEffect, useState, type SyntheticEvent } from "react";
import { Navigate } from "react-router";
import { api } from "../api";
import { Loading } from "../components";
import { useSession } from "../session";

interface Preferences {
  country: string;
  remote: boolean;
  targetTitles: string;
}

/** Preferences, export and deletion (SPEC.md, Account). */
export function Account() {
  const { me, loading, clear } = useSession();
  const [deleted, setDeleted] = useState(false);
  const [prefs, setPrefs] = useState<Preferences | null>(null);
  const [saved, setSaved] = useState(false);
  const [confirmText, setConfirmText] = useState("");
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    if (me) api<Preferences>("/api/preferences").then(setPrefs, () => { setError("We couldn't load your preferences."); });
  }, [me]);
  if (deleted) return <Navigate to="/" replace />;
  if (loading) return <Loading what="your account" />;
  if (!me) return <Navigate to="/signin" replace />;

  const save = async (event: SyntheticEvent) => {
    event.preventDefault();
    if (!prefs) return;
    setSaved(false);
    try {
      setPrefs(await api<Preferences>("/api/preferences", { method: "PUT", body: prefs }));
      setSaved(true);
    } catch {
      setError("Saving failed. Please try again.");
    }
  };
  const remove = async () => {
    try {
      await api("/api/me", { method: "DELETE" });
      // One render: the page goes home, and home sees no session.
      setDeleted(true);
      clear();
    } catch {
      setError("Deleting failed. Please try again.");
    }
  };

  return (
    <div className="narrow stack">
      <p className="eyebrow">Account</p>
      <h1>Your account</h1>
      {error ? (
        <p className="error" role="alert">
          {error}
        </p>
      ) : null}
      <section className="card" aria-labelledby="prefs">
        <h2 id="prefs">What you&apos;re looking for</h2>
        {!prefs ? (
          <Loading what="preferences" />
        ) : (
          <form onSubmit={(e) => void save(e)}>
            <label htmlFor="country">Where you can work on site</label>
            <select id="country" value={prefs.country} onChange={(e) => { setPrefs({ ...prefs, country: e.target.value }); }}>
              <option value="IN">India</option>
            </select>
            <label className="check">
              <input type="checkbox" checked={prefs.remote} onChange={(e) => { setPrefs({ ...prefs, remote: e.target.checked }); }} />
              Also show remote roles open to India
            </label>
            <label htmlFor="titles">Target roles</label>
            <input
              id="titles"
              placeholder="Backend Engineer, Data Analyst"
              maxLength={200}
              value={prefs.targetTitles}
              onChange={(e) => { setPrefs({ ...prefs, targetTitles: e.target.value }); }}
            />
            <p className="hint">Matches lean towards these titles. Leave blank to use the roles on your resume.</p>
            <div className="row">
              <button className="primary" type="submit">
                Save preferences
              </button>
              {saved ? (
                <span role="status" className="hint">
                  Saved.
                </span>
              ) : null}
            </div>
          </form>
        )}
      </section>
      <section className="card" aria-labelledby="export">
        <h2 id="export">Your data</h2>
        <p>Download everything RoleRank keeps about you as one JSON file: your account, resume text, profiles, tracker and AI usage.</p>
        <a className="button" href="/api/me/export" download="rolerank-export.json">
          Export my data
        </a>
      </section>
      <section className="card" aria-labelledby="delete">
        <h2 id="delete">Delete your account</h2>
        <p>This removes, at once and for good:</p>
        <ul>
          <li>your account and sign-in links;</li>
          <li>your resume text, contact details and every profile;</li>
          <li>your tracker, preferences and the encryption key for your data.</li>
        </ul>
        <p className="hint">
          Backups aren&apos;t running yet. When they start, the <a href="/privacy">privacy page</a> will say how long a deleted account can
          stay in one.
        </p>
        <label htmlFor="confirm-delete">Type DELETE to confirm</label>
        <input id="confirm-delete" value={confirmText} onChange={(e) => { setConfirmText(e.target.value); }} autoComplete="off" />
        <p>
          <button type="button" disabled={confirmText !== "DELETE"} onClick={() => void remove()}>
            Delete my account
          </button>
        </p>
      </section>
    </div>
  );
}
