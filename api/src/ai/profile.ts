import { z } from "zod";
import type { Line } from "./contacts.js";

/**
 * The parsed profile: what the model returns and what the user confirms. Every value cites the
 * resume lines (L<n>) it came from, so the grounding gate can check it. Years of experience are
 * not asked for: they are computed from the grounded dates.
 */
const lineRef = z.number().int();
export const ProfileSchema = z
  .object({
    level: z.enum(["intern", "entry", "mid", "senior"]),
    skills: z.array(z.object({ name: z.string(), line: lineRef }).strict()),
    experience: z.array(
      z
        .object({
          employer: z.string(),
          title: z.string(),
          start: z.string(), // YYYY-MM, YYYY, or "" when the resume gives none
          end: z.string(), // YYYY-MM, YYYY, "present", or ""
          lines: z.array(lineRef),
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
          line: lineRef,
        })
        .strict(),
    ),
    projects: z.array(z.object({ name: z.string(), line: lineRef }).strict()),
  })
  .strict();
export type Profile = z.infer<typeof ProfileSchema>;

/** The same schema in the JSON Schema subset the structured-outputs API accepts. */
export const profileJsonSchema = {
  type: "object",
  additionalProperties: false,
  required: ["level", "skills", "experience", "education", "projects"],
  properties: {
    level: { type: "string", enum: ["intern", "entry", "mid", "senior"] },
    skills: {
      type: "array",
      items: {
        type: "object",
        additionalProperties: false,
        required: ["name", "line"],
        properties: { name: { type: "string" }, line: { type: "integer" } },
      },
    },
    experience: {
      type: "array",
      items: {
        type: "object",
        additionalProperties: false,
        required: ["employer", "title", "start", "end", "lines"],
        properties: {
          employer: { type: "string" },
          title: { type: "string" },
          start: { type: "string" },
          end: { type: "string" },
          lines: { type: "array", items: { type: "integer" } },
        },
      },
    },
    education: {
      type: "array",
      items: {
        type: "object",
        additionalProperties: false,
        required: ["institution", "degree", "grad_year", "score", "line"],
        properties: {
          institution: { type: "string" },
          degree: { type: "string" },
          grad_year: { anyOf: [{ type: "integer" }, { type: "null" }] },
          score: { type: "string" },
          line: { type: "integer" },
        },
      },
    },
    projects: {
      type: "array",
      items: {
        type: "object",
        additionalProperties: false,
        required: ["name", "line"],
        properties: { name: { type: "string" }, line: { type: "integer" } },
      },
    },
  },
} as const;

const norm = (s: string): string =>
  s
    .normalize("NFKD")
    .replace(/\p{M}/gu, "")
    .toLowerCase()
    .replace(/[^\p{L}\p{N}+#.]+/gu, " ")
    .trim();

/** A value is grounded when its normalised text appears in one of the cited lines. */
function inLines(value: string, cited: string[]): boolean {
  const v = norm(value);
  return v !== "" && cited.some((t) => norm(t).includes(v));
}

const dateOK = (d: string): boolean => d === "" || d === "present" || /^\d{4}(-(0[1-9]|1[0-2]))?$/.test(d);
const year = (d: string): string => d.slice(0, 4);

export interface Grounded {
  profile: Profile;
  dropped: number; // entries removed because a value isn't in the lines they cite
  droppedBy: Record<string, number>;
}

/**
 * The grounding gate for parsing. Every employer, title, institution, degree, skill and project
 * must appear in a line it cites, every date's year too, and every cited line must exist. An
 * entry that fails is dropped and counted, never repaired. Hidden lines (white or invisible
 * text) can't ground anything: a value seen only there is dropped.
 */
export function ground(p: Profile, lines: Line[]): Grounded {
  const visible = new Map(lines.filter((l) => !l.hidden).map((l) => [l.n, l.text]));
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
  const profile: Profile = {
    level: p.level,
    skills: keep("skills", p.skills, (s) => {
      const c = cite([s.line]);
      return c !== null && inLines(s.name, c);
    }),
    experience: keep("experience", p.experience, (e) => {
      const c = cite(e.lines);
      if (c === null || !inLines(e.employer, c) || !inLines(e.title, c)) return false;
      if (!dateOK(e.start) || !dateOK(e.end)) return false;
      return [e.start, e.end].every((d) => d === "" || d === "present" || c.some((t) => t.includes(year(d))));
    }),
    education: keep("education", p.education, (e) => {
      // A score may sit on the line after the degree, so it may come from the next line too.
      const c = cite([e.line]);
      const near = cite([e.line, e.line + 1]) ?? c;
      if (e.score !== "" && (near === null || !inLines(e.score, near))) return false;
      if (c === null || !inLines(e.institution, c) || (e.degree !== "" && !inLines(e.degree, c))) return false;
      return e.grad_year === null || c.some((t) => t.includes(String(e.grad_year)));
    }),
    projects: keep("projects", p.projects, (pr) => {
      const c = cite([pr.line]);
      return c !== null && inLines(pr.name, c);
    }),
  };
  const dropped = Object.values(droppedBy).reduce((a, b) => a + b, 0);
  return { profile, dropped, droppedBy };
}

/** Months of experience from grounded dates, overlapping roles counted once. */
export function experienceMonths(p: Profile, now = new Date()): number {
  const toMonth = (d: string, fallback: number): number => {
    if (d === "present") return now.getUTCFullYear() * 12 + now.getUTCMonth();
    if (!d) return fallback;
    const [y, m] = d.split("-").map(Number);
    return (y ?? 0) * 12 + ((m ?? 1) - 1);
  };
  const spans = p.experience
    .filter((e) => e.start)
    .map((e) => [toMonth(e.start, 0), toMonth(e.end, toMonth(e.start, 0) + 1)] as const)
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
