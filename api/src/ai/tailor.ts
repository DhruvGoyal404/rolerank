import type { Line } from "./contacts.js";
import type { Profile } from "./profile.js";
import { taxonomy } from "./skills.js";

/**
 * The tailoring grounding gate (CLAUDE.md, AI invariants; EVALS.md, "Tailoring"). A suggestion
 * must cite a resume line that exists and is visible, and every number, year, technology and
 * proper name in its rewrite must appear in that line or in the user's confirmed profile. A
 * suggestion that fails is dropped and counted. Tailoring never invents experience.
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
  const profileText = JSON.stringify(profile).toLowerCase();
  const profileSkills = new Set(profile.skills.flatMap((s) => [s.name, ...t.find(s.name)]));
  const out: GateResult = { kept: [], dropped: [] };
  for (const s of suggestions) {
    const source = visible.get(s.line);
    if (source === undefined) {
      out.dropped.push({ suggestion: s, why: "cited line doesn't exist or is hidden" });
      continue;
    }
    const src = source.toLowerCase();
    const known = (v: string) => src.includes(v.toLowerCase()) || profileText.includes(v.toLowerCase());
    const badNumber = numbers(s.rewrite).find((n) => !known(n));
    if (badNumber !== undefined) {
      out.dropped.push({ suggestion: s, why: `number ${badNumber} not in the line or profile` });
      continue;
    }
    const lineSkills = new Set(t.find(source));
    const badSkill = t.find(s.rewrite).find((k) => !lineSkills.has(k) && !profileSkills.has(k));
    if (badSkill !== undefined) {
      out.dropped.push({ suggestion: s, why: `technology ${badSkill} not in the line or profile` });
      continue;
    }
    const badName = names(s.rewrite).find((n) => !known(n) && t.find(n).length === 0);
    if (badName !== undefined) {
      out.dropped.push({ suggestion: s, why: `name "${badName}" not in the line or profile` });
      continue;
    }
    out.kept.push(s);
  }
  return out;
}
