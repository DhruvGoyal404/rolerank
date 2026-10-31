# Dependency exceptions

Releases younger than 14 days, allowed only because they fix a published security
advisory (CLAUDE.md, "Dependencies"). Remove an entry once it passes the rule.
`scripts/check-deps.mjs` reads the table below.

| Package | Version | Published | Advisory | Recorded | Expires |
|---|---|---|---|---|---|
| body-parser | 2.4.0 | 2026-10-22 | GHSA-27r3-jxmh-p828 | 2026-10-31 | 2026-11-05 |
| minimatch | 11.0.3 | 2026-10-28 | GHSA-g3hv-75wg-m249 | 2026-10-31 | 2026-11-11 |

minimatch comes in through ESLint, which declares `^10`, so it's forced with an npm
override in `api` and `web`. It's a development dependency only. ESLint and
typescript-eslint were tested with it, including a negative control. When ESLint declares
a fixed range itself, drop the override.
