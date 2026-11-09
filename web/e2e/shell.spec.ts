import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

const pages = ["/", "/signin", "/signin?error=state", "/about/bot", "/sources", "/privacy", "/no-such-page"];

for (const path of pages) {
  test(`${path} has no accessibility, console or CSP problems`, async ({ page }) => {
    const problems: string[] = [];
    page.on("console", (message) => {
      if (message.type() === "error") problems.push(message.text());
    });
    page.on("pageerror", (error) => problems.push(error.message));

    await page.goto(path);
    await expect(page.locator("h1")).toBeVisible();
    for (const scheme of ["light", "dark"] as const) {
      await page.emulateMedia({ colorScheme: scheme });
      const results = await new AxeBuilder({ page })
        .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"])
        .analyze();
      expect(results.violations, scheme).toEqual([]);
    }
    // Signed out, /api/me answers 401: that's the page asking, not a problem.
    expect(problems.filter((p) => !/status of 401/.test(p))).toEqual([]);
  });
}

test("the strict CSP is really enforced", async ({ page }) => {
  await page.goto("/");
  const blocked = await page.evaluate(() => {
    const script = document.createElement("script");
    try {
      script.textContent = "window.injected = true";
      document.head.append(script);
    } catch {
      return "threw";
    }
    return (window as unknown as { injected?: boolean }).injected === true ? "ran" : "blocked";
  });
  expect(blocked).not.toBe("ran");
});

test("/about/bot shows the bot contact", async ({ page }) => {
  await page.goto("/about/bot");
  await expect(page.getByRole("link", { name: "bot@dhruvgoyal.tech" })).toHaveAttribute(
    "href",
    "mailto:bot@dhruvgoyal.tech",
  );
});

test("works on a 360 px phone", async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 740 });
  await page.goto("/signin");
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
  );
  expect(overflow).toBe(false);
});

// Walkthrough bug (2026-11-08): the CSP had no manifest-src, so default-src 'none' blocked the
// manifest. Chrome loads it here as it would to install the app, and any CSP block fails the test.
test("the web manifest loads under the CSP", async ({ page, context }) => {
  const blocked: string[] = [];
  page.on("console", (m) => {
    if (/manifest/i.test(m.text())) blocked.push(m.text());
  });
  await page.goto("/");
  const cdp = await context.newCDPSession(page);
  const manifest = (await cdp.send("Page.getAppManifest")) as { url: string; errors: unknown[]; data?: string };
  expect(manifest.url).toMatch(/\/manifest\.webmanifest$/);
  expect(manifest.errors).toEqual([]);
  expect(manifest.data ?? "").toContain('"name": "RoleRank"');
  expect(blocked).toEqual([]);
});

// Real files, never the app's "Page not found" (Caddy serves them by name in production).
test("robots.txt and sitemap.xml are real static files", async ({ request }) => {
  const robots = await request.get("/robots.txt");
  expect(robots.status()).toBe(200);
  expect(robots.headers()["content-type"]).toMatch(/^text\/plain/);
  const r = await robots.text();
  expect(r).not.toContain("<html");
  for (const rule of ["Disallow: /api/", "Disallow: /matches", "Disallow: /account", "Sitemap: https://rolerank.dhruvgoyal.tech/sitemap.xml"]) expect(r).toContain(rule);

  const sitemap = await request.get("/sitemap.xml");
  expect(sitemap.status()).toBe(200);
  expect(sitemap.headers()["content-type"]).toMatch(/xml/);
  const locs = [...(await sitemap.text()).matchAll(/<loc>([^<]+)<\/loc>/g)].map((m) => m[1]);
  expect(locs).toEqual(["/", "/sources", "/about/bot", "/privacy"].map((p) => `https://rolerank.dhruvgoyal.tech${p}`));
});

// Crawlers read real HTML for the public pages (web/scripts/prerender.mjs), never an empty shell.
test("public pages are prerendered with their own title, description, canonical and card", async ({ request }) => {
  const pages: Array<[string, string, string]> = [
    ["/", "/index.html", "Real jobs, ranked against your resume"],
    ["/sources", "/sources/index.html", "Where our jobs come from"],
    ["/about/bot", "/about/bot/index.html", "About RoleRankBot"],
    ["/privacy", "/privacy/index.html", "Privacy"],
  ];
  const titles = new Set<string>();
  for (const [path, file, h1] of pages) {
    const html = await (await request.get(file)).text();
    expect(html).toContain(`data-prerendered="${path}"`);
    expect(html).toContain(`<h1`);
    expect(html).toContain(h1);
    expect(html).toContain(`<link rel="canonical" href="https://rolerank.dhruvgoyal.tech${path}" />`);
    expect(html).toContain('<meta property="og:image" content="https://rolerank.dhruvgoyal.tech/og.png" />');
    expect(html).toContain('<meta name="robots" content="index, follow" />');
    expect(html).not.toContain("JobPosting");
    titles.add(/<title>([^<]+)<\/title>/.exec(html)?.[1] ?? "");
  }
  expect(titles.size).toBe(pages.length);
  const landing = await (await request.get("/index.html")).text();
  expect(landing).toContain('"@type":"Organization"');
  // Every other route gets the noindex shell.
  const shell = await (await request.get("/app.html")).text();
  expect(shell).toContain('<meta name="robots" content="noindex" />');
  expect(shell).not.toContain("data-prerendered");
  expect((await request.get("/og.png")).headers()["content-type"]).toBe("image/png");
});

test("a prerendered page hydrates with no console errors", async ({ page }) => {
  const problems: string[] = [];
  page.on("console", (m) => {
    if (m.type() === "error" || m.type() === "warning") problems.push(m.text());
  });
  page.on("pageerror", (e) => problems.push(e.message));
  await page.goto("/");
  await expect(page.getByRole("link", { name: "Get started" })).toBeVisible();
  await page.getByRole("link", { name: "Get started" }).click();
  await expect(page).toHaveURL(/\/signin$/);
  expect(problems).toEqual([]);
});
