# Phase 1: foundation and CI

Run date: 2026-10-31. Everything below is reproducible from `main`.

**Summary. Phase 1 is closed.** The foundation is built, the gate is green on every job on
x86 and arm64, and the owner has signed in with real Google and GitHub accounts.

## What was built

- One Postgres 18 cluster with `app` and `corpus` databases, five roles, and dbmate
  migrations. One SQL template provisions both real and test databases, so the isolation
  tests exercise the real setup.
- Express API: validated config, field-allowlisted logs, Google and GitHub OAuth, hashed
  sessions, invite codes, 18+ confirmation, Origin check, rate limits, account deletion
  with tombstones.
- React shell with sign-in and `/about/bot`, under a strict CSP with Trusted Types.
- Go worker module with the bot User-Agent.
- CI on push to main, with policy checks that have their own failing fixtures.

## Exit criteria

| Criterion | Result |
|---|---|
| TypeScript 7.0.2 tested | typescript-eslint rejects it, so 6.0.3 is pinned (ARCHITECTURE.md) |
| arm64 proven | **Passed.** See below |
| Database isolation proven | **Passed.** 8 tests: each role is refused by the other database, `PUBLIC` has no `CONNECT`, runtime roles can't change schema, `corpus_ro` can't write, no PII column in `corpus` |
| Sign in with both real providers | **Passed** 2026-10-31. See "Real sign-in" below. 33 auth tests also pass against a fake provider that speaks both protocols |
| Negative auth tests | **Passed:** bad and reused `state`, cross-provider `state`, bad `nonce`, wrong `aud` and `iss`, expired token, `alg=none`, HS256 key confusion, unsigned and tampered tokens, foreign `next`, missing and foreign Origin |
| Shortcut refused outside tests | **Passed.** The built server is started with the flag in production and development, and exits 1 |
| e2e refuses a database not ending in `_test` | **Passed** (config test and real-process test) |
| Log allowlist | **Passed.** Planted email, code and tokens don't reach the logs |
| pipefail lint, Actions pinned by SHA | **Passed**, with failing fixtures |
| Dependency-age check | **Passed**, with failing fixtures. A real run also caught a package this phase missed |

## Measured

| Suite | Result |
|---|---|
| API unit | 36 passed |
| API integration (real Postgres and Valkey) | 45 passed, on x86 and on arm64 |
| Web unit | 2 passed |
| Web e2e (Playwright with axe) | 8 passed |
| Policy scripts | 15 passed |
| Dependencies | 284 API and 166 web packages checked; `npm audit` clean |
| CI, run 41190528739 | All 6 jobs green; slowest job 67 s |

## arm64 and gVisor (Q9)

The `arm64` CI job ran on GitHub's `ubuntu-24.04-arm` runner, with gVisor release-20261012.0.

| Check | Under runsc | Under runc (control) |
|---|---|---|
| `docker inspect` `.HostConfig.Runtime` | `runsc` | `runc` |
| `uname -r` | `4.19.0-gvisor` | `7.0.0-1017-azure` |
| `/proc/version` | `Linux version 4.19.0-gvisor ...` | Host kernel |
| UID | 65534 | 65534 |
| Rootfs write | Refused | Refused |
| Network | Blocked | Blocked |

The expected string comes from `defaultRelease` in gVisor's source at the pinned tag.
gVisor's docs warn that the `dmesg` banner is "easily replicated", and it isn't readable
in the container anyway, so `docker inspect` from outside is the primary proof.

Postgres, Valkey and dbmate each pulled as arm64 and ran under runsc. The 45 integration
tests pass on arm64 with those services under the default runtime.

**Decision: Q9 stays on m7g.xlarge.** Limits of this proof: the runner is Azure Arm, not
Graviton. Caddy, the Node base image and the extraction sandbox don't exist yet, so P4 and
P8 repeat the check for them, and the P8 load test runs on the real host.

## Real sign-in

The owner signed in on `localhost` with real OAuth apps, using a 2-use invite. The `app`
database then held:

| Provider | User | Email verified | 18+ confirmed | Invite recorded | Created (IST) |
|---|---|---|---|---|---|
| google | `e018ee3d` | yes | yes | yes | 16:00:51 |
| github | `6cadc343` | no | yes | yes | 16:27:06 |

Also 2 `signup` audit rows, 2 sessions, and 0 invite uses left. They are two separate
accounts, by design: identity is (provider, subject), never email. GitHub shows no verified
email because the `read:user` scope doesn't read one, and P7 needs that for alerts.

The first GitHub attempt left no rows in the database, although it was reported as working.
Its cause wasn't found, because the API's terminal output was gone. The retry worked, so
P1 was closed on the database evidence, not on the report.

## What went wrong

1. **TS 7 proof, first attempt:** `tsc` and Vitest failed on setup mistakes of mine (no
   `rootDir`, and a skipped Vite peer that resolved to a stray copy in the home folder).
   Fixed before drawing any conclusion.
2. **A deletion test failed** because both users had the same fake identity. Test bug. The
   test now also asserts the two IDs differ.
3. **New advisory during the replay.** GHSA-g3hv-75wg-m249 (minimatch) was published after
   the previous clean audit, and the gate refused the commit. Fixed with an override and
   the security exception. npm's suggested fix would have been eslint 4.
4. **Replay mistake.** Replaying whole old trees dropped that override from the web and
   worker commits. Commit `ef8b45f` restored it. Nothing ran on those commits.
5. **Gate bypassed once.** `set -e` has no effect in the agent's shell wrapper, so `ef8b45f`
   was pushed while `check-deps` reported a new transitive package. Commit `e279274`
   recorded it. Gates now stop on an explicit exit status, never on `set -e` alone.
6. **gVisor proof, three tries.** `/sys/class/net` doesn't exist under gVisor, then `dmesg`
   was refused. Both were faults in the check, not the sandbox.

## Next

P2 (ingestion) is under way.
