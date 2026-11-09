# Security review before open signup

Date: 2026-11-09. Scope: the four paths QUESTIONS.md ("Before public signup") names: sign-in,
resume upload, account deletion, and every path that reaches the model. Method: a read of the
code as committed, then the hostile requests run against it (tests named below), plus the
production checks in `rolerank-up.sh`. Reviewer: the project's own process, not an outside
audit; that limit is stated here on purpose.

Each finding is **fixed** (with the commit's test) or **accepted** (with the reason and what
would change the decision). Severity is for this deployment: a personal project, normally down,
with a $1/day global model cap.

## Summary

| ID | Finding | Severity | State |
|---|---|---|---|
| S1 | Open signup lets one client farm accounts against the global model cap | Medium | **Fixed**: 10 new accounts an hour per client address; residual accepted |
| S2 | The extractor runs uploads under the default runtime if its runtime variable is unset | Low | **Fixed** in production: `rolerank-up.sh` fails unless the extractor reports `runtime=runsc` |
| S3 | Text in a data block could close it with a variant tag (`</RESUME_LINES >`) | Low | **Fixed**: every case and spacing of the tag is stripped (`gateway.test.ts`) |
| S4 | No "sign out everywhere" | Low | Accepted |
| S5 | Old resumes (extracted text, encrypted) accumulate per account | Low | Accepted |
| S6 | No zero-data-retention agreement with the model provider | Info | Accepted, disclosed on /privacy (Q17) |
| S7 | An unknown `kid` makes the Google JWKS refetch | Info | Accepted |
| S8 | Contact header and link lines were used as tailoring evidence | Info (correctness) | **Fixed** (`evidence.test.ts`) |

No finding allows reading or changing another user's data, bypassing a cost cap, or getting
text past the contact strip or the hidden-text filter.

## Sign-in (api/src/http/app.ts, api/src/auth/)

Checked and holding:

- Hand-written OAuth for Google and GitHub, PKCE S256 and `state` on both, `nonce` on Google.
  `state` is 32 random bytes, sent in a short-lived httpOnly SameSite=Lax cookie, compared in
  constant time, stored hashed and taken with GETDEL, so a replay finds nothing. The pending
  login records its provider; a state from one provider fails at the other.
- Google ID token: `alg` must be RS256 before any key is touched (no `none`, no HS256 key
  confusion), signature against the JWKS, then `iss`, `aud`, `exp` and `nonce`.
- Users are keyed on (provider, subject). Accounts are never linked by email.
- Sessions: a 32-byte random token in an httpOnly, SameSite=Lax, Secure, `__Host-` cookie; only
  its SHA-256 is stored; 30 days, sliding. Provider tokens are used once and dropped.
- Every non-GET request passes the Origin check (Referer as fallback); post-login `next`
  accepts same-origin paths only (`//host` and backslashes go home).
- Auth routes are rate-limited per address (20 a minute). Logs carry the route pattern only,
  never the query string, codes, tokens or email.

Tests that send the hostile request: `auth.int.test.ts` (bad, missing, reused and cross-provider
state; foreign Origin and Referer; foreign `next`; GitHub's 200-with-error body; expired session;
no email linking; races), `process.int.test.ts` (the test-only login exists only with
`NODE_ENV=test`, proved by starting the real process).

**Open signup (this change).** The invite requirement is removed. A new account still needs the
18+ confirmation. The `invites` table and `users.invite_id` stay until a later release contracts
them (forward-only migrations; the previous release still writes them).

**S1.** With signup open, anyone can create accounts. Each account has its own $0.05/day model
cap, but all share the $1/day global cap, so one client creating many accounts could use up the
day's model budget and turn the model-written parts off for everyone (they degrade to the
deterministic versions, as designed at the cap). Fix: at most `SIGNUPS_PER_HOUR` = 10 new
accounts an hour per client address, checked only for a new account and only after the age
check, so a returning user is never limited (test: "limits new accounts to 10 an hour per
address, never a returning user"). Ten allows for many people behind one mobile-carrier
address. **Residual, accepted:** an attacker with many addresses can still spend the global cap
for a day. The cost is bounded at $1/day by code, and nothing but the optional prose degrades.
Revisit if the ledger shows the global cap being hit by new accounts.

## Resume upload (api/src/http/resumes.ts, worker/internal/extract, worker/cmd/extractor)

Checked and holding:

- 5 MB limit in the API and again in the extractor; magic bytes (`%PDF-`, or a zip for DOCX)
  before anything is stored; 5 uploads an hour and 20 a day per user.
- The file is encrypted with the user's key (AES-256-GCM, additional data naming table, column,
  row and user) before it is written, and deleted once extracted.
- The extractor checks the file again before the sandbox: a PDF that is also a zip is refused,
  and a DOCX's zip directory is read without inflating it, refusing a declared total over 20 MB
  and stopping any part that inflates past it (zip bombs).
- Each parse runs in its own container: no network, read-only root, all capabilities dropped,
  no-new-privileges, user 65534, 16 MB noexec tmpfs, 512 MB memory, 1 CPU, 64 pids, a 20 s
  timeout, under gVisor (`runsc`) in production. The extractor is reachable only on the
  private network, with a shared token compared in constant time.
- Hidden, white and tiny text is shown to the user, flagged, and never sent to the model.

Tests: `resumes.int.test.ts` (wrong type, oversize, foreign ID is 404), `extract_test.go` and
`sandbox_test.go` (zip bomb, polyglot, the sandbox's flags), `isolation.int.test.ts`.

**S2.** The extractor passes `--runtime` only when `ROLERANK_SANDBOX_RUNTIME` is set; unset, a
parse would run under Docker's default runtime with every other limit still in place. Production
sets it to `runsc` in `deploy/host/deploy.sh`, and `rolerank-up.sh` now fails its checks unless the
extractor's start-up log says `runtime=runsc`. Local development runs without gVisor on purpose
(Docker Desktop on Windows has no runsc).

**S5, accepted.** Each upload keeps its extracted lines (encrypted) until the account is deleted.
Uploads are rate-limited and extracted text is small (a few kilobytes), so storage per account
is bounded at about 20 a day. Revisit if disk use grows.

## Account deletion (api/src/auth/users.ts, deploy/backup)

Checked and holding:

- `DELETE /api/me` runs in one transaction: the user row goes, and cascades remove identities,
  sessions (so every device is signed out at once), keys, resumes, profiles, tracker and
  preferences; the user's spend rows go; model-call records keep only cost with the user ID set
  to null; a tombstone records the deletion.
- Without the user's key, any copy of their ciphertext is unreadable.
- Backups: a restore re-applies every tombstone before the site serves (`pg-restore.sh
  --into-live`), and the restore drill on every down proves it. /privacy states the backup window.
- While the site is down, deletion by email to the bot address within 7 days (/privacy).

Tests: `auth.int.test.ts` ("removes everything for that user and leaves a tombstone", "only ever
deletes the caller"), the e2e story that deletes an account through the UI, and the restore
drill in `rolerank-down.sh`.

**S4, accepted.** Signing out ends only the current session; there is no "sign out everywhere".
Deleting the account ends every session. Sessions expire after 30 days unused. Revisit if users
ask for it.

## Paths to the model (api/src/ai/, api/src/aiserver.ts)

Checked and holding:

- One client, in aiworker only. Calls have no tools and no side effects. Output is constrained
  by a JSON schema, then validated with Zod; invalid output is retried once, then the task fails.
- Resume text and job text are untrusted: never in the system prompt, sent as a delimited data
  block, and the system prompt says the block is data. Contact details are stripped before any
  text is sent, and hidden lines never are.
- Grounding: every profile value must cite a resume line that says it; tailoring is computed from
  the resume and the taxonomy with no model at all (model-written reasons are off in v1).
- Cost caps in code: each call reserves its worst case per user and globally, inside one locking
  transaction, before it runs, and settles after; at a cap the call does not happen. Operations'
  check has its own scope and cap.
- Prompts, resume text and output are never logged; `llm_calls` stores cost, tokens, outcome and
  an input hash only.
- aiworker takes a user ID and job facts (titles truncated to 120 characters), never resume text,
  and reads the user's own rows itself.

Tests: `ledger.int.test.ts` (caps under concurrency, unsettled reservations counted), 
`gateway.int.test.ts`, `jobs.int.test.ts` (no body or resume text reaches the ranker or the
model), and the injection sets below.

**S3.** The data-block helper removed only the exact closing tag. A variant such as
`</RESUME_LINES >` or `< /resume_lines>` stayed in the text. The model is told the block is data
and every value must be grounded, so the effect was limited, but the fix is cheap: every case and
spacing of the block's tag, opening or closing, is stripped (`gateway.test.ts`).

**S6, accepted** (owner's decision, Q17): no zero-data-retention agreement with Anthropic for v1.
/privacy says so in plain words, with Anthropic's 30-day and 2-year windows.

**Injection, held out (D7-S).** A fresh set, written and committed before it ran, run once:
results below.

## S7 and S8

**S7, accepted.** A token naming an unknown key ID makes the Google JWKS refetch once. Reaching
that code needs a valid authorization code from Google, so it can't be used to make the API
hammer Google.

**S8, fixed.** Not a security bug, found in the owner's walkthrough: the contact header line
("[PHONE] | [EMAIL] | LinkedIn | GitHub | ...") was shown as evidence for skills ("GitHub" is a
taxonomy alias of Git; "DSA" of Data Structures), and whole sections were joined into one
"bullet". Link and label lines are never evidence now, platform names never count as the skill,
and only true wraps join.

## D7-S results

Pending its single run.
