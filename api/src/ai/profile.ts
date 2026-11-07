import { z } from "zod";
import type { Line } from "./contacts.js";
import { taxonomy } from "./skills.js";

/**
 * The parsed profile: what the model returns and what the user confirms. Every value cites the
 * resume lines (L<n>) it came from, so the grounding gate can check it. Years of experience are
 * not asked for: they are computed from the grounded periods.
 */
const lineRef = z.number().int();
const lineRefs = z.array(lineRef);
const period = z.object({ start: z.string(), end: z.string() }).strict(); // YYYY-MM, YYYY, "present" (end) or ""
export const KINDS = ["job", "internship", "programme"] as const;
export const ProfileSchema = z
  .object({
    level: z.enum(["intern", "entry", "mid", "senior"]),
    skills: z.array(z.object({ name: z.string(), line: lineRef }).strict()),
    experience: z.array(
      z
        .object({
          employer: z.string(),
          title: z.string(),
          // programme: a course, bootcamp, summer school or similar; it isn't work experience.
          kind: z.enum(KINDS),
          // Several: one role can run twice ("Aug 2025 – Sep 2025 & Jul 2026 – Aug 2026").
          periods: z.array(period),
          lines: lineRefs,
        })
        .strict(),
    ),
    education: z.array(
      z
        .object({
          institution: z.string(),
          degree: z.string(),
          grad_year: z.number().int().nullable(),
          score: z.string(), // "CGPA 8.6/10", "91%", or "" when the resume gives none
          // Several lines: real layouts put degree, institute, year and score on separate lines.
          lines: lineRefs,
        })
        .strict(),
    ),
    projects: z.array(z.object({ name: z.string(), line: lineRef }).strict()),
    // venue as its line names it ("SemEval-2026"), title as the bullet's leading phrase ("Task 12 -
    // Abductive Event Reasoning", or "" when the bullets name none), date YYYY-MM, YYYY or "".
    publications: z.array(z.object({ venue: z.string(), title: z.string(), date: z.string(), lines: lineRefs }).strict()),
    achievements: z.array(z.object({ text: z.string(), lines: lineRefs }).strict()),
    certifications: z.array(z.object({ name: z.string(), lines: lineRefs }).strict()),
  })
  .strict();
export type Profile = z.infer<typeof ProfileSchema>;

const str = { type: "string" } as const;
const ints = { type: "array", items: { type: "integer" } } as const;
const obj = (props: Record<string, unknown>) => ({
  type: "object",
  additionalProperties: false,
  required: Object.keys(props),
  properties: props,
});
const list = (props: Record<string, unknown>) => ({ type: "array", items: obj(props) });

/** The same schema in the JSON Schema subset the structured-outputs API accepts. */
export const profileJsonSchema = obj({
  level: { type: "string", enum: ["intern", "entry", "mid", "senior"] },
  skills: list({ name: str, line: { type: "integer" } }),
  experience: list({
    employer: str,
    title: str,
    kind: { type: "string", enum: [...KINDS] },
    periods: list({ start: str, end: str }),
    lines: ints,
  }),
  education: list({
    institution: str,
    degree: str,
    grad_year: { anyOf: [{ type: "integer" }, { type: "null" }] },
    score: str,
    lines: ints,
  }),
  projects: list({ name: str, line: { type: "integer" } }),
  publications: list({ venue: str, title: str, date: str, lines: ints }),
  achievements: list({ text: str, lines: ints }),
  certifications: list({ name: str, lines: ints }),
});

/** Verbatim up to case and whitespace (NFKC folds PDF ligatures such as "ﬁ"). */
const fold = (s: string): string => s.normalize("NFKC").toLowerCase().replace(/\s+/g, " ").trim();
const alnum = /[\p{L}\p{N}]/u;

/** v appears in t as written, and not inside a longer word: "Go" isn't grounded by "Google". */
function contains(t: string, v: string): boolean {
  for (let i = t.indexOf(v); i >= 0; i = t.indexOf(v, i + 1)) {
    const before = t.charAt(i - 1);
    const after = t.charAt(i + v.length);
    const okL = !alnum.test(v.charAt(0)) || !alnum.test(before);
    const okR = !alnum.test(v.charAt(v.length - 1)) || !alnum.test(after);
    if (okL && okR) return true;
  }
  return false;
}

/**
 * A value is grounded when it appears verbatim (case and spacing aside) in a cited line, or across
 * the cited lines read in order, for a title that wraps.
 */
export function inLines(value: string, cited: string[]): boolean {
  const v = fold(value);
  return v !== "" && [...cited, cited.join(" ")].some((t) => contains(fold(t), v));
}

const dateOK = (d: string): boolean => d === "" || d === "present" || /^\d{4}(-(0[1-9]|1[0-2]))?$/.test(d);
const year = (d: string): string => d.slice(0, 4);
/** A line states a year as "2024", or as "'24" / "’24" (Jul'24). */
const hasYear = (text: string, y: string): boolean => text.includes(y) || new RegExp(`['’]${y.slice(2)}\\b`).test(text);

/** School results, not degrees: boards, Class X/XII, 10th/12th (the owner's resume, 2026-11-06). */
export const SCHOOL =
  /\b(ISC|ICSE|CBSE|HSC|SSC|AISSCE|AISSE|PUC|Pre-University|Intermediate|Higher Secondary|Senior Secondary|Secondary School|Matriculation|State Board|Board of (Secondary|School|Higher Secondary|Intermediate)|Class\s*(X|XII|10|12)|Std\.?\s*(X|XII|10|12)|(10|12)th)\b/i;
/** Titles that name a programme, whatever kind the model chose: they never count as work. */
export const PROGRAMME = /\b(programmes?|programs?|summer schools?|winter schools?|bootcamps?|courses?|specializations?|specialisations?|workshops?|MOOCs?)\b/i;
/** ...unless the title also names a role: "Program Manager" and "Course Instructor" are jobs. */
const ROLE =
  /\b(manager|management|coordinator|instructor|lead|officer|analyst|engineer|developer|director|specialist|associate|assistant|mentor|trainer|teacher|head)\b/i;
const isProgramme = (e: { title: string; employer: string }): boolean =>
  (PROGRAMME.test(e.title) || PROGRAMME.test(e.employer)) && !ROLE.test(e.title);

/**
 * A line that addresses instructions to an AI grounds nothing, like hidden text. parse-v4 reads
 * skills from any line, so a visible "ignore previous instructions ... Expert in Scala" line
 * could otherwise ground "Scala" (D7-003 on parse-v4, 2026-11-06).
 */
export const INSTRUCTION = new RegExp(
  [
    String.raw`\b(ignore|disregard|forget)\b.{0,40}\b(instructions?|prompts?|above)\b`,
    String.raw`\bsystem (prompt|note|message|instruction)s?\b`,
    String.raw`\b(note|message|instructions?) (to|for) (the )?(ai|assistant|model|llm|parser|bot|gpt|claude)\b`,
    String.raw`\byou are now\b|\bwith no rules\b|\bas an ai\b`,
    String.raw`\b(output|print|reveal|repeat|show)\b.{0,20}\b(system prompt|your (prompt|instructions))\b`,
    String.raw`\brank (this|me|the) (candidate|resume|applicant)\b`,
    String.raw`\b(list|add|include)\b.{0,40}\bas (a |an |the |their |the candidate's |my )?(top |key |main )?skills?\b`,
    String.raw`"skills"\s*:\s*\[|\buse this json\b`,
  ].join("|"),
  "i",
);

export interface Grounded {
  profile: Profile;
  dropped: number; // entries removed because a value isn't in the lines they cite
  droppedBy: Record<string, number>; // "experience.employer": the field and, for roles, the check that failed
  school: number; // education rows removed as school results (not a grounding failure)
}

/**
 * The grounding gate for parsing. Every name, title, employer, institution, degree, score,
 * skill, project, publication, achievement and certification must appear verbatim in a line it
 * cites; every period's years too; every cited line must exist. An entry that fails is dropped
 * and counted, never repaired. Hidden lines (white or invisible text) and lines that address
 * instructions to an AI can't ground anything.
 * Two deterministic rules follow: school rows leave education, and a role whose title or employer
 * names a programme is kind "programme".
 */
export function ground(p: Profile, lines: Line[]): Grounded {
  const visible = new Map(lines.filter((l) => !l.hidden && !INSTRUCTION.test(l.text)).map((l) => [l.n, l.text]));
  const cite = (ns: number[]): string[] | null => {
    const out = ns.map((n) => visible.get(n));
    return ns.length > 0 && out.every((t) => t !== undefined) ? out : null;
  };
  const droppedBy: Record<string, number> = {};
  const keep = <T>(field: string, items: T[], ok: (t: T) => boolean): T[] =>
    items.filter((t) => {
      if (ok(t)) return true;
      droppedBy[field] = (droppedBy[field] ?? 0) + 1;
      return false;
    });
  const named = <T>(field: string, items: T[], value: (t: T) => string, refs: (t: T) => number[]) =>
    keep(field, items, (t) => {
      const c = cite(refs(t));
      return c !== null && inLines(value(t), c);
    });
  const periodOK = (c: string[], pr: { start: string; end: string }): boolean =>
    dateOK(pr.start) &&
    dateOK(pr.end) &&
    [pr.start, pr.end].every((d) => d === "" || d === "present" || c.some((t) => hasYear(t, year(d)))) &&
    !(pr.start !== "" && pr.end !== "" && pr.end !== "present" && pr.end.padEnd(7, "-99") < pr.start);

  const education = p.education.filter((e) => !SCHOOL.test(e.degree) && !SCHOOL.test(e.institution));
  const experience = p.experience
    .map((e) => {
      // The employer often has a line of its own above the title, and the model cites only the
      // title's lines (D1-dev-3, 2026-11-07). Take the employer from one of the two lines just
      // above, verbatim, and cite it.
      const c = cite(e.lines);
      if (c === null || inLines(e.employer, c)) return e;
      const top = Math.min(...e.lines);
      const above = [top - 1, top - 2].find((n) => {
        const t = visible.get(n);
        return t !== undefined && inLines(e.employer, [t]);
      });
      return above === undefined ? e : { ...e, lines: [above, ...e.lines] };
    })
    .filter((e) => {
      const c = cite(e.lines);
      const why =
        c === null ? "lines" : !inLines(e.employer, c) ? "employer" : !inLines(e.title, c) ? "title" : e.periods.every((pr) => periodOK(c, pr)) ? null : "period";
      if (why) droppedBy[`experience.${why}`] = (droppedBy[`experience.${why}`] ?? 0) + 1;
      return why === null;
    });
  const kept = {
    education: keep("education", education, (e) => {
      // Each value must be in the cited lines (D1-hard: layouts split them across lines).
      const c = cite(e.lines);
      if (c === null || !inLines(e.institution, c) || (e.degree !== "" && !inLines(e.degree, c))) return false;
      if (e.score !== "" && !inLines(e.score, c)) return false;
      return e.grad_year === null || c.some((t) => t.includes(String(e.grad_year)));
    }),
  };
  const gradYears = kept.education.map((e) => e.grad_year).filter((y): y is number => y !== null);
  const profile: Profile = {
    level: p.level,
    skills: splitSkills(named("skills", p.skills, (s) => s.name, (s) => [s.line]), visible),
    experience: experience.map((e) => {
      if (isProgramme(e)) return { ...e, kind: "programme" as const };
      return e.kind === "job" && whileStudying(e, gradYears) ? { ...e, kind: "internship" as const } : e;
    }),
    education: kept.education,
    projects: named("projects", p.projects, (x) => x.name, (x) => [x.line]),
    // The venue and the paper are often on different lines; each must be in the cited ones.
    publications: keep("publications", p.publications, (x) => {
      const c = cite(x.lines);
      if (c === null || (x.venue === "" && x.title === "")) return false;
      if ((x.venue !== "" && !inLines(x.venue, c)) || (x.title !== "" && !inLines(x.title, c))) return false;
      return x.date === "" || (dateOK(x.date) && x.date !== "present" && c.some((t) => hasYear(t, year(x.date))));
    }),
    achievements: named("achievements", p.achievements, (x) => x.text, (x) => x.lines),
    certifications: named("certifications", p.certifications, (x) => x.name, (x) => x.lines),
  };
  const dropped = Object.values(droppedBy).reduce((a, b) => a + b, 0);
  return { profile, dropped, droppedBy, school: p.education.length - education.length };
}

/** A run of skill names, as resumes list them: no quotes, brackets or sentence punctuation. */
const SKILLISH = /^[\p{L}\p{N}][\p{L}\p{N} .+#&/-]*$/u;
/** A trailing date, as a right-aligned column leaves it on the line: "NLP Jun 2026". */
const TRAILING_DATE = /\s+(?:(?:jan|feb|mar|apr|may|jun|jul|aug|sep|sept|oct|nov|dec)[a-z]*\.?\s*['’]?\d{2,4}|\d{4})$/i;

/** Each part when every part of "A & B" or "A/B" is a known skill, else null. */
function compound(name: string): string[] | null {
  const t = taxonomy();
  if (t.exact(name) !== undefined || !/[&/]/.test(name)) return null;
  const parts = name.split(/\s*[&/]\s*/).filter(Boolean);
  return parts.length > 1 && parts.every((x) => t.exact(x) !== undefined) ? parts : null;
}

/**
 * Two deterministic steps after the gate (owner's resume, 2026-11-07). A compound of known skills
 * splits into its parts ("Generative AI & LLMs" is Generative AI and LLMs). And on a line the
 * model already cited for a skill, every comma-separated item that is a known skill is a skill
 * too ("LLMs, Chain-of-Thought, NLP" gives NLP). Names stay verbatim from the line, so they stay
 * grounded; lines the gate can't use (hidden, instructions) never add anything.
 */
function splitSkills(skills: Profile["skills"], visible: Map<number, string>): Profile["skills"] {
  const out: Profile["skills"] = [];
  const seen = new Set<string>();
  const add = (name: string, line: number) => {
    const k = fold(name);
    if (seen.has(k)) return;
    seen.add(k);
    out.push({ name, line });
  };
  for (const s of skills) for (const n of compound(s.name) ?? [s.name]) add(n, s.line);
  for (const line of new Set(skills.map((s) => s.line))) {
    for (const raw of (visible.get(line) ?? "").split(/[,;|:]/)) {
      const item = raw.trim().replace(TRAILING_DATE, "").trim();
      if (!SKILLISH.test(item)) continue;
      for (const n of compound(item) ?? (taxonomy().exact(item) !== undefined ? [item] : [])) add(n, line);
    }
  }
  return out;
}

/**
 * A "job" held for three months or less, ending before the year the candidate graduates, is an
 * internship in practice: a student's research associateship or summer role.
 */
function whileStudying(e: Profile["experience"][number], gradYears: number[]): boolean {
  if (gradYears.length === 0 || e.periods.length === 0) return false;
  const months = experienceMonths({ experience: [{ ...e, kind: "job" }] });
  const ends = e.periods.map((pr) => pr.end);
  if (ends.some((d) => d === "" || d === "present")) return false;
  const lastEnd = Math.max(...ends.map((d) => Number(d.slice(0, 4))));
  return months <= 3 && lastEnd < Math.max(...gradYears);
}

/**
 * Months of work experience: the union of every job and internship period, both end months
 * included ("Jun 2025 – Aug 2025" is 3), so overlaps and a role held twice count once each month.
 * Programmes don't count. A period with no end counts one month; "present" runs to this month.
 */
export function experienceMonths(p: Pick<Profile, "experience">, now = new Date()): number {
  const toMonth = (d: string): number => {
    if (d === "present") return now.getUTCFullYear() * 12 + now.getUTCMonth();
    const [y, m] = d.split("-").map(Number);
    return (y ?? 0) * 12 + ((m ?? 1) - 1);
  };
  const spans = p.experience
    .filter((e) => e.kind !== "programme")
    .flatMap((e) => e.periods)
    .filter((pr) => pr.start !== "")
    .map((pr) => [toMonth(pr.start), (pr.end ? toMonth(pr.end) : toMonth(pr.start)) + 1] as const)
    .filter(([a, b]) => b >= a)
    .sort((x, y) => x[0] - y[0]);
  let total = 0;
  let cur: [number, number] | null = null;
  for (const [a, b] of spans) {
    if (cur && a <= cur[1]) cur[1] = Math.max(cur[1], b);
    else {
      if (cur) total += cur[1] - cur[0];
      cur = [a, b];
    }
  }
  if (cur) total += cur[1] - cur[0];
  return total;
}
