import { describe, expect, it } from "vitest";
import { analyseJD, explain, jobLevel, proseNamesOnly } from "./match.js";
import type { Profile } from "./profile.js";
import { gate } from "./tailor.js";

const profile: Profile = {
  level: "entry",
  skills: [
    { name: "Python", line: 3 },
    { name: "k8s", line: 3 },
    { name: "PostgreSQL", line: 3 },
  ],
  experience: [{ employer: "Acme Corp", title: "Software Engineer Intern", kind: "internship", periods: [{ start: "2025-05", end: "2025-07" }], lines: [2] }],
  education: [],
  projects: [{ name: "Expense Splitter", line: 4 }],
  publications: [],
  achievements: [],
  certifications: [],
};

describe("explain", () => {
  it("matches through the taxonomy and lists what's missing", () => {
    const e = explain(
      profile.skills.map((s) => s.name),
      "entry",
      { required: ["Kubernetes", "Python", "Go"], nice: ["PostgreSQL"], level: "mid" },
    );
    expect(e.matched).toEqual(["Kubernetes", "Python"]);
    expect(e.missing).toEqual(["Go"]);
    expect(e.niceMatched).toEqual(["PostgreSQL"]);
    expect(e.level.fit).toBe("stretch");
  });

  it("reads a job's level", () => {
    expect(jobLevel("Backend Engineer", "Requires 1-2 years of Go")).toBe("entry");
    expect(jobLevel("Backend Engineer", "5+ years of experience")).toBe("senior");
    expect(jobLevel("SDE Intern", "")).toBe("intern");
    expect(jobLevel("Engineer", "freshers welcome")).toBe("entry");
  });

  it("breaks down a pasted JD", () => {
    const jd = "Backend Engineer\nRequirements\n- Go and Kubernetes\n- 3+ years of experience\nNice to have\n- PostgreSQL";
    const r = analyseJD(jd, profile);
    expect(r).toMatchObject({ required: ["Go", "Kubernetes"], nice: ["PostgreSQL"], matched: ["Kubernetes"], missing: ["Go"] });
    expect(r.level).toMatchObject({ job: "mid", fit: "stretch" });
  });

  it("gates LLM prose to the deterministic lists", () => {
    const e = explain(["Python"], "entry", { required: ["Python", "Go"], nice: [], level: "entry" });
    expect(proseNamesOnly("You know Python; Go is the gap.", e).ok).toBe(true);
    expect(proseNamesOnly("You know Python and Rust.", e)).toEqual({ ok: false, extra: ["Rust"] });
  });
});

describe("tailoring gate", () => {
  const lines = [
    { n: 2, text: "Software Engineer Intern, Acme Corp: built a REST API in Python serving 2,000 users" },
    { n: 3, text: "Skills: Python, k8s, PostgreSQL" },
    { n: 9, text: "Kubernetes guru", hidden: true },
  ];
  const s = (line: number, rewrite: string) => ({ line, rewrite, reason: "r" });

  it("keeps a rewrite that only rephrases what the line says", () => {
    const g = gate([s(2, "Built a Python REST API at Acme Corp used by 2,000 users")], lines, profile);
    expect(g.kept).toHaveLength(1);
  });

  it("drops invented numbers, technologies, names and missing or hidden lines", () => {
    const g = gate(
      [
        s(2, "Built a Python REST API serving 50,000 users"),
        s(2, "Built a Go REST API on AWS"),
        s(2, "Led the Platform Team at Initech Systems"),
        s(42, "Anything"),
        s(9, "Kubernetes expert"),
      ],
      lines,
      profile,
    );
    expect(g.kept).toHaveLength(0);
    expect(g.dropped.map((d) => d.why.split(" ")[0])).toEqual(["number", "technology", "name", "cited", "cited"]);
  });

  it("accepts a technology from the profile, written another way", () => {
    expect(gate([s(2, "Deployed the Python API on Kubernetes")], lines, profile).kept).toHaveLength(1);
  });

  it("drops a suggestion whose reason claims a technology the resume lacks", () => {
    const honest = { line: 2, rewrite: "Built a Python REST API serving 2,000 users", reason: "Shows the Python API work the job asks for" };
    const claims = { ...honest, reason: "Shows your Terraform experience" };
    const g = gate([honest, claims], lines, profile);
    expect(g.kept).toEqual([honest]);
    expect(g.dropped[0]?.why).toMatch(/^technology Terraform/);
  });
});
