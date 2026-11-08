import { renderToString } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";
import type { JobCard } from "./api";
import { SOURCES } from "./attribution";
import { ApplyLink, Band, Via } from "./components";

/**
 * Each source's attribution rule (docs/SOURCES.md), checked on the rendered card: the credit
 * shown, where the links go, and the rel each source's terms allow.
 */
const job = (source: string, applyUrl: string | null = null): JobCard => ({
  key: "1",
  title: "Backend Engineer",
  company: "Initech",
  location: "Bengaluru",
  level: "entry",
  url: `https://jobs.example.com/${source}/1`,
  applyUrl,
  excerpt: "",
  source,
  closed: false,
  postedAt: "2026-11-01T00:00:00Z",
});
const html = (el: React.ReactElement) => renderToString(<MemoryRouter>{el}</MemoryRouter>);

describe("attribution per source", () => {
  it("covers every source the ingester has", () => {
    expect(SOURCES.sort()).toEqual(["greenhouse", "himalayas", "lever", "remoteok"]);
  });

  it.each([
    ["greenhouse", "Greenhouse"],
    ["lever", "Lever"],
    ["himalayas", "Himalayas"],
    ["remoteok", "Remote OK"],
  ])("credits %s as \"via %s\", linking to the posting", (source, credit) => {
    const out = html(<Via job={job(source)} />);
    expect(out).toContain(`>${credit}</a>`);
    expect(out).toContain(`href="https://jobs.example.com/${source}/1"`);
  });

  it("gives Remote OK a followable link and no logo", () => {
    const out = html(<Via job={job("remoteok")} />);
    expect(out).toContain('rel="noopener"');
    expect(out).not.toContain("nofollow");
    expect(out).not.toContain("<img");
  });

  it("sends Himalayas applicants to their applicationLink, unchanged", () => {
    const link = "https://himalayas.app/companies/initech/jobs/backend?utm_source=rolerank&x=1";
    const out = html(<ApplyLink job={job("himalayas", link)} />);
    expect(out).toContain(`href="${link.replaceAll("&", "&amp;")}"`);
  });

  it("falls back to the posting when a source gives no apply link", () => {
    expect(html(<ApplyLink job={job("greenhouse")} />)).toContain('href="https://jobs.example.com/greenhouse/1"');
  });
});

it("shows a fit band, never a percentage", () => {
  for (const band of ["Strong", "Good", "Stretch"]) {
    const out = html(<Band band={band} />);
    expect(out).toContain(`${band} fit`);
    expect(out).not.toMatch(/\d+\s*%/);
  }
});
