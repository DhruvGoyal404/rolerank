import { describe, expect, it } from "vitest";
import { experienceMonths, ground, ProfileSchema, type Profile } from "./profile.js";

const lines = [
  { n: 1, text: "Software Engineer, Acme Corp (2022-03 to 2024-06)" },
  { n: 2, text: "Built services in Go and PostgreSQL" },
  { n: 3, text: "B.Tech Computer Science, Thapar Institute, 2022" },
  { n: 4, text: "Project: RoleRank job matcher" },
  { n: 5, text: "Kubernetes expert", hidden: true },
];

type Role = Profile["experience"][number];
const role = (r: Partial<Role> & Pick<Role, "employer" | "title">): Role => ({ kind: "job", periods: [], lines: [1], ...r });
const empty: Profile = { level: "entry", skills: [], experience: [], education: [], projects: [], publications: [], achievements: [], certifications: [] };

const base: Profile = {
  ...empty,
  level: "mid",
  skills: [
    { name: "Go", line: 2 },
    { name: "PostgreSQL", line: 2 },
  ],
  experience: [role({ employer: "Acme Corp", title: "Software Engineer", periods: [{ start: "2022-03", end: "2024-06" }], lines: [1, 2] })],
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
        role({ employer: "Google", title: "Software Engineer", periods: [{ start: "2020", end: "2021" }] }),
        role({ employer: "Acme Corp", title: "Software Engineer", periods: [{ start: "2019-01", end: "present" }] }),
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
    const r = role({ employer: "Zoho", title: "Data Engineer", periods: [{ start: "2024-03", end: "present" }], lines: [1, 2] });
    expect(ground({ ...empty, experience: [r] }, awk).profile.experience).toHaveLength(1);
    const wrong = { ...r, periods: [{ start: "2023-03", end: "present" }] };
    expect(ground({ ...empty, experience: [wrong] }, awk).profile.experience).toHaveLength(0);
  });

  it("grounds an education entry whose values sit on separate lines", () => {
    const split = [
      { n: 1, text: "B.Tech in Computer Science" },
      { n: 2, text: "IIT Delhi" },
      { n: 3, text: "2024 · CGPA 8.10/10" },
    ];
    const e = { institution: "IIT Delhi", degree: "B.Tech in Computer Science", grad_year: 2024, score: "CGPA 8.10/10", lines: [1, 2, 3] };
    expect(ground({ ...empty, education: [e] }, split).profile.education).toHaveLength(1);
    expect(ground({ ...empty, education: [{ ...e, lines: [1] }] }, split).profile.education).toHaveLength(0);
  });

  it("refuses malformed dates and a period that ends before it starts", () => {
    const bad = [
      role({ employer: "Acme Corp", title: "Software Engineer", periods: [{ start: "March 2022", end: "2024-06" }] }),
      role({ employer: "Acme Corp", title: "Software Engineer", periods: [{ start: "2024-06", end: "2022-03" }] }),
    ];
    expect(ground({ ...base, experience: bad }, lines).profile.experience).toHaveLength(0);
  });

  // The owner's resume, 2026-11-06: "2nd Consecutive Year" came back as "2 Consecutive Year".
  it("requires names verbatim: an altered title, a fixed typo or a word inside a word fails", () => {
    const l = [{ n: 1, text: "Apprentice - ML Program (2nd Consecutive Year) Aug 2025 – Sept 2025" }];
    const r = role({ employer: "", title: "Apprentice - ML Program (2nd Consecutive Year)", periods: [{ start: "2025-08", end: "2025-09" }] });
    const ok = (title: string, employer = "ML Program") => ground({ ...empty, experience: [{ ...r, title, employer }] }, l).profile.experience.length === 1;
    expect(ok("Apprentice - ML Program (2nd Consecutive Year)")).toBe(true);
    expect(ok("apprentice  -  ml program (2nd consecutive year)")).toBe(true); // case and spacing only
    expect(ok("Apprentice - ML Program (2 Consecutive Year)")).toBe(false);
    expect(ok("Apprentice - ML Program 2nd Consecutive Year")).toBe(false);
    expect(ok("Apprentice - ML Program (2nd Consecutive Year)", "ML Programme")).toBe(false);
    const go = [{ n: 1, text: "Interned at Google" }];
    expect(ground({ ...empty, skills: [{ name: "Go", line: 1 }] }, go).profile.skills).toHaveLength(0);
    expect(ground({ ...empty, skills: [{ name: "C++", line: 1 }] }, [{ n: 1, text: "C++, Java" }]).profile.skills).toHaveLength(1);
  });

  it("grounds each of several periods on one line", () => {
    const l = [{ n: 1, text: "Research Intern, Acme Corp" }, { n: 2, text: "Aug 2025 – Sept 2025 & Jul 2026 – Aug 2026" }];
    const two = role({
      employer: "Acme Corp",
      title: "Research Intern",
      kind: "internship",
      periods: [
        { start: "2025-08", end: "2025-09" },
        { start: "2026-07", end: "2026-08" },
      ],
      lines: [1, 2],
    });
    expect(ground({ ...empty, experience: [two] }, l).profile.experience).toHaveLength(1);
    const invented = { ...two, periods: [...two.periods, { start: "2024-01", end: "2024-02" }] };
    expect(ground({ ...empty, experience: [invented] }, l).profile.experience).toHaveLength(0);
  });

  it("keeps school boards out of education, and says how many it removed", () => {
    const l = [
      { n: 1, text: "Thapar Institute, B.E. Computer Engineering, 2027" },
      { n: 2, text: "Modern Academy, ISC Intermediate PCM, 2023" },
      { n: 3, text: "Delhi Public School, CBSE Class XII, 2021" },
      { n: 4, text: "St. Mary's, 12th (HSC), 2021" },
    ];
    const e = (institution: string, degree: string, n: number, y: number) => ({ institution, degree, grad_year: y, score: "", lines: [n] });
    const g = ground(
      {
        ...empty,
        education: [
          e("Thapar Institute", "B.E. Computer Engineering", 1, 2027),
          e("Modern Academy", "ISC Intermediate PCM", 2, 2023),
          e("Delhi Public School", "CBSE Class XII", 3, 2021),
          e("St. Mary's", "12th (HSC)", 4, 2021),
        ],
      },
      l,
    );
    expect(g.profile.education.map((x) => x.institution)).toEqual(["Thapar Institute"]);
    expect(g.school).toBe(3);
    expect(g.dropped).toBe(0);
  });

  it("marks a course or programme as a programme, but not a Program Manager", () => {
    const l = [{ n: 1, text: "Deep Learning Summer School, IIIT Hyderabad 2025" }, { n: 2, text: "Program Manager, Zoho 2024" }];
    const g = ground(
      {
        ...empty,
        experience: [
          role({ employer: "IIIT Hyderabad", title: "Deep Learning Summer School", lines: [1] }),
          role({ employer: "Zoho", title: "Program Manager", lines: [2] }),
        ],
      },
      l,
    );
    expect(g.profile.experience.map((x) => x.kind)).toEqual(["programme", "job"]);
  });

  it("grounds publications, achievements and certifications like any value", () => {
    const l = [
      { n: 1, text: "CRF-based NER for Code-Mixed Text IEEE ICACCS 2026" },
      { n: 2, text: "– Winner, Smart India Hackathon 2025" },
      { n: 3, text: "– Certification: AWS Certified Cloud Practitioner" },
    ];
    const g = ground(
      {
        ...empty,
        publications: [{ title: "CRF-based NER for Code-Mixed Text", lines: [1] }, { title: "A paper never written", lines: [1] }],
        achievements: [{ text: "Winner, Smart India Hackathon 2025", lines: [2] }],
        certifications: [{ name: "AWS Certified Cloud Practitioner", lines: [3] }, { name: "AWS Certified Solutions Architect", lines: [3] }],
      },
      l,
    );
    expect([g.profile.publications.length, g.profile.achievements.length, g.profile.certifications.length]).toEqual([1, 1, 1]);
    expect(g.droppedBy).toEqual({ publications: 1, certifications: 1 });
  });
});

describe("ProfileSchema", () => {
  it("rejects extra fields and wrong types", () => {
    expect(ProfileSchema.safeParse({ ...base, years: 5 }).success).toBe(false);
    expect(ProfileSchema.safeParse({ ...base, level: "principal" }).success).toBe(false);
    expect(ProfileSchema.safeParse({ ...base, experience: [{ ...base.experience[0], kind: "volunteer" }] }).success).toBe(false);
    expect(ProfileSchema.safeParse(base).success).toBe(true);
  });
});

describe("experienceMonths", () => {
  const at = (periods: Role["periods"], kind: Role["kind"] = "job") => role({ employer: "A", title: "T", kind, periods });

  it("counts overlapping roles once", () => {
    const p = { experience: [at([{ start: "2022-01", end: "2023-01" }]), at([{ start: "2022-07", end: "2023-07" }])] };
    expect(experienceMonths(p)).toBe(18);
  });

  it("counts a role held twice as the union of its periods, not one long range", () => {
    const p = {
      experience: [
        at(
          [
            { start: "2025-08", end: "2025-09" },
            { start: "2026-07", end: "2026-08" },
          ],
          "internship",
        ),
      ],
    };
    expect(experienceMonths(p)).toBe(2);
  });

  it("leaves programmes out", () => {
    const p = { experience: [at([{ start: "2025-01", end: "2025-07" }], "programme"), at([{ start: "2025-06", end: "2025-08" }], "internship")] };
    expect(experienceMonths(p)).toBe(2);
  });
});
