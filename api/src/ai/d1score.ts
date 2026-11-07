import type { Profile } from "./profile.js";

/**
 * D1 scoring (EVALS.md, "Parsing (D1)"): field-level precision and recall after normalisation,
 * set matching for skills and names, best-assignment matching for entries. A fabricated value is
 * an extracted employer, title, institution, degree or date that appears nowhere in the gold.
 */
type Period = { start: string; end: string };
export interface Gold {
  id: string;
  segment: string;
  skills: string[];
  experience: Array<{ employer: string; title: string; kind?: string; periods: Period[] }>;
  education: Array<{ institution: string; degree: string; grad_year: number; score: string }>;
  projects: string[];
  publications: Array<{ venue: string; title: string }>; // venue "" in gold before D1-hard-4
  achievements: string[];
  certifications: string[];
  months?: number; // D1-hard-3 states it; older sets compute it from the periods
}

/**
 * Gold as the generators write it: an empty list may be null (Go's nil slice), and sets before
 * D1-hard-3 give one start and end per role instead of periods and no new sections.
 */
export type GoldIn = Omit<Gold, "skills" | "education" | "projects" | "experience" | "publications" | "achievements" | "certifications"> & {
  [K in "skills" | "education" | "projects"]: Gold[K] | null;
} & {
  experience: Array<{ employer: string; title: string; kind?: string; periods?: Period[]; start?: string; end?: string }> | null;
  publications?: Array<string | { venue: string; title: string }> | null; // a string before D1-hard-4
  achievements?: string[] | null;
  certifications?: string[] | null;
};

export function normalise(raw: GoldIn): Gold {
  return {
    ...raw,
    skills: raw.skills ?? [],
    experience: (raw.experience ?? []).map(({ start, end, periods, ...e }) => ({
      ...e,
      periods: periods ?? [{ start: start ?? "", end: end ?? "" }],
    })),
    education: raw.education ?? [],
    projects: raw.projects ?? [],
    publications: (raw.publications ?? []).map((x) => (typeof x === "string" ? { venue: "", title: x } : x)),
    achievements: raw.achievements ?? [],
    certifications: raw.certifications ?? [],
  };
}

export interface Counts {
  extracted: number;
  gold: number;
  correct: number;
}

export type Field =
  | "skills"
  | "employer"
  | "title"
  | "dates"
  | "institution"
  | "degree"
  | "grad_year"
  | "score"
  | "projects"
  | "kind"
  | "publications"
  | "achievements"
  | "certifications";

/** A publication as one string: "SemEval-2026 Task 12 - Abductive Event Reasoning". */
export const pubName = (x: { venue?: string; title: string }): string => [x.venue ?? "", x.title].filter(Boolean).join(" ");
const norm = (s: string): string => s.toLowerCase().replace(/\s+/g, " ").replace(/[.,]+$/g, "").trim();
/** A list item without its bullet or label ("– Certification: AWS ..." is "AWS ..."). */
const item = (s: string): string => s.replace(/^[\s\-–—•*]+/, "").replace(/^(project|certification|certificate)s?:\s*/i, "");

function setCounts(got: string[], want: string[]): Counts {
  const w = new Set(want.map(norm));
  const g = [...new Set(got.map(norm))];
  return { extracted: g.length, gold: w.size, correct: g.filter((x) => w.has(x)).length };
}

/** Pairs extracted entries with gold entries by key, each gold entry used once. */
function assign<A, B>(got: A[], want: B[], key: (x: A) => string, gkey: (x: B) => string): Array<[A, B]> {
  const free = [...want];
  const pairs: Array<[A, B]> = [];
  for (const a of got) {
    const i = free.findIndex((b) => norm(gkey(b)) === norm(key(a)));
    if (i >= 0) pairs.push([a, free.splice(i, 1)[0] as B]);
  }
  return pairs;
}

export interface Scored {
  counts: Record<Field, Counts>;
  fabricated: number;
}

export function score(p: Profile, raw: GoldIn): Scored {
  // The generator writes an empty list as null (Go's nil slice).
  const g = normalise(raw);
  const counts = {} as Record<Field, Counts>;
  counts.skills = setCounts(
    p.skills.map((s) => s.name),
    g.skills,
  );
  counts.projects = setCounts(p.projects.map((x) => item(x.name)), g.projects);
  // Gold before D1-hard-4 names only the paper's own title; from D1-hard-4 it names venue and title.
  const venues = g.publications.some((x) => x.venue !== "");
  counts.publications = setCounts(
    p.publications.map((x) => item(venues ? pubName(x) : x.title || x.venue)),
    g.publications.map((x) => pubName(x)),
  );
  counts.achievements = setCounts(p.achievements.map((x) => item(x.text)), g.achievements);
  counts.certifications = setCounts(p.certifications.map((x) => item(x.name)), g.certifications);
  const exp = assign(p.experience, g.experience, (e) => e.employer, (e) => e.employer);
  const per = (n: number) => ({ extracted: p.experience.length * n, gold: g.experience.length * n, correct: 0 });
  counts.employer = { ...per(1), correct: exp.length };
  counts.title = { ...per(1), correct: exp.filter(([a, b]) => norm(a.title) === norm(b.title)).length };
  // Each period's start and end; periods pair by start, each gold period used once.
  counts.dates = {
    extracted: 2 * p.experience.reduce((n, e) => n + e.periods.length, 0),
    gold: 2 * g.experience.reduce((n, e) => n + e.periods.length, 0),
    correct: exp.reduce((n, [a, b]) => {
      const free = [...b.periods];
      for (const x of a.periods) {
        const i = free.findIndex((y) => y.start === x.start);
        if (i < 0) continue;
        const [y] = free.splice(i, 1);
        n += 1 + Number(y?.end === x.end);
      }
      return n;
    }, 0),
  };
  const kinded = g.experience.filter((e) => e.kind !== undefined);
  counts.kind = {
    extracted: kinded.length ? p.experience.length : 0,
    gold: kinded.length,
    correct: exp.filter(([a, b]) => b.kind !== undefined && a.kind === b.kind).length,
  };
  const edu = assign(p.education, g.education, (e) => e.institution, (e) => e.institution);
  const eper = { extracted: p.education.length, gold: g.education.length };
  counts.institution = { ...eper, correct: edu.length };
  counts.degree = { ...eper, correct: edu.filter(([a, b]) => norm(a.degree) === norm(b.degree)).length };
  counts.grad_year = { ...eper, correct: edu.filter(([a, b]) => a.grad_year === b.grad_year).length };
  const scoreNorm = (x: string) => norm(x).replace(/\s/g, "");
  counts.score = {
    extracted: p.education.filter((e) => e.score !== "").length,
    gold: g.education.filter((e) => e.score !== "").length,
    correct: edu.filter(([a, b]) => b.score !== "" && scoreNorm(a.score) === scoreNorm(b.score)).length,
  };

  // Fabricated: a value the resume's gold doesn't contain anywhere.
  const known = new Set(
    [
      ...g.experience.flatMap((e) => [e.employer, e.title, ...e.periods.flatMap((x) => [x.start, x.end])]),
      ...g.education.flatMap((e) => [e.institution, e.degree, String(e.grad_year)]),
    ].map(norm),
  );
  const values = [
    ...p.experience.flatMap((e) => [e.employer, e.title, ...e.periods.flatMap((x) => [x.start, x.end])].filter((v) => v !== "")),
    ...p.education.flatMap((e) => [e.institution, e.degree, e.grad_year === null ? "" : String(e.grad_year)].filter(Boolean)),
  ];
  const fabricated = values.filter((v) => !known.has(norm(v))).length;
  return { counts, fabricated };
}

export function add(a: Partial<Record<Field, Counts>>, b: Record<Field, Counts>): Record<Field, Counts> {
  const out = {} as Record<Field, Counts>;
  for (const k of Object.keys(b) as Field[]) {
    out[k] = {
      extracted: (a[k]?.extracted ?? 0) + b[k].extracted,
      gold: (a[k]?.gold ?? 0) + b[k].gold,
      correct: (a[k]?.correct ?? 0) + b[k].correct,
    };
  }
  return out;
}

export const precision = (c: Counts): number => (c.extracted ? c.correct / c.extracted : 1);
export const recall = (c: Counts): number => (c.gold ? c.correct / c.gold : 1);
