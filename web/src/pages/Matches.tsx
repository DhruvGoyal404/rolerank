import { useEffect, useMemo, useState } from "react";
import { Link, Navigate } from "react-router";
import { api, ApiError, type Match } from "../api";
import { ago, Band, Facts, JobTitle, Loading, MatchSkeleton, SkillSummary, Via } from "../components";
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

const active = (f: Filters) => [f.level !== "", f.internship, f.location.trim() !== "", f.company.trim() !== "", f.days !== 0].filter(Boolean).length;

export function Matches() {
  const { me, loading } = useSession();
  const [remote, setRemote] = useState<boolean | null>(null); // null: use saved preferences
  const [all, setAll] = useState<Match[] | null>(null);
  const [refreshedAt, setRefreshedAt] = useState<string | null>(null);
  const [filters, setFilters] = useState<Filters>(NO_FILTERS);
  const [open, setOpen] = useState(false); // the filter sheet, on phones
  const [error, setError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    if (!me?.profile?.confirmed) return;
    setAll(null);
    setError(null);
    const q = remote === null ? "" : `&remote=${remote ? "1" : "0"}`;
    api<{ matches: Match[]; remote: boolean; refreshedAt: string | null }>(`/api/matches?limit=50${q}`).then(
      (r) => {
        setAll(r.matches);
        setRefreshedAt(r.refreshedAt);
        if (remote === null) setRemote(r.remote);
      },
      (e: unknown) => {
        setError(
          e instanceof ApiError && e.code === "search_unavailable"
            ? "Search isn't answering right now. Try again in a minute."
            : "Matches didn't load. Check your connection and try again.",
        );
      },
    );
  }, [me, remote, attempt]);
  const shown = useMemo(() => (all ? applyFilters(all, filters) : null), [all, filters]);
  if (loading) return <Loading what="your account" />;
  if (!me) return <Navigate to="/signin" replace />;
  if (!me.profile?.confirmed) return <Navigate to="/resume" replace />;
  const set = (patch: Partial<Filters>) => {
    setFilters({ ...filters, ...patch });
  };
  const n = active(filters);

  return (
    <>
      <header className="page-head">
        <h1>Roles that fit your resume</h1>
        <p className="lede">
          Ranked by how well your confirmed profile fits. <Link to="/jd">Found a job elsewhere? Bring its description.</Link>
        </p>
        {refreshedAt ? <p className="fine">Jobs last refreshed {ago(refreshedAt)}.</p> : null}
      </header>
      <button
        type="button"
        className="filter-toggle"
        aria-expanded={open}
        aria-controls="filters"
        onClick={() => {
          setOpen(!open);
        }}
      >
        Filters{n > 0 ? ` (${String(n)} on)` : ""}
      </button>
      <form id="filters" className={open ? "filter-bar open" : "filter-bar"} aria-label="Filter matches" onSubmit={(e) => { e.preventDefault(); }}>
        <div className="field">
          <label htmlFor="f-company">Company</label>
          <input id="f-company" value={filters.company} onChange={(e) => { set({ company: e.target.value }); }} />
        </div>
        <div className="field">
          <label htmlFor="f-location">Location</label>
          <input id="f-location" value={filters.location} placeholder="Bengaluru" onChange={(e) => { set({ location: e.target.value }); }} />
        </div>
        <div className="field">
          <label htmlFor="f-level">Level</label>
          <select id="f-level" value={filters.level} onChange={(e) => { set({ level: e.target.value }); }}>
            <option value="">Any</option>
            <option value="intern">Intern</option>
            <option value="entry">Entry</option>
            <option value="mid">Mid</option>
            <option value="unknown">Not stated</option>
          </select>
        </div>
        <div className="field">
          <label htmlFor="f-days">Posted</label>
          <select id="f-days" value={filters.days} onChange={(e) => { set({ days: Number(e.target.value) }); }}>
            <option value={0}>Any time</option>
            <option value={7}>Last week</option>
            <option value={30}>Last month</option>
          </select>
        </div>
        <label className="check">
          <input type="checkbox" checked={filters.internship} onChange={(e) => { set({ internship: e.target.checked }); }} />
          Internships only
        </label>
        <label className="check">
          <input
            type="checkbox"
            checked={remote ?? true}
            onChange={(e) => {
              setRemote(e.target.checked);
            }}
          />
          Remote roles open to India
        </label>
      </form>

      {error ? (
        <div className="state" role="alert">
          <p>{error}</p>
          <button
            type="button"
            onClick={() => {
              setAttempt(attempt + 1);
            }}
          >
            Try again
          </button>
        </div>
      ) : null}
      {!shown && !error ? <MatchSkeleton /> : null}
      {shown ? (
        <p className="count" role="status">
          {shown.length === all?.length ? `${String(shown.length)} matches` : `${String(shown.length)} of ${String(all?.length ?? 0)} matches`}
        </p>
      ) : null}
      {all?.length === 0 ? (
        <div className="state">
          <p>No open role fits your profile right now. New postings arrive every day.</p>
          <Link to="/account">Widen what you&apos;re looking for</Link>
        </div>
      ) : null}
      {all && all.length > 0 && shown?.length === 0 ? (
        <div className="state">
          <p>Nothing matches these filters.</p>
          <button
            type="button"
            onClick={() => {
              setFilters(NO_FILTERS);
            }}
          >
            Clear filters
          </button>
        </div>
      ) : null}
      {shown && shown.length > 0 ? (
        <ul className="match-list">
          {shown.map((m) => (
            <li className="match" key={m.job.key}>
              <div className="match-head">
                <JobTitle job={m.job} />
                <Band band={m.band} />
              </div>
              <Facts job={m.job} />
              <SkillSummary e={m.explanation} compact />
              <div className="match-foot">
                <SaveButton jobKey={m.job.key} />
                <Via job={m.job} />
              </div>
            </li>
          ))}
        </ul>
      ) : null}
    </>
  );
}
