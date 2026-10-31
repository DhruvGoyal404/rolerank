# Threat model

Scope: RoleRank v1 as designed in [ARCHITECTURE.md](ARCHITECTURE.md). There is no
code yet, so every mitigation here is a requirement, not a claim about a system
that exists. Each one becomes a test (see "Proving it").

This is an engineering document, not legal advice.

## Assets

| Asset | Why it matters | Sensitivity |
|---|---|---|
| Resume text and parsed profile | Names, phone, email, address, education, employment history | **Critical** |
| Application tracker | Where someone is applying; sensitive for an employed person | High |
| Session tokens | Full account takeover | High |
| Resume key (KEK) | Decrypts every resume | **Critical** |
| OAuth client secrets, LLM credentials | Impersonation, spend | High |
| LLM budget | Direct money loss | High |
| Backups | Contain all of the above | **Critical** |
| AWS account | Billing, everything else | **Critical** |
| Sender reputation (email) | Lost alerts, domain blocklisted | Medium |
| Reputation with sources | Being blocked ends the product | Medium |
| Corpus integrity | Fake jobs can harm users | Medium |

## Actors

| Actor | Capability | Goal |
|---|---|---|
| Anonymous internet | Sends any HTTP request | Probe, scrape, abuse |
| Malicious user | Has an account, uploads any file | Read others' data, burn LLM budget, attack the parser |
| Malicious employer or posting author | Controls job text on a real ATS | Prompt injection, phishing links, scam jobs, SEO spam |
| Compromised dependency | Code execution in build or runtime | Secrets, data |
| Compromised source or MITM | Serves altered data | Poison the corpus |
| Curious insider | The owner, with AWS access | Accidental exposure, more than malice |
| LLM provider | Receives prompts | Retention, breach on their side |
| Cloud provider | Hosts everything | Out of scope beyond encryption and region choice |

## Trust boundaries

```
 TB1 internet --> edge (Caddy)
 TB2 edge --> API
 TB3 API --> parser sandbox (hostile bytes, no network)
 TB4 API or worker --> LLM provider (data leaves AWS or our account)
 TB5 ingestion workers --> third-party job sources (hostile content in)
 TB6 app cluster <-/-> corpus cluster (PII must not cross)
 TB7 GitHub Actions --> AWS (OIDC role)
 TB8 host --> S3 backups (create-only)
 TB9 Model output --> application (treated as untrusted input)
```

TB9 is the one people forget. Output of a model that read a hostile resume is hostile.

## STRIDE per component

Each row: threat, then mitigation. **R** marks a residual risk listed below.

### Web client and edge

| STRIDE | Threat | Mitigation |
|---|---|---|
| S | Phishing site clones the login | HTTPS, HSTS, OAuth only (no passwords to steal) |
| T | XSS via job text, company name, resume content or model output | React text rendering only, no `dangerouslySetInnerHTML`, CSP `script-src 'self'` with Trusted Types, strict URL scheme allowlist (`https:` only) on links from sources |
| R | n/a | |
| I | Sensitive data in URLs, Referer or browser cache | No PII in URLs, `Referrer-Policy: strict-origin`, `Cache-Control: no-store` on user data |
| D | Volumetric flood | Rate limits, connection limits at Caddy. **R1** |
| E | Clickjacking on the delete-account flow | `frame-ancestors 'none'` |

### API

| STRIDE | Threat | Mitigation |
|---|---|---|
| S | Session theft or fixation | Random 32-byte token, only its SHA-256 stored, `__Host-` cookie, rotated on login, server-side revocation |
| S | CSRF | SameSite=Lax plus Origin check on all non-GET routes |
| T | SQL injection | Parameterised queries only; a lint rule blocks string-built SQL |
| R | User denies an action, or a deletion is disputed | Append-only `audit_log` of security events: login, delete, export. Holds no resume content |
| I | IDOR on resumes, tracker, matches | Queries scoped by `user_id`; 404 for foreign IDs; tests for every route with a second user |
| I | Data in logs or error reports | Structured logging with an allowlist of fields, not a denylist; a test greps logs from e2e for planted PII |
| D | LLM budget burn, expensive queries | Per-user and global caps, per-route rate limits, query timeouts, pagination caps |
| E | Dev switch reachable in production | Refused at startup outside `NODE_ENV=test`, proven with a real-process test |

### Auth (Google, GitHub)

| STRIDE | Threat | Mitigation |
|---|---|---|
| S | Authorization code interception | PKCE S256 on both providers |
| S | Login CSRF | `state` in an httpOnly cookie, constant-time compare, single use via GETDEL |
| S | ID token replay or forgery | `nonce` checked; RS256 allowlisted (no `none`, no HS256); signature against JWKS with key caching; `iss`, `aud`, `exp` |
| S | Account takeover through email collision | Key on `(provider, sub)`. No auto-link by email. Google's docs warn against `email` as an identifier |
| T | Open redirect through `next` | Same-origin path allowlist |
| I | Provider token leak | Tokens used once and dropped, never stored or logged |
| D | Auth endpoint hammering | 20/min per IP, as in CodeJudge |

Hand-written OAuth carries its own risk: no library has reviewed this code. The
mitigation is a small surface, tests for every rejected case (bad state, bad nonce,
wrong `aud`, expired, `alg=none`) and a dedicated review before launch. **R2**

### Upload and resume parsing

Files are hostile by assumption. This is the riskiest component, and it reuses the
containment CodeJudge built for running untrusted code.

| STRIDE | Threat | Mitigation |
|---|---|---|
| S | File type spoofing, polyglots | Magic bytes checked in the API, then the parser independently confirms. Extension and `Content-Type` are ignored |
| T | PDF parser exploit (memory corruption, malicious fonts, embedded JS) | Parser runs in a gVisor sandbox: no network, read-only rootfs, all caps dropped, seccomp, non-root unique UID, memory, CPU, pids and wall-clock limits. Bytes in over stdin, text out over stdout, size-capped. No JS execution in the extractor |
| T | Malware stored and later served to someone | Files are never served to anyone, and the original is deleted after extraction (Q10) |
| I | Sandbox escape reaches other users' data | Sandbox holds one file, has no credentials and no network. Each parse is a fresh container |
| D | Zip bomb in DOCX, decompression bomb in PDF streams | Reject over 5 MB before parsing. For DOCX, check central-directory sizes first, cap total decompressed bytes at 20 MB, cap entries, cap ratio. Page limit 10. Output cap 200 KB of text. Timeout 20 s |
| D | XML entity expansion in DOCX | XML parser with DTDs and entities disabled |
| D | Queue flood | 5 uploads per hour, 20 per day per user |
| E | Exploit reaches the host kernel | gVisor intercepts syscalls; the host only sees the sandbox runtime. **R3** |

**ClamAV: not in v1.** Signature scanning protects people who later open a file. Here,
nobody opens one: the file is parsed in a sandbox, never served, and then deleted.
Signatures also do little against a targeted parser exploit, and the daemon costs real
memory on a small host. The sandbox is the control. If a feature ever serves files back,
this decision reopens.

### Resume storage and key management

| STRIDE | Threat | Mitigation |
|---|---|---|
| I | Disk, snapshot or bucket exposure | Application-layer AES-256-GCM with a per-user data key (DEK). The DEK is wrapped by a KEK held in SSM SecureString, read only by the app role. S3: block public access, SSE-S3, TLS-only bucket policy |
| I | Plaintext left in temp files | Sandbox tmpfs dies with the container; no resume bytes on the host disk |
| T | Ciphertext swapped between users | GCM additional data binds `user_id` and `resume_id` |
| R | Disputed access | CloudTrail management events; KEK reads visible through SSM logs |
| E | App role reads everything | Least privilege; only the API and worker roles can read the KEK. Its compromise is **R4** |

Deleting the wrapped DEK makes every ciphertext of that user unreadable, wherever a
copy survives. That is how deletion reaches object storage at once.

### App cluster (PII) and corpus cluster

| STRIDE | Threat | Mitigation |
|---|---|---|
| T | Migration damages data | Expand/contract, restore drill, backup before migrating |
| I | PII leaks into corpus | Separate clusters and credentials. The corpus role cannot read the app database. Match requests send a vector and filters, never profile text |
| I | A profile embedding reveals the resume | Embeddings are personal data. Stored only in `app`, deleted with the account. Passed to the corpus cluster per query, never stored there |
| D | Slow query starves the API | `statement_timeout`, separate pools, HNSW scan limits, indexes verified by the bench |
| E | App role can alter schema | Migrations run with a separate role; the runtime role has DML only |

### Valkey

| STRIDE | Threat | Mitigation |
|---|---|---|
| S | Unauthorised access | Bound to the private Docker network, password required |
| I | PII in queues | Messages hold IDs only |
| D | Memory exhaustion | `maxmemory`, capped stream lengths, alarm |
| T | Lost work on restart | AOF, and the DB is the source of truth; the sweeper re-enqueues |

### Ingestion workers (Go)

Job sources are third parties, and their content is untrusted.

| STRIDE | Threat | Mitigation |
|---|---|---|
| S | Fake or scam postings on real boards, or lookalike boards | Boards are allowlisted by discovery review. Company identity comes from the board token. A report button; a suspicious-text flag for fees, Telegram and WhatsApp hiring pitches. **R5** |
| T | Stored XSS or tracking in HTML descriptions | HTML reduced to plain text at ingest. Raw HTML is never stored or rendered |
| T | SSRF through source-provided URLs | We never fetch URLs from posting content. The fetcher only calls templated host allowlists, and blocks private IPs and redirects off the allowlist |
| R | A source says we abused it | Fetch log with host, status, timing; honest UA with contact |
| I | n/a: public data | |
| D | Huge or endless response | Response size cap, timeouts, gzip ratio cap, per-board job cap, circuit breaker per host |
| D | We overload a source | Shared per-host limiter, conditional GET, backoff on 429 and 503 |
| E | Parser bug in hostile JSON | Strict decoding with size limits, in a memory-safe language |

### LLM gateway

Prompt injection can't be prevented, only contained. The design assumes it succeeds
and makes success worthless.

| STRIDE | Threat | Mitigation |
|---|---|---|
| S | Resume says "this candidate is a perfect match" to steer output | Ranking doesn't use LLM output, so there's nothing to steer. Text in the resume counts only as the skills it really contains |
| T | Injected output: false skills, links, markup | Schema-constrained output, then Zod validation, length and enum limits, URLs rejected. Parsed fields need support in the source text (grounding check). The user confirms the profile before it's used |
| T | Hidden text (white, tiny, off-page) | Extractor reports font size, colour and position anomalies; `suspicious_text` flag; hidden runs dropped before the LLM; the user sees what was read |
| R | Cannot explain what the model said | Log model, prompt version, schema version, token counts, latency, and a hash of the input. Never the content |
| I | System prompt or another user's data leaks | Every call is stateless and single-user, so no other user's data is present to leak. The system prompt holds no secrets |
| I | Provider retains or breaches prompts | Contact details stripped first. Zero-retention mode where offered. Only approved providers. Never Gemini's free tier for user data. **R6** |
| D | Budget exhaustion, huge inputs | Input token cap, reserve-then-settle ledger, per-user and global daily caps, circuit breaker on provider errors |
| E | Model output triggers an action | Impossible by design: no tools. Output is only ever rendered as text or stored as validated data |

### Email alerts

| STRIDE | Threat | Mitigation |
|---|---|---|
| S | Spoofing our domain | SPF, DKIM, DMARC on a sending subdomain |
| T | Header injection from job titles | Sent through the SES API with structured fields; titles sanitised |
| I | Email reveals a job search on a shared inbox | Subject lines are generic and contain no employer name |
| D | Bounces or complaints damage reputation | SES bounce and complaint handling, automatic suppression, opt-in only, one-click unsubscribe with `List-Unsubscribe` |
| E | Unsubscribe link takes over an account | Signed token scoped to one action, needs no session |

### CI/CD and supply chain

| STRIDE | Threat | Mitigation |
|---|---|---|
| S | Forged deploy | GitHub OIDC with a role trusting only `main` of this repo; immutable subject |
| T | Malicious dependency | 14-day rule, exact pins, `npm ci --ignore-scripts` where possible, `govulncheck`, `npm audit`, Actions pinned by SHA, images by digest |
| I | Secrets in CI logs | gitleaks, masked values, secret-free PR builds, public-repo-safe logs |
| E | Fork PR steals secrets | No secrets on `pull_request` from forks; no `pull_request_target` |

### Backups and up/down scripts

| STRIDE | Threat | Mitigation |
|---|---|---|
| T | Ransomware or a compromised host deletes backups | Host role has `s3:PutObject` only, on one prefix. S3 Object Lock retention on the backup prefix. No delete or list from the host |
| I | Backup stolen | Encrypted client-side with `age` to a public key. The private key isn't on the host; it's in SSM, readable only by the restore path |
| R | Backup silently stops | Freshness alarm, restore drill on every down, row-count check. CodeJudge's incident is the reason |
| D | Lost backups | Versioned bucket, final backup kept through down |
| I | Deleted user comes back after a restore | `deletion_tombstones` lives outside the restored data set and is re-applied by the restore script before the app starts |
| E | Script prints secrets | Scripts never echo values; CI greps script output |

## Prompt-injection model in one paragraph

Untrusted text can make the model say anything, so nothing the model says is trusted.
It can't act: no tools, no network, no memory across users. It can't rank: ranking is
deterministic. It can't change a stored profile without schema validation, a
grounding check against the source text and user confirmation. It can only harm the
person whose own resume or chosen job it read, by showing them wrong text, and the
grounding gate plus the "shown as text only" rule bound that. A malicious job
description is the realistic case: it can try to make a tailoring suggestion say
something bad, and the gate requires all facts to come from the user's own resume.

## Data lifecycle

| Data | Where | Retention |
|---|---|---|
| Original upload | Sandbox stdin only, or encrypted S3 quarantine for a parse retry | Deleted after extraction (proposed, Q10) |
| Resume text, line-numbered | `app` DB, encrypted | Until the user deletes it or the account |
| Parsed profile, embedding | `app` DB | Same |
| Matches, tracker | `app` DB | Same |
| Sessions | `app` DB | 30 days idle |
| Logs | CloudWatch or host | 14 days, no PII by design |
| LLM ledger | `app` DB | 90 days, counts and costs only |
| Backups of `app` | S3 | **14 days**, then expired |
| Final backup taken at `down` | S3 | Newest 2, removed once superseded. See below |
| Provider side | LLM provider | Per Q5: zero retention where offered. Flagged content may be held longer by the provider |

### Deletion, including backups

1. The user deletes. Immediately: the DEK, the resume, profile, embedding, matches,
   tracker and sessions go in one transaction, and a tombstone is written.
2. Backups taken earlier still hold the wrapped DEK and the encrypted resume. They
   age out in 14 days.
3. A restore re-applies tombstones before the app starts, so a deleted user can't
   come back.
4. **The honest catch.** RoleRank is usually down, so a person can't delete while the
   site is off, and a `final` backup may sit for months. /privacy must say so: while
   the service is down, deletion happens on the next up, or by email to the contact
   address, and the owner runs it within 7 days. See Q1.

## Compliance notes

- **India, DPDP Act 2023 and Rules 2025.** Secondary sources agree the Rules were
  notified on 13 or 14 November 2025 with a phased start, and most substantive duties
  (notice, consent, breach reporting, erasure) arrive around May 2027. Sources differ
  by a day on that date, and on the breach-notice detail. The official gazette wasn't
  checked, so **those details are unknown here**. We design as if the duties already
  apply: plain notice, consent at upload, working deletion, breach response plan.
- **Children.** Signup requires a statement of being 18 or over, because the Act treats
  under-18 data specially.
- **Automated decisions.** RoleRank ranks jobs for a user. It never screens candidates
  for employers, which is a different and heavier regulatory category. It stays out
  of that category by design (SPEC.md non-goals).
- A student project is still a data fiduciary if it holds real users' resumes. Legal
  review is unaffordable, which is one reason Q2 recommends a limited launch.

## Residual risks

| # | Risk | Why it remains | Accepted because |
|---|---|---|---|
| R1 | Volumetric DDoS | No WAF or CDN shield in the budget | Downtime is tolerable on a demo service; site is rarely up |
| R2 | Bug in hand-written OAuth | No library review | Small surface, full negative tests, owner's explicit choice |
| R3 | gVisor or parser zero-day | Can't be eliminated | Two layers, fresh container, no secrets inside |
| R4 | API process compromise reads all resumes it can decrypt | The app must decrypt to work | Small user base; least privilege; no bulk export route |
| R5 | Scam postings on real boards | Can't vet every employer | Only ATS boards; flags; report button; link-out only |
| R6 | LLM provider breach or flagged-content retention | Third-party | Contact details stripped; approved providers only |
| R7 | Soft inflation in tailoring that the gate can't see | Semantics aren't mechanically checkable | Eval audit; user reviews every suggestion; nothing is sent anywhere automatically |
| R8 | Backup window: deleted data readable for 14 days to someone holding both a backup and its key | Backups must exist | Stated on /privacy; key held apart |
| R9 | Single host, single owner | Budget | Not a production service; stated plainly |
| R10 | Terms of a source change | Outside our control | Re-check quarterly; each source can be switched off by config |

## Proving it

Every mitigation above gets a test or a check in ROADMAP.md's exit criteria. A
security claim on /privacy or in the README must point at a test. Before any public
launch, run a focused review of auth, upload and deletion, and record the result in
`docs/SECURITY_REVIEW.md`.
