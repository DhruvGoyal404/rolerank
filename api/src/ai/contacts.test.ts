import { describe, expect, it } from "vitest";
import { stripContacts } from "./contacts.js";

describe("stripContacts", () => {
  const strip = (text: string) => stripContacts([{ n: 1, text }]);

  it.each([
    ["jane.doe+jobs@example.co.in", "[EMAIL]"],
    ["+91 98765 43210", "[PHONE]"],
    ["098765-43210", "[PHONE]"],
    ["(555) 123-4567 x", "[PHONE] x"],
    ["linkedin.com/in/jane-doe", "[URL]"],
    ["https://github.com/janedoe/rolerank", "[URL]"],
    ["Sector 21, Gurugram - 122015", "Sector 21, Gurugram - [PIN]"],
  ])("removes %s", (input, want) => {
    const out = strip(input);
    expect(out.lines[0]?.text).toBe(want);
    expect(out.contacts).toHaveLength(1);
    expect(out.contacts[0]?.value).not.toContain("[");
  });

  it("leaves dates, years and counts alone", () => {
    for (const text of ["2019 - 2023", "Built a service handling 120000 requests a day", "B.Tech, 2026"]) {
      expect(strip(text).lines[0]?.text).toBe(text);
    }
  });

  // Below the header, only real phone formats count (owner's resume, 2026-11-07).
  const body = (text: string) => stripContacts([1, 2, 3, 4, 5].map((n) => ({ n, text: "x" })).concat({ n: 6, text })).lines[5]?.text;

  it.each([
    "Indian Patent Application (Published) | Application No. 202611078951 | Publication No. 32/2026 Aug 2026",
    "University Enrolment No. 1022034567",
    "Roll No. 9876543210",
    "Registration Number: 9812345678",
    "Student ID: 98123 45678",
    "Achieved 1234567890123 total requests served",
    "(555) 123-4567 on a later line is not an Indian mobile",
  ])("keeps a number that isn't a phone: %s", (text) => {
    expect(body(text)).toBe(text);
  });

  it.each([
    ["Call 9876543210 after 6", "Call [PHONE] after 6"],
    ["Mobile: +91-98765-43210", "Mobile: [PHONE]"],
    ["Alt. 098765 43210", "Alt. [PHONE]"],
    ["Reach me on +44 20 7946 0958", "Reach me on [PHONE]"],
  ])("still removes a phone below the header: %s", (text, want) => {
    expect(body(text)).toBe(want);
  });

  it("keeps line numbers and flags", () => {
    const out = stripContacts([{ n: 7, text: "Mail jane@x.io", hidden: true }]);
    expect(out.lines[0]).toEqual({ n: 7, text: "Mail [EMAIL]", hidden: true });
    expect(out.contacts[0]).toEqual({ kind: "email", line: 7, value: "jane@x.io" });
  });
});
