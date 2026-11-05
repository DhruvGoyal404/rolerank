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

  it("keeps line numbers and flags", () => {
    const out = stripContacts([{ n: 7, text: "Mail jane@x.io", hidden: true }]);
    expect(out.lines[0]).toEqual({ n: 7, text: "Mail [EMAIL]", hidden: true });
    expect(out.contacts[0]).toEqual({ kind: "email", line: 7, value: "jane@x.io" });
  });
});
