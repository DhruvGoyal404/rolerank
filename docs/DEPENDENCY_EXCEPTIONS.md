# Dependency exceptions

Releases younger than 14 days, allowed only because they fix a published security
advisory (CLAUDE.md, "Dependencies"). `scripts/check-deps.mjs` reads the table below.

An entry past its expiry date fails CI in any ecosystem, even if the release is now old
enough, so a stale exception can't linger. A row the script can't read fails too. A
daily scheduled run catches a lapse when nothing was pushed.

| Ecosystem | Package | Version | Published | Advisory | Recorded | Expires |
|---|---|---|---|---|---|---|
| npm | body-parser | 2.4.0 | 2026-10-22 | GHSA-27r3-jxmh-p828 | 2026-10-31 | 2026-11-05 |
| npm | minimatch | 11.0.3 | 2026-10-28 | GHSA-g3hv-75wg-m249 | 2026-10-31 | 2026-11-11 |
| npm | @isaacs/nonrecursive-glob-match | 1.0.3 | 2026-10-28 | GHSA-g3hv-75wg-m249 | 2026-10-31 | 2026-11-11 |
| go-toolchain | go | 1.27.3 | 2026-10-19 | GO-2026-6809 | 2026-10-31 | 2026-11-03T23:59+05:30 |
| go | golang.org/x/net | v0.62.0 | 2026-10-27 | GO-2026-6866, GO-2026-6860 | 2026-10-31 | 2026-11-10 |

minimatch comes in through ESLint, which declares `^10`, so it's forced with an npm
override in `api` and `web`. It's a development dependency only, and
`@isaacs/nonrecursive-glob-match` is required by that fix. ESLint and typescript-eslint
were tested with it, including a negative control. When ESLint declares a fixed range
itself, drop the override.

The Go 1.27.3 row first expired on 2026-11-02. That was off by hours: 1.27.3 was published
at 22:20 UTC on 2026-10-19, so it reaches 14 days at 03:50 IST on 2026-11-03, after that
expiry. The owner moved it to 23:59 IST on 2026-11-03. Remove the row as soon as it has
passed 14 days.

`Ecosystem` is `npm`, `go` (modules) or `go-toolchain`. Adding an ecosystem means adding
a reader to `scripts/check-deps.mjs`.
