import { useEffect, useMemo, useState } from "react";
import { Link, Navigate } from "react-router";
import { api, ApiError, type Match } from "../api";
import { ago, ApplyLink, Band, Facts, JobTitle, Loading, MatchSkeleton, SkillSummary, Via } from "../components";
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

/** Runs of the same band, in rank order: the list stays ranked, with a heading where the band changes. */
export function byBand(ms: Match[]): Array<{ band: string; items: Match[] }> {
  const out: Array<{ band: string; items: Match[] }> = [];
  for (const m of ms) {
    const last = out.at(-1);
    if (last?.band === m.band) last.items.push(m);
    else out.push({ band: m.band, items: [m] });
  }
  return out;
}

const BAND_NOTE: Record<string, string> = {
  Strong: "Your resume shows most of what these ask for.",
  Good: "A solid overlap, with a few skills to show or learn.",
  Stretch: "Worth a look if you're ready to learn on the job.",
};

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
        {refreshedAt ? (
          <p className="fine">
            Jobs last refreshed {ago(refreshedAt)}. New postings are checked every 6 hours while RoleRank is running.
          </p>
        ) : null}
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
        {open ? "Hide filters" : "Filters"}
        {n > 0 ? ` (${String(n)} on)` : ""}
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
        <div className="checks">
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
        {n > 0 ? (
          <button
            type="button"
            className="quiet clear"
            onClick={() => {
              setFilters(NO_FILTERS);
            }}
          >
            Clear filters
          </button>
        ) : null}
        </div>
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
          <h2>No open role fits your profile yet</h2>
          <p>New postings arrive every day. Adding target roles, or turning on remote roles, widens the search.</p>
          <Link className="button" to="/account">
            Widen what you&apos;re looking for
          </Link>
        </div>
      ) : null}
      {all && all.length > 0 && shown?.length === 0 ? (
        <div className="state">
          <h2>Nothing matches these filters</h2>
          <p>{`${String(all.length)} roles fit your profile; the filters hide all of them.`}</p>
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
      {shown && shown.length > 0
        ? byBand(shown).map((group, g) => (
            <section key={`${group.band}-${String(g)}`} aria-labelledby={`band-${String(g)}`}>
              <div className="group-head">
                <h2 id={`band-${String(g)}`}>
                  <Band band={group.band} />
                  <span className="sr-only">{`, ${String(group.items.length)} roles`}</span>
                </h2>
                <p className="fine">{BAND_NOTE[group.band] ?? ""}</p>
              </div>
              <ol className={g === 0 ? "match-list reveal" : "match-list"}>
                {group.items.map((m) => (
                  <li className="match" key={m.job.key}>
                    <div className="match-head">
                      <JobTitle job={m.job} band={m.band} />
                      <Band band={m.band} />
                    </div>
                    <div className="match-body">
                      <Facts job={m.job} />
                      <SkillSummary e={m.explanation} compact />
                    </div>
                    <div className="match-foot">
                      <SaveButton jobKey={m.job.key} />
                      <ApplyLink job={m.job} compact />
                      <Via job={m.job} />
                    </div>
                  </li>
                ))}
              </ol>
            </section>
          ))
        : null}
    </>
  );
}
