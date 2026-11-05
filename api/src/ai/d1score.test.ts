import { describe, expect, it } from "vitest";
import { add, precision, recall, score, type Gold, type GoldIn } from "./d1score.js";
import type { Profile } from "./profile.js";

const gold: Gold = {
  id: "D1-001",
  segment: "fresher",
  skills: ["Go", "PostgreSQL", "Docker"],
  experience: [{ employer: "Zoho", title: "Software Engineer", start: "2023-04", end: "present" }],
  education: [{ institution: "IIT Delhi", degree: "B.Tech in Computer Science", grad_year: 2023, score: "" }],
  projects: ["URL Shortener"],
};

const profile: Profile = {
  level: "entry",
  skills: [
    { name: "go", line: 1 },
    { name: "PostgreSQL", line: 1 },
    { name: "Rust", line: 1 },
  ],
  experience: [
    { employer: "Zoho", title: "Software Engineer", start: "2023-04", end: "present", lines: [2] },
    { employer: "Google", title: "Software Engineer", start: "2021-01", end: "2022-01", lines: [2] },
  ],
  education: [{ institution: "IIT Delhi", degree: "B.Tech in Computer Science", grad_year: 2023, score: "", line: 3 }],
  projects: [{ name: "Project: URL Shortener", line: 4 }],
};

describe("score", () => {
  it("counts per field and flags invented values", () => {
    const s = score(profile, gold);
    expect(s.counts.skills).toEqual({ extracted: 3, gold: 3, correct: 2 });
    expect(s.counts.employer).toEqual({ extracted: 2, gold: 1, correct: 1 });
    expect(s.counts.dates).toEqual({ extracted: 4, gold: 2, correct: 2 });
    expect(s.counts.projects).toEqual({ extracted: 1, gold: 1, correct: 1 });
    expect(s.fabricated).toBe(3); // Google, 2021-01, 2022-01; the title is in the gold
    expect(precision(s.counts.employer)).toBe(0.5);
    expect(recall(s.counts.skills)).toBeCloseTo(2 / 3);
    expect(add(s.counts, s.counts).skills).toEqual({ extracted: 6, gold: 6, correct: 4 });
  });

  it("reads a null list in the gold as empty", () => {
    const g: GoldIn = { ...gold, projects: null, experience: null };
    const s = score({ ...profile, experience: [], projects: [] }, g);
    expect(s.counts.projects).toEqual({ extracted: 0, gold: 0, correct: 0 });
    expect(s.counts.employer.gold).toBe(0);
  });
});
