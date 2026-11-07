import { readFileSync } from "node:fs";

/**
 * The skill matcher, ported from worker/internal/skills (Go) so the API can read a pasted job
 * description and canonicalise profile skills without a round trip. It reads the same
 * taxonomy.txt, and testdata/skills.json (written by the Go matcher, `go run
 * ./cmd/skillfixture`) proves both give the same answers. Go measures line length in bytes and
 * this in characters, which differs only for non-ASCII text.
 */
const token = /[A-Za-z0-9+#.]+|[/-]/g;

function tokenSpans(s: string): { words: string[]; spans: Array<[number, number]> } {
  const words: string[] = [];
  const spans: Array<[number, number]> = [];
  for (const m of s.matchAll(token)) {
    const t = m[0].replace(/\.+$/, ""); // a sentence's full stop isn't part of "Node.js."
    if (t !== "") {
      words.push(t);
      spans.push([m.index, m.index + t.length]);
    }
  }
  return { words, spans };
}

export class Taxonomy {
  private readonly insensitive = new Map<string, string>();
  private readonly sensitive = new Map<string, string>();
  private longest = 0;
  readonly names: string[] = [];

  constructor(text: string) {
    const seen = new Set<string>();
    text.split("\n").forEach((raw, n) => {
      const line = raw.trim();
      if (line === "" || line.startsWith("#")) return;
      const parts = line.split("|");
      if (parts.length !== 3) throw new Error(`taxonomy line ${n + 1}: want 3 fields`);
      const first = (parts[0] ?? "").trim();
      const forms = [first, ...(parts[2] ?? "").split(",").map((a) => a.trim()).filter(Boolean)];
      const name = first.replace(/^=/, "");
      if (seen.has(name)) throw new Error(`taxonomy line ${n + 1}: ${name} listed twice`);
      seen.add(name);
      this.names.push(name);
      for (const f of forms) {
        const exact = f.startsWith("=");
        const { words } = tokenSpans(f.replace(/^=/, ""));
        if (words.length === 0) throw new Error(`taxonomy line ${n + 1}: empty form`);
        this.longest = Math.max(this.longest, words.length);
        if (exact) {
          this.sensitive.set(words.join(" "), name);
          continue;
        }
        const key = words.join(" ").toLowerCase();
        const other = this.insensitive.get(key);
        if (other !== undefined && other !== name) throw new Error(`taxonomy line ${n + 1}: ${key} means ${other} and ${name}`);
        this.insensitive.set(key, name);
      }
    });
  }

  /** The canonical skill when the whole of s is one skill's name or alias ("LLMs"), else undefined. */
  exact(s: string): string | undefined {
    const key = tokenSpans(s).words.join(" ");
    return this.sensitive.get(key) ?? this.insensitive.get(key.toLowerCase());
  }

  /** Canonical skills in text, in order of first appearance, longest match first. */
  find(text: string): string[] {
    const { words, spans } = tokenSpans(text);
    const out: string[] = [];
    const seen = new Set<string>();
    for (let i = 0; i < words.length; ) {
      let matched = 0;
      for (let n = Math.min(this.longest, words.length - i); n > 0 && matched === 0; n--) {
        const key = words.slice(i, i + n).join(" ");
        let name = this.sensitive.get(key);
        const span = spans[i];
        // One letter is a skill (C, R) only standing alone: not "(C)", "P&C", "C-level", "Series C".
        if (name !== undefined && key.length === 1 && span && !alone(text, span[0], span[1], i, words)) name = undefined;
        name ??= this.insensitive.get(key.toLowerCase());
        if (name !== undefined) {
          matched = n;
          if (!seen.has(name)) {
            seen.add(name);
            out.push(name);
          }
        }
      }
      i += Math.max(matched, 1);
    }
    return out;
  }

  /** Splits a job's skills into required and nice to have, as the Go Extract does. */
  extract(body: string): { required: string[]; nice: string[] } {
    let inNice = false;
    const req = new Set<string>();
    const nice = new Set<string>();
    for (const line of body.split("\n")) {
      const heading = line.trim().length <= 80;
      const found = this.find(line);
      if (heading && niceMark.test(line) && found.length === 0) {
        inNice = true;
        continue;
      }
      if (heading && requiredMark.test(line) && !niceMark.test(line) && found.length === 0) {
        inNice = false;
        continue;
      }
      const isNice = inNice || niceMark.test(line);
      for (const s of found) (isNice ? nice : req).add(s);
    }
    const sort = (xs: string[]) => xs.sort((a, b) => (a < b ? -1 : a > b ? 1 : 0));
    return { required: sort([...req]), nice: sort([...nice].filter((s) => !req.has(s))) };
  }
}

function alone(text: string, start: number, end: number, i: number, words: string[]): boolean {
  if (start > 0 && "(&-'’".includes(text.charAt(start - 1))) return false;
  if (end < text.length && ")&-'’".includes(text.charAt(end))) return false;
  const prev = (words[i - 1] ?? "").toLowerCase();
  return !(i > 0 && (prev === "series" || prev === "grade" || prev === "level"));
}

const niceMark =
  /\b(nice[- ]to[- ]haves?|preferred|preferably|bonus|good[- ]to[- ]have|a plus|plus points?|desired|desirable|ideally|extra credit|not required)\b/i;
const requiredMark =
  /\b(requirements?|qualifications?|must[- ]haves?|what you('|’)?ll need|what we('|’)?re looking for|what you bring|about you|you have|you will have|skills|experience|responsibilities|what you('|’)?ll do)\b/i;

let loaded: Taxonomy | undefined;

/** The shared taxonomy file; ROLERANK_TAXONOMY overrides its path (the deployed image). */
export function taxonomy(): Taxonomy {
  loaded ??= new Taxonomy(
    readFileSync(process.env.ROLERANK_TAXONOMY ?? new URL("../../../worker/internal/skills/taxonomy.txt", import.meta.url), "utf8"),
  );
  return loaded;
}
