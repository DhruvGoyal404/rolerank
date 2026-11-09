import { renderToString } from "react-dom/server";
import { expect, it } from "vitest";
import { ago, Facts, STALE_DAYS } from "./components";

const now = Date.parse("2026-11-09T12:00:00Z");

it("says how long ago the corpus was refreshed", () => {
  expect(ago("2026-11-09T11:59:30Z", now)).toBe("just now");
  expect(ago("2026-11-09T11:15:00Z", now)).toBe("45 minutes ago");
  expect(ago("2026-11-09T06:00:00Z", now)).toBe("6 hours ago");
  expect(ago("2026-11-08T10:00:00Z", now)).toBe("yesterday");
  expect(ago("2026-11-02T12:00:00Z", now)).toBe("7 days ago");
});

it(`marks a posting older than ${String(STALE_DAYS)} days`, () => {
  const job = { company: "Acme", location: "Pune", closed: false } as const;
  const old = renderToString(<Facts job={{ ...job, postedAt: new Date(Date.now() - 45 * 86_400_000).toISOString() } as never} />);
  expect(old).toContain("may already be filled");
  const fresh = renderToString(<Facts job={{ ...job, postedAt: new Date(Date.now() - 3 * 86_400_000).toISOString() } as never} />);
  expect(fresh).not.toContain("may already be filled");
});
