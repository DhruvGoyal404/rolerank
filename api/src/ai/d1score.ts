import type { Profile } from "./profile.js";

/**
 * D1 scoring (EVALS.md, "Parsing (D1)"): field-level precision and recall after normalisation,
 * set matching for skills and names, best-assignment matching for entries. A fabricated value is
 * an extracted employer, title, institution, degree or date that appears nowhere in the gold.
 */
export interface Gold {
  id: string;
  segment: string;
  skills: string[];
  experience: Array<{ employer: string; title: string; start: string; end: string }>;
  education: Array<{ institution: string; degree: string; grad_year: number; score: string }>;
  projects: string[];
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
  | "projects";

const norm = (s: string): string => s.toLowerCase().replace(/\s+/g, " ").replace(/[.,]+$/g, "").trim();

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

export function score(p: Profile, g: Gold): Scored {
  const counts = {} as Record<Field, Counts>;
  counts.skills = setCounts(
    p.skills.map((s) => s.name),
    g.skills,
  );
  counts.projects = setCounts(
    p.projects.map((x) => x.name.replace(/^project:\s*/i, "")),
    g.projects,
  );
  const exp = assign(p.experience, g.experience, (e) => e.employer, (e) => e.employer);
  const per = (n: number) => ({ extracted: p.experience.length * n, gold: g.experience.length * n, correct: 0 });
  counts.employer = { ...per(1), correct: exp.length };
  counts.title = { ...per(1), correct: exp.filter(([a, b]) => norm(a.title) === norm(b.title)).length };
  counts.dates = {
    ...per(2),
    correct: exp.reduce((n, [a, b]) => n + Number(a.start === b.start) + Number(a.end === b.end), 0),
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
      ...g.experience.flatMap((e) => [e.employer, e.title, e.start, e.end]),
      ...g.education.flatMap((e) => [e.institution, e.degree, String(e.grad_year)]),
    ].map(norm),
  );
  const values = [
    ...p.experience.flatMap((e) => [e.employer, e.title, e.start, e.end].filter((v) => v !== "")),
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
