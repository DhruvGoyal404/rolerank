import { renderToString } from "react-dom/server";
import { expect, it } from "vitest";
import { AboutBot } from "./AboutBot";
import { MemoryRouter } from "react-router";
import { startUrl } from "./SignIn";
import { Sources } from "./Sources";

it("publishes the bot contact, and no personal address", () => {
  const html = renderToString(<AboutBot />);
  expect(html).toContain("bot@dhruvgoyal.tech");
  expect(html).not.toContain("gmail");
});

it("offers employers the opt-out, within 7 days", () => {
  const html = renderToString(
    <MemoryRouter>
      <Sources />
    </MemoryRouter>,
  );
  expect(html).toContain("mailto:bot@dhruvgoyal.tech");
  expect(html).toContain("within 7 days");
  expect(html.toLowerCase()).not.toContain("ashby");
});

it("builds sign-in links", () => {
  expect(startUrl("google", "", false)).toBe("/api/auth/google/start");
  expect(startUrl("github", " abc ", true)).toBe("/api/auth/github/start?invite=abc&age18=1");
  expect(startUrl("github", "a&b=c", false)).toBe("/api/auth/github/start?invite=a%26b%3Dc");
});
