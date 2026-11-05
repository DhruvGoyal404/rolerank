/**
 * Contact details are stripped before any text reaches an LLM (CLAUDE.md, AI invariants). Each
 * one is replaced by a placeholder in its line; the originals are returned so they can be kept
 * encrypted for the user and never sent anywhere.
 */
export interface Line {
  n: number;
  text: string;
  hidden?: boolean;
  tiny?: boolean;
}

export interface Contact {
  kind: "email" | "phone" | "url" | "pin";
  line: number;
  value: string;
}

const patterns: Array<{ kind: Contact["kind"]; re: RegExp; placeholder: string }> = [
  { kind: "email", re: /[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}/g, placeholder: "[EMAIL]" },
  {
    kind: "url",
    re: /\b(?:https?:\/\/|www\.)\S+|\b(?:linkedin\.com|github\.com|gitlab\.com|twitter\.com|x\.com|leetcode\.com|medium\.com)\/\S*/gi,
    placeholder: "[URL]",
  },
  // +91 98765 43210, 098765-43210, (555) 123-4567: a run of digits and separators holding 10 to
  // 15 digits (counted in stripContacts), so years, ranges and counts are left alone.
  { kind: "phone", re: /\+?\(?\d[\d\s().-]{8,20}\d/g, placeholder: "[PHONE]" },
  // An Indian PIN code after a comma or "-" (an address), not a year or a count.
  { kind: "pin", re: /(?<=[,-]\s?)\b[1-9]\d{5}\b/g, placeholder: "[PIN]" },
];

export function stripContacts(lines: Line[]): { lines: Line[]; contacts: Contact[] } {
  const contacts: Contact[] = [];
  const out = lines.map((line) => {
    let text = line.text;
    for (const p of patterns) {
      text = text.replace(p.re, (value) => {
        const digits = value.replace(/\D/g, "").length;
        if (p.kind === "phone" && (digits < 10 || digits > 15)) return value;
        contacts.push({ kind: p.kind, line: line.n, value });
        return p.placeholder;
      });
    }
    return { ...line, text };
  });
  return { lines: out, contacts };
}
