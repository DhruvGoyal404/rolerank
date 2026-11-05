import type { Line } from "./contacts.js";
import type { Profile } from "./profile.js";
import { taxonomy } from "./skills.js";

/**
 * The explanation that needs no LLM (CLAUDE.md, AI invariants): matched and missing skills come
 * from the taxonomy on both sides, so "k8s" on a profile matches "Kubernetes" in a job.
 */
export function canonical(names: string[]): string[] {
  const t = taxonomy();
  const out = new Set<string>();
  for (const n of names) out.add(t.find(n)[0] ?? n);
  return [...out];
}

export type Level = "intern" | "entry" | "mid" | "senior" | "unknown";

/** A job's level from its title and stated years; a range counts by its middle. */
export function jobLevel(title: string, body: string): Level {
  if (/\b(intern|internship|co-?op|trainee)\b/i.test(title)) return "intern";
  if (/\b(senior|sr\.?|staff|principal|lead)\b/i.test(title)) return "senior";
  const m = /\b(\d{1,2}(?:\.\d)?)\s*(?:\+|(?:-|–|to)\s*(\d{1,2}(?:\.\d)?))?\s*\+?\s*(?:years?|yrs?)\b/i.exec(body);
  if (!m) return /\b(freshers?|graduates?|entry[- ]level|junior)\b/i.test(title + " " + body) ? "entry" : "unknown";
  const low = Number(m[1]);
  const n = m[2] && Number(m[2]) >= low ? (low + Number(m[2])) / 2 : low;
  return n <= 1.5 ? "entry" : n <= 4 ? "mid" : "senior";
}

const order: Record<string, number> = { intern: 0, entry: 1, mid: 2, senior: 3 };

export interface Explanation {
  matched: string[];
  missing: string[];
  niceMatched: string[];
  level: { job: Level; profile: string; fit: "fits" | "stretch" | "above" | "unknown" };
}

export function explain(profileSkills: string[], profileLevel: string, job: { required: string[]; nice: string[]; level: Level }): Explanation {
  const have = new Set(canonical(profileSkills));
  const req = canonical(job.required);
  let fit: Explanation["level"]["fit"] = "unknown";
  if (job.level !== "unknown" && profileLevel in order) {
    const gap = (order[job.level] ?? 0) - (order[profileLevel] ?? 0);
    fit = gap <= 0 ? "fits" : gap === 1 ? "stretch" : "above";
  }
  return {
    matched: req.filter((s) => have.has(s)),
    missing: req.filter((s) => !have.has(s)),
    niceMatched: canonical(job.nice).filter((s) => have.has(s)),
    level: { job: job.level, profile: profileLevel, fit },
  };
}

/** Bring a JD: the same breakdown as an indexed job, from pasted text. Nothing is stored. */
export function analyseJD(text: string, profile: Pick<Profile, "skills" | "level">): Explanation & { required: string[]; nice: string[] } {
  const { required, nice } = taxonomy().extract(text);
  const firstLine = text.split("\n").find((l) => l.trim() !== "") ?? "";
  return {
    required,
    nice,
    ...explain(
      profile.skills.map((s) => s.name),
      profile.level,
      { required, nice, level: jobLevel(firstLine, text) },
    ),
  };
}

/** The prose gate: every skill an LLM paragraph names must be in the deterministic lists. */
export function proseNamesOnly(prose: string, e: Explanation): { ok: boolean; extra: string[] } {
  const allowed = new Set([...e.matched, ...e.missing, ...e.niceMatched]);
  const extra = taxonomy()
    .find(prose)
    .filter((s) => !allowed.has(s));
  return { ok: extra.length === 0, extra };
}

export type { Line };
