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

const EMAIL = /[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}/g;
const URL = /\b(?:https?:\/\/|www\.)\S+|\b(?:linkedin\.com|github\.com|gitlab\.com|twitter\.com|x\.com|leetcode\.com|medium\.com)\/\S*/gi;
// In the contact header, any run of digits and separators holding 10 to 15 digits is a phone:
// +91 98765 43210, (555) 123-4567.
const ANY_PHONE = /\+?\(?\d[\d\s().-]{8,20}\d/g;
// Elsewhere only a real phone format: an Indian mobile (optional +91 or 0, then ten digits from
// 6-9, spaces or dashes allowed) or a number written with its "+" country code. A 12-digit patent
// application number is neither (owner's resume, 2026-11-07).
const PHONE = /(?<![\d+])(?:(?:\+91|0)[\s-]?)?[6-9]\d{4}[\s-]?\d{5}(?!\d)|\+\d{1,3}[\s-]?\(?\d{2,5}\)?(?:[\s-]?\d{2,5}){1,3}(?!\d)/g;
// A line that labels its number as an application, publication, roll, registration or ID number
// holds no phone, whatever its digits look like.
const LABELLED =
  /\b(application|publication|patent|roll|registration|regn?|enrol(?:l)?ment|admission|student|employee|candidate|certificate|credential|licen[cs]e|membership|isbn|issn|doi)\.?\s*(no|number|num|#|id)\b|\bID\s*(no\b|number\b|[:#])/i;
// An Indian PIN code after a comma or "-" (an address), not a year or a count.
const PIN = /(?<=[,-]\s?)\b[1-9]\d{5}\b/g;
/** The contact header: the first lines of the resume, where names, phones and emails sit. */
const HEADER_LINES = 5;

export function stripContacts(lines: Line[]): { lines: Line[]; contacts: Contact[] } {
  const contacts: Contact[] = [];
  const out = lines.map((line, i) => {
    let text = line.text;
    const sub = (kind: Contact["kind"], re: RegExp, placeholder: string, ok: (v: string) => boolean = () => true) => {
      text = text.replace(re, (value) => {
        if (!ok(value)) return value;
        contacts.push({ kind, line: line.n, value });
        return placeholder;
      });
    };
    const digits = (v: string) => v.replace(/\D/g, "").length;
    sub("email", EMAIL, "[EMAIL]");
    sub("url", URL, "[URL]");
    if (!LABELLED.test(text)) {
      if (i < HEADER_LINES) sub("phone", ANY_PHONE, "[PHONE]", (v) => digits(v) >= 10 && digits(v) <= 15);
      sub("phone", PHONE, "[PHONE]", (v) => digits(v) >= 10 && digits(v) <= 15);
    }
    sub("pin", PIN, "[PIN]");
    return { ...line, text };
  });
  return { lines: out, contacts };
}
