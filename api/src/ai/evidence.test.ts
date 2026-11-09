import { describe, expect, it } from "vitest";
import { formsIn, groundedCount, isLinkLine, tailoring, units } from "./evidence.js";

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
      { skill: "Redis", lines: [3] }, // the bullet, named by its first line
    ]);
  });

  it("finds a skill whose name is split by the wrap", () => {
    const wrapped = [
      { n: 1, text: "- Built REST endpoints in Spring Boot for the loan flow and wrote the integration" },
      { n: 2, text: "tests that run on every merge" },
    ];
    const t = tailoring(wrapped, { required: ["Integration Testing"], nice: [] });
    expect(t.evidence).toEqual([{ skill: "Integration Testing", lines: [1] }]);
    expect(t.notShown).toEqual([]);
  });

  it("finds nothing to say for a job the resume doesn't fit", () => {
    const none = tailoring(lines, { required: ["Swift", "SwiftUI"], nice: [] });
    expect(none).toEqual({ evidence: [], notShown: ["Swift", "SwiftUI"], alignments: [], emphasis: { top: [], addMetric: [] } });
  });

  it("reads a pasted JD's own words for each skill", () => {
    expect(formsIn("We use Postgres, k8s and Golang")).toEqual({ PostgreSQL: "Postgres", Kubernetes: "k8s", Go: "Golang" });
  });
});

// Owner's walkthrough, 2026-11-09. Every value below is invented, in the shape of a real resume.
describe("evidence never comes from headers, links or merged sections", () => {
  const resume = [
    { n: 1, text: "Kabir Sethi" },
    { n: 2, text: "[PHONE] | [EMAIL] | LinkedIn | GitHub | LeetCode | Codeforces | DSA Portfolio | ORCID" },
    { n: 3, text: "EXPERIENCE" },
    { n: 4, text: "- Kept the team's release branches tidy with Git rebase workflows and pre-commit hooks" },
    { n: 5, text: "- Mirrored the course projects to GitHub and wrote their READMEs for new contributors" },
    { n: 6, text: "ACHIEVEMENTS & CERTIFICATIONS" },
    { n: 7, text: "- Filed a design patent for a low-cost soil moisture probe (Application No. 202511004321)" },
    { n: 8, text: "Finalist, BuildFest 2025 | 36-hour national hackathon on civic tech" },
    { n: 9, text: "Winner, Smart Campus Challenge 2024, team of four, problem on data structures for bus routing" },
    { n: 10, text: "Letter of appreciation from the Meesho Grid team for a ranking prototype" },
    { n: 11, text: "Certificate of completion, Advanced Java, Coding Ninjas" },
    { n: 12, text: "Mar 2024 - Jun 2024 Teaching assistant, Data Structures lab" },
  ];
  const t = tailoring(resume, { required: ["Git", "Data Structures", "Java"], nice: [] });

  it("never uses the contact or link line as evidence", () => {
    expect(isLinkLine(resume[1]?.text ?? "")).toBe(true);
    for (const e of t.evidence) expect(e.lines).not.toContain(2);
  });

  it("never counts a platform name (GitHub) as the skill (Git), and never swaps it", () => {
    expect(t.evidence.find((e) => e.skill === "Git")?.lines).toEqual([4]);
    expect(t.alignments.filter((a) => /github/i.test(a.from))).toEqual([]);
  });

  it("keeps each entry of a section its own unit", () => {
    expect(units(resume).map((u) => u.n)).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12]);
    expect(t.evidence.find((e) => e.skill === "Data Structures")?.lines).toEqual([9, 12]);
    expect(t.evidence.find((e) => e.skill === "Java")?.lines).toEqual([11]);
  });

  it("still joins a true wrap after a bullet, and stops at a heading or a dated line", () => {
    const wrapped = [
      { n: 1, text: "- Moved the attendance service from cron scripts to a queue worker written in Go, and" },
      { n: 2, text: "Redis streams for retries" },
      { n: 3, text: "- Built the timetable export for 1,200 students with a small Python service" },
      { n: 4, text: "PROJECTS" },
      { n: 5, text: "- Wrote a parser for the bus timetable PDFs and published the schedule as a" },
      { n: 6, text: "simple JSON API" },
      { n: 7, text: "Jan 2023 - Apr 2023" },
    ];
    expect(units(wrapped).map((u) => u.n)).toEqual([1, 3, 4, 5, 7]);
  });

  it("treats a short skills line with pipes as a list, not a header", () => {
    expect(isLinkLine("Python | SQL | Git | Docker")).toBe(false);
    expect(isLinkLine("Ananya Iyer | Bengaluru | [EMAIL] | [PHONE]")).toBe(true);
  });
});
