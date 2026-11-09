// Draws public/og.png (1200 x 630), the social card for the public pages, from the logo and the
// site's own fonts. Run by hand when the card changes: `node scripts/og.mjs`; the PNG is committed.
import { chromium } from "@playwright/test";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const web = join(dirname(fileURLToPath(import.meta.url)), "..");
const font = (f) => `data:font/woff2;base64,${readFileSync(join(web, "public", "fonts", f)).toString("base64")}`;

const html = `<!doctype html><html><head><style>
@font-face { font-family: D; src: url(${font("schibsted-grotesk.woff2")}); font-weight: 400 900; }
@font-face { font-family: T; src: url(${font("atkinson-hyperlegible-next.woff2")}); font-weight: 200 800; }
html, body { margin: 0; }
body { width: 1200px; height: 630px; background: #0d1226; color: #e8ebf6; font-family: T; display: grid;
  grid-template-columns: 1fr 360px; align-items: center; padding: 0 88px; box-sizing: border-box; gap: 40px; }
.brand { display: flex; align-items: center; gap: 14px; font-family: D; font-weight: 800; font-size: 34px; letter-spacing: -0.03em; color: #fff; }
h1 { font-family: D; font-weight: 800; font-size: 72px; line-height: 1.02; letter-spacing: -0.035em; margin: 36px 0 24px; color: #fff; }
p { font-size: 28px; line-height: 1.35; color: #c9cee0; margin: 0; max-width: 32ch; }
.mark { display: flex; align-items: flex-end; gap: 22px; height: 300px; justify-self: end; position: relative; }
.mark i { display: block; width: 72px; border-radius: 10px; background: #5a52de; }
.mark i:nth-child(1) { height: 120px; } .mark i:nth-child(2) { height: 196px; } .mark i:nth-child(3) { height: 260px; background: #b2f12b; }
svg.tick { position: absolute; right: -36px; top: -40px; width: 200px; }
</style></head><body>
<div>
  <div class="brand"><svg width="44" height="44" viewBox="0 0 32 32"><rect width="32" height="32" rx="7" fill="#1b2140"/>
  <rect x="7" y="18" width="4.5" height="8" rx="1" fill="#5a52de"/><rect x="13.5" y="14" width="4.5" height="12" rx="1" fill="#5a52de"/>
  <rect x="20" y="13" width="4.5" height="13" rx="1" fill="#b2f12b"/><path d="M17 11.5l3.6 4.2L27 6" fill="none" stroke="#b2f12b" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round"/></svg>RoleRank</div>
  <h1>Real jobs, ranked against your resume</h1>
  <p>Every match explained. Nothing invented.</p>
</div>
<div class="mark"><i></i><i></i><i></i>
  <svg class="tick" viewBox="0 0 100 100"><path d="M18 52l26 28L92 12" fill="none" stroke="#b2f12b" stroke-width="14" stroke-linecap="round" stroke-linejoin="round"/></svg>
</div>
</body></html>`;

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1200, height: 630 } });
await page.setContent(html, { waitUntil: "load" });
await page.evaluate(() => document.fonts.ready);
await page.screenshot({ path: join(web, "public", "og.png") });
await browser.close();
console.log("wrote public/og.png");
