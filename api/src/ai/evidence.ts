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

export function tailoring(lines: Line[], job: JobSkills): Tailoring {
  const t = taxonomy();
  const required = canonical(job.required);
  const nice = canonical(job.nice).filter((s) => !required.includes(s));
  const visible = lines
    .filter((l) => !l.hidden && !l.tiny)
    .map((l) => {
      const spans = t.spans(l.text);
      return { ...l, spans, names: new Set(spans.map((s) => s.name)), list: isList(l.text) };
    });

  const evidence: Evidence[] = [];
  const notShown: string[] = [];
  for (const skill of required) {
    const hits = visible.filter((l) => l.names.has(skill));
    if (hits.length === 0) notShown.push(skill);
    else
      evidence.push({
        skill,
        lines: [...hits.filter((l) => !l.list), ...hits.filter((l) => l.list)].slice(0, MAX_LINES_PER_SKILL).map((l) => l.n),
      });
  }

  // One swap per (line, skill): the line's own words for that skill become the job's.
  const alignments: Alignment[] = [];
  const wanted = new Set([...required, ...nice]);
  for (const l of visible) {
    const done = new Set<string>();
    for (const m of l.spans) {
      if (!wanted.has(m.name) || done.has(m.name)) continue;
      const to = job.forms?.[m.name] ?? m.name;
      if (m.surface.toLowerCase() === to.toLowerCase()) continue;
      done.add(m.name);
      alignments.push({ line: l.n, from: m.surface, to, rewrite: l.text.slice(0, m.start) + to + l.text.slice(m.end) });
    }
  }

  let lastLine = -1;
  // Whole bullets: a line that continues a wrapped bullet (no marker of its own, after a line that
  // had one) belongs to it, so its skills and numbers count for the bullet, named by its first line.
  const bullets: Array<{ n: number; text: string; names: Set<string> }> = [];
  for (const l of visible) {
    const last = bullets.at(-1);
    if (last && !BULLET.test(l.text) && !l.list && BULLET.test(visible.find((v) => v.n === last.n)?.text ?? "") && l.n === lastLine + 1) {
      last.text += ` ${l.text}`;
      for (const s of l.names) last.names.add(s);
    } else if (!l.list && l.text.length >= 30) bullets.push({ n: l.n, text: l.text, names: new Set(l.names) });
    lastLine = l.n;
  }
  // Ranked by the job's skills they name: required ones count double.
  const scored = bullets
    .map((b) => ({ b, score: 2 * required.filter((s) => b.names.has(s)).length + nice.filter((s) => b.names.has(s)).length }))
    .filter((x) => x.score > 0)
    .sort((a, b) => b.score - a.score || a.b.n - b.b.n);
  return {
    evidence,
    notShown,
    alignments,
    emphasis: {
      top: scored.slice(0, TOP).map((x) => x.b.n),
      addMetric: scored
        .filter((x) => !/\d/.test(x.b.text))
        .slice(0, MAX_METRIC_PROMPTS)
        .map((x) => x.b.n),
    },
  };
}

/** Grounded outputs, as EVALS.md counts them: each mapped skill, each alias swap, each emphasis pick. */
export const groundedCount = (t: Tailoring): number => t.evidence.length + t.alignments.length + t.emphasis.top.length;
