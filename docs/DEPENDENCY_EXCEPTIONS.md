# Dependency exceptions

Releases younger than 14 days, allowed only because they fix a published security
advisory (CLAUDE.md, "Dependencies"). Remove an entry once it passes the rule.
`scripts/check-deps.mjs` reads the table below.

| Package | Version | Published | Advisory | Recorded | Expires |
|---|---|---|---|---|---|
| body-parser | 2.4.0 | 2026-10-22 | GHSA-27r3-jxmh-p828 | 2026-10-31 | 2026-11-05 |
