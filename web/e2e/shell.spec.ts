import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

const pages = ["/", "/signin", "/signin?error=state", "/about/bot", "/sources", "/no-such-page"];

for (const path of pages) {
  test(`${path} has no accessibility, console or CSP problems`, async ({ page }) => {
    const problems: string[] = [];
    page.on("console", (message) => {
      if (message.type() === "error") problems.push(message.text());
    });
    page.on("pageerror", (error) => problems.push(error.message));

    await page.goto(path);
    await expect(page.locator("h1")).toBeVisible();
    const results = await new AxeBuilder({ page })
      .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"])
      .analyze();

    expect(results.violations).toEqual([]);
    expect(problems).toEqual([]);
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
