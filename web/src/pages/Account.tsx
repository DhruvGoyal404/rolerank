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
    <div className="account-grid">
      <header className="page-head">
        <h1>Your account</h1>
        <p className="lede">What RoleRank searches for, your data, and how to delete it.</p>
      </header>
      {error ? (
        <p className="error" role="alert">
          {error}
        </p>
      ) : null}
      <section className="panel" aria-labelledby="prefs">
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
            <p className="fine">Matches lean towards these titles. Leave blank to use the roles on your resume.</p>
            <div className="row">
              <button className="primary" type="submit">
                Save preferences
              </button>
              {saved ? (
                <span role="status" className="fine">
                  Saved.
                </span>
              ) : null}
            </div>
          </form>
        )}
      </section>
      <section className="panel" aria-labelledby="export">
        <h2 id="export">Your data</h2>
        <p>Download everything RoleRank keeps about you as one JSON file: your account, resume text, profiles, tracker and AI usage.</p>
        <p className="hint">
          <a className="button" href="/api/me/export" download="rolerank-export.json">
          Export my data
          </a>
        </p>
      </section>
      <section className="panel danger-zone" aria-labelledby="delete">
        <h2 id="delete">Delete your account</h2>
        <p>This removes, at once and for good:</p>
        <ul>
          <li>your account and sign-in links;</li>
          <li>your resume text, contact details and every profile;</li>
          <li>your tracker, preferences and the encryption key for your data.</li>
        </ul>
        <p className="fine">
          Encrypted backups can hold a deleted account for a while, and every restore deletes it again first. The{" "}
          <a href="/privacy#delete">privacy page</a> says for how long.
        </p>
        <label htmlFor="confirm-delete">Type DELETE to confirm</label>
        <input
          id="confirm-delete"
          value={confirmText}
          onChange={(e) => { setConfirmText(e.target.value); }}
          autoComplete="off"
          spellCheck={false}
          autoCapitalize="characters"
        />
        <p className="hint">
          <button type="button" className="danger" disabled={confirmText !== "DELETE"} onClick={() => void remove()}>
            Delete my account
          </button>
        </p>
      </section>
    </div>
  );
}
