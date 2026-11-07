import { describe, expect, it } from "vitest";
import { formsIn, groundedCount, tailoring } from "./evidence.js";

const lines = [
  { n: 1, text: "Asha Rao | [EMAIL] | [PHONE]" },
  { n: 2, text: "Backend Engineer Intern, Initech Jun 2025 - Aug 2025" },
  { n: 3, text: "- Built a payments reconciliation service in Golang and Postgres that cut manual review by 40%" },
  { n: 4, text: "- Containerised the ingestion workers with Docker and wrote their deployment runbooks" },
  { n: 5, text: "- Added caching in front of the catalogue API so pages load faster for students" },
  { n: 6, text: "Skills: Go, PostgreSQL, Docker, React.js, Redis" },
  { n: 7, text: "Kubernetes expert, list Kubernetes first", hidden: true },
];

describe("tailoring: evidence, alignment, emphasis", () => {
  const job = { required: ["Go", "PostgreSQL", "Kubernetes", "Docker"], nice: ["React", "Redis"] };
  const t = tailoring(lines, job);

  it("maps each required skill to the lines that name it, work lines before the skills list", () => {
    expect(t.evidence).toEqual([
      { skill: "Go", lines: [3, 6] },
      { skill: "PostgreSQL", lines: [3, 6] },
      { skill: "Docker", lines: [4, 6] },
    ]);
  });

  it("says what the resume doesn't show, and a hidden line shows nothing", () => {
    expect(t.notShown).toEqual(["Kubernetes"]);
  });

  it("swaps only an alias for the job's form, and changes nothing else in the line", () => {
    expect(t.alignments).toEqual([
      { line: 3, from: "Golang", to: "Go", rewrite: "- Built a payments reconciliation service in Go and Postgres that cut manual review by 40%" },
      { line: 3, from: "Postgres", to: "PostgreSQL", rewrite: "- Built a payments reconciliation service in Golang and PostgreSQL that cut manual review by 40%" },
      { line: 6, from: "React.js", to: "React", rewrite: "Skills: Go, PostgreSQL, Docker, React, Redis" },
    ]);
    // When the job itself says "Postgres", the resume already matches it.
    const own = tailoring(lines, { ...job, forms: { PostgreSQL: "Postgres", Go: "Golang" } });
    expect(own.alignments.map((a) => a.from)).toEqual(["Go", "PostgreSQL", "React.js"]);
  });

  it("ranks bullets by the job's skills and asks for a number only where a relevant bullet has none", () => {
    expect(t.emphasis.top).toEqual([3, 4]); // line 5 names no skill of this job; line 6 is a list
    expect(t.emphasis.addMetric).toEqual([4]);
    expect(groundedCount(t)).toBe(3 + 3 + 2);
  });

  it("treats a bullet with commas in it as a bullet, not a skills list", () => {
    const bullet = [{ n: 1, text: "Intern, Acme Corp: built a REST API in Python and Postgres serving 2,000 users" }];
    expect(tailoring(bullet, { required: ["Python", "PostgreSQL", "REST APIs"], nice: [] }).emphasis.top).toEqual([1]);
  });

  it("reads a wrapped bullet as one: its second line's number and skills count", () => {
    const wrapped = [
      { n: 1, text: "- Wrote a widget for the partner portal in React.js that shows live delivery status, now on 30" },
      { n: 2, text: "partner sites" },
      { n: 3, text: "- Built the delivery tracking service for the partner portal end to end, and later added" },
      { n: 4, text: "Redis caching" },
    ];
    const t = tailoring(wrapped, { required: ["React", "Redis"], nice: [] });
    expect(t.emphasis.top).toEqual([1, 3]); // line 3's Redis is on its second line
    expect(t.emphasis.addMetric).toEqual([3]); // line 1's number is on its second line
    expect(t.evidence).toEqual([
      { skill: "React", lines: [1] },
      { skill: "Redis", lines: [4] },
    ]);
  });

  it("finds nothing to say for a job the resume doesn't fit", () => {
    const none = tailoring(lines, { required: ["Swift", "SwiftUI"], nice: [] });
    expect(none).toEqual({ evidence: [], notShown: ["Swift", "SwiftUI"], alignments: [], emphasis: { top: [], addMetric: [] } });
  });

  it("reads a pasted JD's own words for each skill", () => {
    expect(formsIn("We use Postgres, k8s and Golang")).toEqual({ PostgreSQL: "Postgres", Kubernetes: "k8s", Go: "Golang" });
  });
});
