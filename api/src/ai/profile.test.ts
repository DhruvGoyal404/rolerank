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
  education: [{ institution: "Thapar Institute", degree: "B.Tech Computer Science", grad_year: 2022, line: 3 }],
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
      education: [{ institution: "Thapar Institute", degree: "M.Tech", grad_year: 2022, line: 3 }],
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
