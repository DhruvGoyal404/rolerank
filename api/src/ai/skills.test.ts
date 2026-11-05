import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { taxonomy } from "./skills.js";

// Written by the Go matcher: go run ./cmd/skillfixture > ../api/src/ai/testdata/skills.json
const fixture = JSON.parse(readFileSync(new URL("./testdata/skills.json", import.meta.url), "utf8")) as {
  find: Array<{ text: string; want: string[] }>;
  extract: Array<{ body: string; required: string[]; nice: string[] }>;
};

describe("skill matcher, TypeScript port", () => {
  it("loads the shared taxonomy", () => {
    expect(taxonomy().names.length).toBeGreaterThan(400);
  });

  it.each(fixture.find)("finds what Go finds in $text", ({ text, want }) => {
    expect(taxonomy().find(text)).toEqual(want);
  });

  it.each(fixture.extract)("splits required and nice as Go does", ({ body, required, nice }) => {
    expect(taxonomy().extract(body)).toEqual({ required, nice });
  });
});
