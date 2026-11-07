import type { Line } from "./contacts.js";
import type { Profile } from "./profile.js";
import { taxonomy } from "./skills.js";

/**
 * The tailoring grounding gate (CLAUDE.md, AI invariants; EVALS.md, "Tailoring"). A suggestion
 * must cite a resume line that exists and is visible. Every number and technology in its rewrite
 * (and every technology in its reason) must appear in that line, and every proper name in that
 * line or among the profile's employers, titles and institutions. A suggestion that fails is
 * dropped and counted. Tailoring never invents experience.
 *
 * Stricter than CLAUDE.md's "line or profile" on purpose: D4 on tailor-v1 (2026-11-08) showed a
 * profile skill attached to a line that never used it ("Project: URL Shortener (Python)") passes
 * a profile-wide check and is still invented. A skill the user has isn't a skill every line used.
 */
export interface Suggestion {
  line: number;
  rewrite: string;
  reason: string;
}

export interface GateResult {
  kept: Suggestion[];
  dropped: Array<{ suggestion: Suggestion; why: string }>;
}

const numbers = (s: string): string[] => s.match(/\d+(?:[.,]\d+)?/g) ?? [];
// Two or more capitalised words in a row: a company, a title, a product. Sentence-initial
// single words don't count, so ordinary capitalisation passes.
const names = (s: string): string[] => s.match(/\b[A-Z][a-zA-Z&.]+(?:\s+[A-Z][a-zA-Z&.]+)+\b/g) ?? [];

export function gate(suggestions: Suggestion[], lines: Line[], profile: Profile): GateResult {
  const t = taxonomy();
  const visible = new Map(lines.filter((l) => !l.hidden && !l.tiny).map((l) => [l.n, l.text]));
  const profileNames = [...profile.experience.flatMap((e) => [e.employer, e.title]), ...profile.education.map((e) => e.institution)]
    .map((x) => x.toLowerCase())
    .filter(Boolean);
  const out: GateResult = { kept: [], dropped: [] };
  for (const s of suggestions) {
    const source = visible.get(s.line);
    if (source === undefined) {
      out.dropped.push({ suggestion: s, why: "cited line doesn't exist or is hidden" });
      continue;
    }
    const src = source.toLowerCase();
    const badNumber = numbers(s.rewrite).find((n) => !src.includes(n.toLowerCase()));
    if (badNumber !== undefined) {
      out.dropped.push({ suggestion: s, why: `number ${badNumber} not in the line` });
      continue;
    }
    const lineSkills = new Set(t.find(source));
    // The reason is shown too, so it can't claim a technology the resume lacks either.
    const badSkill = t.find(`${s.rewrite}\n${s.reason}`).find((k) => !lineSkills.has(k));
    if (badSkill !== undefined) {
      out.dropped.push({ suggestion: s, why: `technology ${badSkill} not in the line` });
      continue;
    }
    const known = (n: string) => src.includes(n.toLowerCase()) || profileNames.some((p) => p.includes(n.toLowerCase()));
    const badName = names(s.rewrite).find((n) => !known(n) && t.find(n).length === 0);
    if (badName !== undefined) {
      out.dropped.push({ suggestion: s, why: `name "${badName}" not in the line or the profile's employers, titles and schools` });
      continue;
    }
    out.kept.push(s);
  }
  return out;
}
