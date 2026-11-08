import { useEffect, useMemo, useState } from "react";
import { Link, Navigate } from "react-router";
import { api, ApiError, type Match } from "../api";
import { Band, JobHeading, Loading, SkillSummary, Via } from "../components";
import { useSession } from "../session";
import { SaveButton } from "./Tracker";

export interface Filters {
  level: string; // "" for any
  internship: boolean;
  location: string;
  company: string;
  days: number; // 0 for any age
}

export const NO_FILTERS: Filters = { level: "", internship: false, location: "", company: "", days: 0 };

/** Filters run on the ranked list: they narrow it, and never reorder it. */
export function applyFilters(ms: Match[], f: Filters, now = Date.now()): Match[] {
  const has = (hay: string | null, needle: string) => needle.trim() === "" || (hay ?? "").toLowerCase().includes(needle.trim().toLowerCase());
  return ms.filter(
    (m) =>
      (f.level === "" || m.job.level === f.level) &&
      (!f.internship || m.job.level === "intern" || /\bintern(ship)?\b/i.test(m.job.title)) &&
      has(m.job.location, f.location) &&
      has(m.job.company, f.company) &&
      (f.days === 0 || now - Date.parse(m.job.postedAt) <= f.days * 86_400_000),
  );
}

/** "today", "3 days ago": how fresh a posting is. */
export function age(postedAt: string, now = Date.now()): string {
  const days = Math.floor((now - Date.parse(postedAt)) / 86_400_000);
  if (!Number.isFinite(days) || days < 1) return "posted today";
  return days === 1 ? "posted yesterday" : `posted ${String(days)} days ago`;
}

export function Matches() {
  const { me, loading } = useSession();
  const [remote, setRemote] = useState<boolean | null>(null); // null: use saved preferences
  const [all, setAll] = useState<Match[] | null>(null);
  const [filters, setFilters] = useState<Filters>(NO_FILTERS);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    if (!me?.profile?.confirmed) return;
    setAll(null);
    setError(null);
    const q = remote === null ? "" : `&remote=${remote ? "1" : "0"}`;
    api<{ matches: Match[]; remote: boolean }>(`/api/matches?limit=50${q}`).then(
      (r) => {
        setAll(r.matches);
        if (remote === null) setRemote(r.remote);
      },
      (e: unknown) => { setError(e instanceof ApiError && e.code === "search_unavailable" ? "Search is resting right now. Please try again shortly." : "We couldn't load matches."); },
    );
  }, [me, remote]);
  const shown = useMemo(() => (all ? applyFilters(all, filters) : null), [all, filters]);
  if (loading) return <Loading what="your account" />;
  if (!me) return <Navigate to="/signin" replace />;
  if (!me.profile?.confirmed) return <Navigate to="/resume" replace />;
  const set = (patch: Partial<Filters>) => { setFilters({ ...filters, ...patch }); };

  return (
    <>
      <p className="eyebrow">Matches</p>
      <h1>Roles that fit your resume</h1>
      <p className="hint">
        Ranked by how well your confirmed profile fits. <Link to="/jd">Found a job somewhere else? Bring its description.</Link>
      </p>
      <form className="card filters" aria-label="Filter matches" onSubmit={(e) => { e.preventDefault(); }}>
        <div className="filter-grid">
          <label className="check">
            <input type="checkbox" checked={remote ?? true} onChange={(e) => { setRemote(e.target.checked); }} />
            Remote roles open to India
          </label>
          <label className="check">
            <input type="checkbox" checked={filters.internship} onChange={(e) => { set({ internship: e.target.checked }); }} />
            Internships only
          </label>
          <div>
            <label htmlFor="f-level">Level</label>
            <select id="f-level" value={filters.level} onChange={(e) => { set({ level: e.target.value }); }}>
              <option value="">Any</option>
              <option value="intern">Intern</option>
              <option value="entry">Entry</option>
              <option value="mid">Mid</option>
              <option value="unknown">Not stated</option>
            </select>
          </div>
          <div>
            <label htmlFor="f-days">Posted</label>
            <select id="f-days" value={filters.days} onChange={(e) => { set({ days: Number(e.target.value) }); }}>
              <option value={0}>Any time</option>
              <option value={7}>In the last week</option>
              <option value={30}>In the last month</option>
            </select>
          </div>
          <div>
            <label htmlFor="f-location">Location</label>
            <input id="f-location" value={filters.location} placeholder="Bengaluru" onChange={(e) => { set({ location: e.target.value }); }} />
          </div>
          <div>
            <label htmlFor="f-company">Company</label>
            <input id="f-company" value={filters.company} onChange={(e) => { set({ company: e.target.value }); }} />
          </div>
        </div>
      </form>
      {error ? (
        <p className="error" role="alert">
          {error}
        </p>
      ) : null}
      {!shown && !error ? <Loading what="matches" /> : null}
      {shown ? (
        <p className="hint" role="status">
          {shown.length === all?.length ? `${String(shown.length)} matches` : `${String(shown.length)} of ${String(all?.length ?? 0)} matches`}
        </p>
      ) : null}
      {shown?.length === 0 ? <p className="notice">Nothing fits these filters. Try widening them; new postings arrive every day.</p> : null}
      {shown && shown.length > 0 ? (
        <ul className="match-list">
          {shown.map((m) => (
            <li className="card match" key={m.job.key}>
              <JobHeading job={m.job} link />
              <Band band={m.band} />
              <SkillSummary e={m.explanation} />
              <div className="row">
                <SaveButton jobKey={m.job.key} />
                <Via job={m.job} />
                <span className="via">{age(m.job.postedAt)}</span>
              </div>
            </li>
          ))}
        </ul>
      ) : null}
    </>
  );
}
