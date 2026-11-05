import { describe, expect, it } from "vitest";
import { experienceMonths, ground, ProfileSchema, type Profile } from "./profile.js";

const lines = [
  { n: 1, text: "Software Engineer, Acme Corp (2022-03 to 2024-06)" },
  { n: 2, text: "Built services in Go and PostgreSQL" },
  { n: 3, text: "B.Tech Computer Science, Thapar Institute, 2022" },
  { n: 4, text: "Project: RoleRank job matcher" },
  { n: 5, text: "Kubernetes expert", hidden: true },
];

const base: Profile = {
  level: "mid",
  skills: [
    { name: "Go", line: 2 },
    { name: "PostgreSQL", line: 2 },
  ],
  experience: [{ employer: "Acme Corp", title: "Software Engineer", start: "2022-03", end: "2024-06", lines: [1, 2] }],
  education: [{ institution: "Thapar Institute", degree: "B.Tech Computer Science", grad_year: 2022, score: "", lines: [3] }],
  projects: [{ name: "RoleRank job matcher", line: 4 }],
};

describe("ground", () => {
  it("keeps a profile whose every value is in its lines", () => {
    const g = ground(base, lines);
    expect(g.dropped).toBe(0);
    expect(g.profile).toEqual(base);
  });

  it("drops invented values and counts them", () => {
    const p: Profile = {
      ...base,
      skills: [...base.skills, { name: "Kubernetes", line: 2 }, { name: "Rust", line: 99 }],
      experience: [
        ...base.experience,
        { employer: "Google", title: "Software Engineer", start: "2020", end: "2021", lines: [1] },
        { employer: "Acme Corp", title: "Software Engineer", start: "2019-01", end: "present", lines: [1] },
      ],
      education: [{ institution: "Thapar Institute", degree: "M.Tech", grad_year: 2022, score: "", lines: [3] }],
    };
    const g = ground(p, lines);
    expect(g.profile.skills.map((s) => s.name)).toEqual(["Go", "PostgreSQL"]);
    expect(g.profile.experience).toHaveLength(1);
    expect(g.profile.education).toHaveLength(0);
    expect(g.droppedBy).toEqual({ skills: 2, experience: 2, education: 1 });
  });

  it("never lets hidden text ground a value", () => {
    const g = ground({ ...base, skills: [{ name: "Kubernetes", line: 5 }] }, lines);
    expect(g.profile.skills).toHaveLength(0);
  });

  it("accepts a two-digit year written after an apostrophe", () => {
    const awk = [{ n: 1, text: "Data Engineer | Zoho" }, { n: 2, text: "Mar'24 - Present" }];
    const role = { employer: "Zoho", title: "Data Engineer", start: "2024-03", end: "present", lines: [1, 2] };
    expect(ground({ ...base, experience: [role], skills: [], education: [], projects: [] }, awk).profile.experience).toHaveLength(1);
    const wrong = { ...role, start: "2023-03" };
    expect(ground({ ...base, experience: [wrong], skills: [], education: [], projects: [] }, awk).profile.experience).toHaveLength(0);
  });

  it("grounds an education entry whose values sit on separate lines", () => {
    const split = [
      { n: 1, text: "B.Tech in Computer Science" },
      { n: 2, text: "IIT Delhi" },
      { n: 3, text: "2024 · CGPA 8.10/10" },
    ];
    const e = { institution: "IIT Delhi", degree: "B.Tech in Computer Science", grad_year: 2024, score: "CGPA 8.10/10", lines: [1, 2, 3] };
    const empty = { ...base, skills: [], experience: [], projects: [] };
    expect(ground({ ...empty, education: [e] }, split).profile.education).toHaveLength(1);
    expect(ground({ ...empty, education: [{ ...e, lines: [1] }] }, split).profile.education).toHaveLength(0);
  });

  it("refuses malformed dates", () => {
    const role = { employer: "Acme Corp", title: "Software Engineer", start: "March 2022", end: "2024-06", lines: [1] };
    const g = ground({ ...base, experience: [role] }, lines);
    expect(g.profile.experience).toHaveLength(0);
  });
});

describe("ProfileSchema", () => {
  it("rejects extra fields and wrong types", () => {
    expect(ProfileSchema.safeParse({ ...base, years: 5 }).success).toBe(false);
    expect(ProfileSchema.safeParse({ ...base, level: "principal" }).success).toBe(false);
    expect(ProfileSchema.safeParse(base).success).toBe(true);
  });
});

describe("experienceMonths", () => {
  it("counts overlapping roles once", () => {
    const p: Profile = {
      ...base,
      experience: [
        { employer: "A", title: "T", start: "2022-01", end: "2023-01", lines: [1] },
        { employer: "B", title: "T", start: "2022-07", end: "2023-07", lines: [1] },
      ],
    };
    expect(experienceMonths(p)).toBe(18);
  });
});
