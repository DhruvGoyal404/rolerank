import type { Line } from "./contacts.js";
import { canonical } from "./match.js";
import { taxonomy } from "./skills.js";

/**
 * Tailoring, re-scoped (owner's decision, 2026-11-08): three grounded outputs instead of free
 * rewriting, which invented experience on tailor-v1 and said almost nothing once gated (PHASE5.md).
 * All three are computed from the resume's own lines and the taxonomy; no model writes any of it.
 *
 * 1. Evidence map: for each skill the job requires, the lines that name it, or "not shown".
 * 2. Terminology alignment: a line naming a skill by another taxonomy alias than the job uses
 *    ("Postgres" where the job says "PostgreSQL") gets that one swap, and nothing else changes.
 * 3. Emphasis: the bullets that matter most for this job first, and a prompt to add a number to
 *    a relevant bullet that has none, if the user has one. A number is never suggested.
 */
export interface JobSkills {
  required: string[];
  nice: string[];
  /** How the job writes each canonical skill, when known (a pasted JD's own words). */
  forms?: Record<string, string> | undefined;
}

export interface Evidence {
  skill: string;
  lines: number[]; // strongest first: work and project lines before a skills list
}
export interface Alignment {
  line: number;
  from: string;
  to: string;
  rewrite: string;
}
export interface Emphasis {
  top: number[]; // put these first
  addMetric: number[]; // relevant bullets with no number: add one if you have it
}
export interface Tailoring {
  evidence: Evidence[];
  notShown: string[];
  alignments: Alignment[];
  emphasis: Emphasis;
}

const MAX_LINES_PER_SKILL = 3;
const TOP = 3;
const MAX_METRIC_PROMPTS = 3;
/** A line that starts a bullet: "-", "–", "•", "*" and the like. */
const BULLET = /^\s*[-–—•*▪●◦]\s/;

/** A skills list ("Languages: Python, Go, SQL"): it names skills but proves less than a bullet. */
// A list when at least three of its comma-separated items, and most of them, are each just a skill
// ("2,000 users" or "Intern, Acme Corp" in a bullet don't make it one).
function isList(text: string): boolean {
  const t = taxonomy();
  const items = text
    .replace(/^[^:]{0,40}:/, "")
    .split(/[,;|]/)
    .map((x) => x.trim())
    .filter(Boolean);
  const skills = items.filter((x) => t.exact(x) !== undefined).length;
  return skills >= 3 && skills * 2 > items.length;
}

/** How a job writes its skills: the first surface the taxonomy finds for each canonical name. */
export function formsIn(text: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const m of taxonomy().spans(text)) out[m.name] ??= m.surface;
  return out;
}

/** A section heading: no lower-case letters ("ACHIEVEMENTS & CERTIFICATIONS"), or a short label ending in ":". */
const HEADING = /^[^a-z]*[A-Z]{2}[^a-z]*$|^\s*\S+(?:\s+\S+){0,4}:\s*$/;
/** A line that starts with a date: "Jan 2024 - Mar 2024", "03/2024", "2023 -". */
const DATE_LED = /^\s*(?:(?:jan|feb|mar|apr|may|jun|jul|aug|sep|sept|oct|nov|dec)[a-z]*\.?\s*['’]?\d{2,4}|\d{1,2}[/.-]\d{2,4}|(?:19|20)\d{2}\b)/i;
/** An entry line: "Title | Organisation | Year". */
const ENTRY = /\s\|\s/;
/** Words that end a line cut off mid-phrase by the wrap ("...and later added"). */
const CUT = /[a-z,]$/;
const MAX_UNIT_LINES = 4;

/**
 * Units: each whole bullet, with the lines a wrapped bullet continues on, and every other line on
 * its own. A unit is named by its first line. Matching runs on the joined text, so "integration" +
 * "tests" across a wrap is one skill (D4-R, 2026-11-08). Only a true wrap joins (owner's
 * walkthrough, 2026-11-09): a line with no marker of its own that starts in lower case, or that
 * follows a long line cut off mid-phrase. A heading, a new bullet, a date-led line, an entry line
 * with "|" or a skills list always starts its own unit, and a unit is at most four lines.
 */
export function units(lines: Line[]): Array<{ n: number; text: string }> {
  const visible = lines.filter((l) => !l.hidden && !l.tiny);
  const wide = Math.max(0, ...visible.map((l) => l.text.trim().length));
  const out: Array<{ n: number; text: string; open: boolean; last: string; count: number }> = [];
  let prev = -1;
  for (const l of visible) {
    const last = out.at(-1);
    const t = l.text.trim();
    const own = BULLET.test(l.text) || HEADING.test(t) || DATE_LED.test(t) || ENTRY.test(t) || isList(t);
    const wraps =
      !own &&
      last?.open === true &&
      l.n === prev + 1 &&
      last.count < MAX_UNIT_LINES &&
      (/^[a-z(&]/.test(t) || (CUT.test(last.last) && last.last.length >= 0.75 * wide));
    if (last && wraps) {
      last.text += ` ${t}`;
      last.last = t;
      last.count++;
    } else out.push({ n: l.n, text: l.text, open: BULLET.test(l.text), last: t, count: 1 });
    prev = l.n;
  }
  return out.map(({ n, text }) => ({ n, text }));
}

/** Link and contact labels: a line made mostly of these is a header, never evidence. */
const LINK_LABEL =
  /\[(?:EMAIL|PHONE|URL|PIN)\]|\b(?:linkedin|github|gitlab|bitbucket|leetcode|codeforces|codechef|hackerrank|hackerearth|kaggle|orcid|portfolio|website|blog|medium|twitter|behance|dribbble|scholar|researchgate|stack\s?overflow)\b/i;

/**
 * The contact header, or any line that is mostly links and labels ("[PHONE] | [EMAIL] | LinkedIn |
 * GitHub | DSA Portfolio"): it names no work, so it proves no skill (owner's walkthrough, 2026-11-09).
 */
export function isLinkLine(text: string): boolean {
  const parts = text
    .split(/\s[|•·]\s|\s{3,}|\s[-–]\s/)
    .map((x) => x.trim())
    .filter(Boolean);
  if (parts.length < 2) return false;
  const labels = parts.filter((x) => LINK_LABEL.test(x) && x.split(/\s+/).length <= 4).length;
  return labels * 2 >= parts.length;
}

/** Platform names the taxonomy lists as aliases of a skill: "GitHub" is not proof of Git. */
const PLATFORMS = new Set(["github", "gitlab", "bitbucket"]);

export function tailoring(lines: Line[], job: JobSkills): Tailoring {
  const t = taxonomy();
  const required = canonical(job.required);
  const nice = canonical(job.nice).filter((s) => !required.includes(s));
  const all = units(lines)
    .filter((u) => !isLinkLine(u.text))
    .map((u) => {
    const spans = t.spans(u.text).filter((m) => !PLATFORMS.has(m.surface.toLowerCase()));
    return { ...u, spans, names: new Set(spans.map((s) => s.name)), list: isList(u.text) };
  });

  const evidence: Evidence[] = [];
  const notShown: string[] = [];
  for (const skill of required) {
    const hits = all.filter((u) => u.names.has(skill));
    if (hits.length === 0) notShown.push(skill);
    else
      evidence.push({
        skill,
        lines: [...hits.filter((u) => !u.list), ...hits.filter((u) => u.list)].slice(0, MAX_LINES_PER_SKILL).map((u) => u.n),
      });
  }

  // One swap per (unit, skill): the unit's own words for that skill become the job's.
  const alignments: Alignment[] = [];
  const wanted = new Set([...required, ...nice]);
  for (const u of all) {
    const done = new Set<string>();
    for (const m of u.spans) {
      if (!wanted.has(m.name) || done.has(m.name)) continue;
      const to = job.forms?.[m.name] ?? m.name;
      if (m.surface.toLowerCase() === to.toLowerCase()) continue;
      done.add(m.name);
      alignments.push({ line: u.n, from: m.surface, to, rewrite: u.text.slice(0, m.start) + to + u.text.slice(m.end) });
    }
  }

  // Bullets, not lists or short headings, ranked by the job's skills they name: required count double.
  const scored = all
    .filter((u) => !u.list && u.text.length >= 30)
    .map((u) => ({ u, score: 2 * required.filter((s) => u.names.has(s)).length + nice.filter((s) => u.names.has(s)).length }))
    .filter((x) => x.score > 0)
    .sort((a, b) => b.score - a.score || a.u.n - b.u.n);
  return {
    evidence,
    notShown,
    alignments,
    emphasis: {
      top: scored.slice(0, TOP).map((x) => x.u.n),
      addMetric: scored
        .filter((x) => !/\d/.test(x.u.text))
        .slice(0, MAX_METRIC_PROMPTS)
        .map((x) => x.u.n),
    },
  };
}

/** Grounded outputs, as EVALS.md counts them: each mapped skill, each alias swap, each emphasis pick. */
export const groundedCount = (t: Tailoring): number => t.evidence.length + t.alignments.length + t.emphasis.top.length;
