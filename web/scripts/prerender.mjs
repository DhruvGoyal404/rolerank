// Prerenders RoleRank's public pages into dist/ after `vite build` (DESIGN.md, SEO). Each page gets
// its HTML, a unique title and description, a canonical URL, Open Graph and Twitter cards, and is
// indexable. dist/app.html is the shell every other route falls back to: noindex, no structured
// data (job pages emit no JobPosting, CLAUDE.md). Run by `npm run build`.
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const web = join(dirname(fileURLToPath(import.meta.url)), "..");
const dist = join(web, "dist");
const ORIGIN = "https://rolerank.dhruvgoyal.tech";
const { render } = await import(pathToFileURL(join(web, "dist-ssr", "entry-server.js")).href);

export const PAGES = [
  {
    path: "/",
    title: "RoleRank: real jobs, ranked against your resume",
    description:
      "RoleRank ranks real software and engineering jobs in India and remote roles open to India against your resume, and explains every match. Free for freshers and interns.",
  },
  {
    path: "/sources",
    title: "Where our jobs come from | RoleRank",
    description: "The public, legal job sources RoleRank reads, what it shows from each, and how employers can remove their jobs within 7 days.",
  },
  {
    path: "/about/bot",
    title: "About RoleRankBot | RoleRank",
    description: "How RoleRankBot reads public job listings: it identifies itself, obeys robots.txt, limits its rate and never trains models on postings.",
  },
  {
    path: "/privacy",
    title: "Privacy | RoleRank",
    description: "What RoleRank keeps, how your resume is read and protected, what reaches an AI model, and how deleting your account works.",
  },
];

const escape = (s) => s.replaceAll("&", "&amp;").replaceAll('"', "&quot;").replaceAll("<", "&lt;").replaceAll(">", "&gt;");

const STRUCTURED = JSON.stringify([
  {
    "@context": "https://schema.org",
    "@type": "Organization",
    name: "RoleRank",
    url: `${ORIGIN}/`,
    logo: `${ORIGIN}/icons/icon-512.png`,
    email: "bot@dhruvgoyal.tech",
  },
  {
    "@context": "https://schema.org",
    "@type": "WebSite",
    name: "RoleRank",
    url: `${ORIGIN}/`,
  },
]).replaceAll("<", "\\u003c");

function head(page) {
  const url = `${ORIGIN}${page.path}`;
  const t = escape(page.title);
  const d = escape(page.description);
  return [
    `<title>${t}</title>`,
    `<meta name="description" content="${d}" />`,
    `<meta name="robots" content="index, follow" />`,
    `<link rel="canonical" href="${url}" />`,
    `<meta property="og:type" content="website" />`,
    `<meta property="og:site_name" content="RoleRank" />`,
    `<meta property="og:title" content="${t}" />`,
    `<meta property="og:description" content="${d}" />`,
    `<meta property="og:url" content="${url}" />`,
    `<meta property="og:image" content="${ORIGIN}/og.png" />`,
    `<meta property="og:image:width" content="1200" />`,
    `<meta property="og:image:height" content="630" />`,
    `<meta property="og:image:alt" content="RoleRank: real jobs, ranked against your resume, with every match explained" />`,
    `<meta name="twitter:card" content="summary_large_image" />`,
    `<meta name="twitter:title" content="${t}" />`,
    `<meta name="twitter:description" content="${d}" />`,
    `<meta name="twitter:image" content="${ORIGIN}/og.png" />`,
    page.path === "/" ? `<script type="application/ld+json">${STRUCTURED}</script>` : "",
  ]
    .filter(Boolean)
    .join("\n    ");
}

const template = readFileSync(join(dist, "index.html"), "utf8");
const marker = /<title>RoleRank<\/title>\s*<meta name="robots" content="noindex" \/>\s*<meta name="description" content="[^"]*" \/>/;
if (!marker.test(template)) throw new Error("index.html lost its <title> and robots markers: update scripts/prerender.mjs");

// The shell for every other route: noindex, no page content.
writeFileSync(join(dist, "app.html"), template);

for (const page of PAGES) {
  const body = render(page.path);
  if (!body.includes("<h1")) throw new Error(`${page.path} rendered no h1`);
  const html = template
    .replace(marker, head(page))
    .replace('<div id="root"></div>', `<div id="root" data-prerendered="${page.path}">${body}</div>`);
  const out = page.path === "/" ? join(dist, "index.html") : join(dist, page.path.slice(1), "index.html");
  mkdirSync(dirname(out), { recursive: true });
  writeFileSync(out, html);
  console.log(`prerendered ${page.path} (${String(Math.round(html.length / 1024))} KB)`);
}
