import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

/**
 * Every M story of SPEC.md that v1 ships, through the real UI, against the e2e API on isolated
 * _test databases (playwright.config.ts). Each signed-in page is also checked with axe at WCAG 2.1
 * AA, in light and dark, with no console errors and no CSP violations.
 */
let n = 0;
async function signIn(page: Page): Promise<void> {
  const subject = `e2e-${String(Date.now())}-${String(++n)}`;
  const seeded = await page.request.post("http://127.0.0.1:8079/seed", { data: { subject } });
  expect(seeded.ok()).toBe(true);
  const login = await page.request.post("/test/login", { data: { subject }, headers: { Origin: "http://localhost:5173" } });
  expect(login.status()).toBe(204);
}

function watch(page: Page): string[] {
  const problems: string[] = [];
  page.on("console", (m) => {
    if (m.type() === "error" && !/status of 40[19]/.test(m.text())) problems.push(m.text());
  });
  page.on("pageerror", (e) => problems.push(e.message));
  return problems;
}

async function axe(page: Page, label: string): Promise<void> {
  for (const scheme of ["light", "dark"] as const) {
    await page.emulateMedia({ colorScheme: scheme });
    const r = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"]).analyze();
    expect(r.violations, `${label} (${scheme})`).toEqual([]);
  }
  await page.emulateMedia({ colorScheme: "light" });
}

const PDF = { name: "resume.pdf", mimeType: "application/pdf", buffer: Buffer.from("%PDF-1.4 resume") };

async function uploadAndConfirm(page: Page): Promise<void> {
  await page.goto("/");
  await expect(page).toHaveURL(/\/resume$/);
  await page.getByLabel("PDF or Word (.docx), up to 5 MB").setInputFiles(PDF);
  await page.getByRole("button", { name: "Upload" }).click();
  await expect(page.getByRole("heading", { name: "Check what we read" })).toBeVisible({ timeout: 30_000 });
  await page.getByRole("button", { name: "Confirm and see matches" }).click();
  await expect(page).toHaveURL(/\/matches$/);
}

test("upload, correct and confirm a profile; matches with bands, explanations and credits", async ({ page }) => {
  const problems = watch(page);
  await signIn(page);
  await page.goto("/");
  await expect(page).toHaveURL(/\/resume$/);
  await axe(page, "upload");

  // Upload: a structured profile within 30 seconds.
  const started = Date.now();
  await page.getByLabel("PDF or Word (.docx), up to 5 MB").setInputFiles(PDF);
  await page.getByRole("button", { name: "Upload" }).click();
  await expect(page.getByRole("heading", { name: "Check what we read" })).toBeVisible({ timeout: 30_000 });
  expect(Date.now() - started).toBeLessThan(30_000);
  await axe(page, "review");

  // Correct what was read: remove a skill, add one, change the level. The confirmed profile drives matching.
  const skills = page.getByRole("list", { name: "Your skills" });
  await expect(skills.getByText("Redis")).toBeVisible();
  await page.getByRole("button", { name: "Remove Redis" }).click();
  await expect(skills.getByText("Redis")).toHaveCount(0);
  await page.getByLabel("Add a skill your resume shows").fill("Kafka");
  await page.getByRole("button", { name: "Add", exact: true }).click();
  await expect(skills.getByText("Kafka")).toBeVisible();
  await page.getByLabel("Career stage").selectOption("entry");
  await page.getByRole("button", { name: "Confirm and see matches" }).click();
  await expect(page).toHaveURL(/\/matches$/);

  // Matches: ranked, with a fit band, skills had and missing, level, credit and freshness.
  const cards = page.locator(".match");
  await expect(cards).toHaveCount(4);
  const first = cards.first();
  await expect(first.getByRole("heading", { name: "Backend Engineer Intern" })).toBeVisible();
  await expect(first.getByText("Strong fit")).toBeVisible();
  await expect(first.getByRole("list", { name: "Required skills you have" })).toContainText("Go");
  await expect(first.getByRole("list", { name: "Required skills your resume doesn't show" })).toContainText("Kubernetes");
  await expect(first.getByRole("link", { name: "Greenhouse" })).toHaveAttribute("href", "https://jobs.example.com/greenhouse/1");
  await expect(first.getByText("posted 2 days ago")).toBeVisible();
  await expect(page.getByText(/\d+\s*%/)).toHaveCount(0); // a band, never a percentage
  await expect(cards.nth(3).getByRole("link", { name: "Remote OK" })).toHaveAttribute("rel", "noopener");
  await axe(page, "matches");

  // Filters narrow the ranked list: internships, location, company, freshness.
  await page.getByLabel("Internships only").check();
  await expect(cards).toHaveCount(2);
  await page.getByLabel("Internships only").uncheck();
  await page.getByLabel("Location").fill("Pune");
  await expect(cards).toHaveCount(1);
  await page.getByLabel("Location").fill("");
  await page.getByLabel("Company").fill("acme");
  await expect(cards).toHaveCount(1);
  await page.getByLabel("Company").fill("");
  await page.getByLabel("Posted").selectOption("7");
  await expect(cards).toHaveCount(3); // the 40-day-old posting drops out
  await page.getByLabel("Posted").selectOption("0");
  expect(problems).toEqual([]);
});

test("a job: credit and apply link, an explanation on demand, grounded tailoring, the tracker", async ({ page }) => {
  const problems = watch(page);
  await signIn(page);
  await uploadAndConfirm(page);
  await page.getByRole("link", { name: "Frontend Developer" }).click();
  // Himalayas: apply goes to their applicationLink, unchanged.
  await expect(page.getByRole("link", { name: "Apply on the employer's site" })).toHaveAttribute(
    "href",
    "https://himalayas.app/companies/acme/jobs/frontend?ref=himalayas&x=1",
  );
  await page.goBack();
  await page.getByRole("link", { name: "Backend Engineer Intern" }).click();
  await expect(page.getByRole("heading", { name: "How you fit" })).toBeVisible();
  await page.getByRole("button", { name: "Explain this match in words" }).click();
  await expect(page.getByText("It also asks for Kubernetes, which your resume doesn't show.")).toBeVisible();

  // Tailoring: where each skill shows, the job's words, what to put first; never invented.
  await page.getByRole("button", { name: "Tailor my resume" }).click();
  const evidence = page.getByRole("region", { name: "Where your resume shows each skill" });
  await expect(evidence).toContainText("Built a payments service in Golang and Postgres");
  await expect(evidence.getByRole("row", { name: /Not shown in your resume/ })).toContainText("Kubernetes");
  const swaps = page.getByRole("region", { name: "Use the job's words" });
  await expect(swaps).toContainText("Golang");
  await expect(swaps).toContainText("Postgres");
  await expect(page.getByRole("region", { name: "Put these first" })).toBeVisible();
  await axe(page, "job");

  // Tracker: save, move through statuses, keep it.
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page.getByRole("button", { name: "Saved to tracker" })).toBeVisible();
  await page.getByRole("link", { name: "Tracker" }).click();
  const status = page.getByLabel("Status of Backend Engineer Intern");
  await expect(status).toHaveValue("saved");
  // The change shows at once; wait for it to be stored before reloading.
  await Promise.all([page.waitForResponse((r) => r.request().method() === "PATCH" && r.url().includes("/api/tracker/")), status.selectOption("applied")]);
  await page.reload();
  await expect(page.getByLabel("Status of Backend Engineer Intern")).toHaveValue("applied");
  await axe(page, "tracker");
  await page.getByRole("button", { name: "Remove Backend Engineer Intern" }).click();
  await expect(page.getByText("Nothing saved yet.")).toBeVisible();
  expect(problems).toEqual([]);
});

test("Bring a JD: the same breakdown and tailoring, and nothing pasted is kept", async ({ page }) => {
  const problems = watch(page);
  await signIn(page);
  await uploadAndConfirm(page);
  await page.getByRole("link", { name: "Bring a JD" }).click();
  await page.getByLabel("Job description").fill("Platform Engineer\nRequirements\n- Golang and Kubernetes\n- Docker\nNice to have\n- Terraform");
  await page.getByRole("button", { name: "See how I fit" }).click();
  await expect(page.getByRole("list", { name: "Required skills you have" })).toContainText("Docker");
  await expect(page.getByRole("list", { name: "Required skills your resume doesn't show" })).toContainText("Kubernetes");
  await page.getByRole("button", { name: "Tailor my resume for it" }).click();
  await expect(page.getByRole("region", { name: "Where your resume shows each skill" })).toContainText("Containerised the ingestion workers with Docker");
  // The JD writes "Golang": the skills line's "Go" gets that one swap, and the bullet that
  // already says "Golang" gets none.
  const swaps = page.getByRole("region", { name: "Use the job's words" });
  await expect(swaps.getByRole("listitem")).toHaveCount(1);
  await expect(swaps).toContainText("Skills: Golang, PostgreSQL, Docker, React, Redis");
  await axe(page, "bring a JD");
  expect(problems).toEqual([]);
});

test("an unreadable file says why and what to do", async ({ page }) => {
  await signIn(page);
  await page.goto("/resume");
  await page.getByLabel("PDF or Word (.docx), up to 5 MB").setInputFiles({ name: "scan.pdf", mimeType: "application/pdf", buffer: Buffer.from("%PDF-1.4 SCANNED") });
  await page.getByRole("button", { name: "Upload" }).click();
  await expect(page.getByRole("alert")).toContainText("no selectable text", { timeout: 30_000 });
  await page.getByLabel("PDF or Word (.docx), up to 5 MB").setInputFiles({ name: "locked.pdf", mimeType: "application/pdf", buffer: Buffer.from("%PDF-1.4 PASSWORD") });
  await page.getByRole("button", { name: "Upload" }).click();
  await expect(page.getByRole("alert")).toContainText("password-protected", { timeout: 30_000 });
  await page.getByLabel("PDF or Word (.docx), up to 5 MB").setInputFiles({ name: "slow.pdf", mimeType: "application/pdf", buffer: Buffer.from("%PDF-1.4 SLOWSTART") });
  await page.getByRole("button", { name: "Upload" }).click();
  await expect(page.getByRole("alert")).toContainText("slow to start, not your file", { timeout: 30_000 });
  // Each check names itself: not a PDF, and too large.
  await page.getByLabel("PDF or Word (.docx), up to 5 MB").setInputFiles({ name: "notes.pdf", mimeType: "application/pdf", buffer: Buffer.from("hello, not a PDF") });
  await page.getByRole("button", { name: "Upload" }).click();
  await expect(page.getByRole("alert")).toContainText("Not a PDF or a Word file");
  await page.getByLabel("PDF or Word (.docx), up to 5 MB").setInputFiles({ name: "big.pdf", mimeType: "application/pdf", buffer: Buffer.concat([Buffer.from("%PDF-1.4"), Buffer.alloc(5 * 1024 * 1024)]) });
  await page.getByRole("button", { name: "Upload" }).click();
  await expect(page.getByRole("alert")).toContainText("File too large");
});

test("account: preferences, export and deletion of everything", async ({ page }) => {
  const problems = watch(page);
  await signIn(page);
  await uploadAndConfirm(page);
  await page.getByRole("link", { name: "Account" }).click();
  await page.getByLabel("Target roles").fill("Backend Engineer");
  await page.getByLabel("Also show remote roles open to India").uncheck();
  await page.getByRole("button", { name: "Save preferences" }).click();
  await expect(page.getByText("Saved.")).toBeVisible();
  await axe(page, "account");

  const download = page.waitForEvent("download");
  await page.getByRole("link", { name: "Export my data" }).click();
  const file = await download;
  const exported = JSON.parse(await (await file.createReadStream()).toArray().then((c) => Buffer.concat(c).toString())) as {
    preferences: { targetTitles: string };
    resumes: Array<{ lines: Array<{ text: string }> }>;
  };
  expect(exported.preferences.targetTitles).toBe("Backend Engineer");
  expect(exported.resumes[0]?.lines[2]?.text).toBe("Backend Engineering Intern, Initech");

  await expect(page.getByRole("button", { name: "Delete my account" })).toBeDisabled();
  await page.getByLabel("Type DELETE to confirm").fill("DELETE");
  await page.getByRole("button", { name: "Delete my account" }).click();
  await expect(page).toHaveURL("http://localhost:5173/");
  expect((await page.request.get("/api/me")).status()).toBe(401);
  expect(problems).toEqual([]);
});

test("signed-in pages work on a 360 px phone", async ({ page }) => {
  await signIn(page);
  await uploadAndConfirm(page);
  await page.setViewportSize({ width: 360, height: 740 });
  for (const path of ["/matches", "/jd", "/tracker", "/account", "/resume"]) {
    await page.goto(path);
    await expect(page.locator("h1")).toBeVisible();
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth);
    expect(overflow, path).toBe(false);
  }
});

test("the theme switch flips light and dark, and is remembered", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "light" });
  await page.goto("/");
  await page.getByRole("button", { name: "Switch to dark theme" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await expect(page.getByRole("button", { name: "Switch to light theme" })).toBeVisible();
});

test("signing out ends the session and goes home", async ({ page }) => {
  await signIn(page);
  await uploadAndConfirm(page);
  await page.getByRole("button", { name: "Sign out" }).click();
  await expect(page).toHaveURL("http://localhost:5173/");
  await expect(page.getByRole("link", { name: "Get started" })).toBeVisible();
  expect((await page.request.get("/api/me")).status()).toBe(401);
});
