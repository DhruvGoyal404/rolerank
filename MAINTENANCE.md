# Maintenance

The production runbook, and the record of anything that broke production or a backup
(CLAUDE.md). RoleRank is normally **down**; it runs only when needed.

**Status (2026-11-09):** the code exists (P8 prep) and nothing has been deployed. `cdk synth`,
the infra tests and shellcheck pass; no step below has run against AWS yet. P8 proper runs the
first real cycle and fills in the measured numbers.

## Up and down

From Git Bash at the repository root:

```
./rolerank-up.sh      # fully down -> live; prints the box with the URL and the cost per hour
./rolerank-down.sh    # live -> nothing billing; ends with the audit
```

- **up** deploys both stacks, waits for the host's bootstrap, restores Caddy's certificates,
  redeploys the release saved at the last down by digest (or builds main with `--build`),
  restores the newest backup set and re-applies deletion tombstones, checks the row counts
  against those recorded at down, and only then points DNS at the host. It warms the ranker
  with two searches for a synthetic profile, then checks health by IP and by domain, both OAuth
  providers, one real LLM call (through the spend ledger under the `ops` scope, capped at
  $0.01 a day: `api/src/cli/llm-check.ts`) and the warm search's latency. There is no email
  check in v1.
- **down** stops the writers, takes a backup, restores it into scratch databases and compares
  every app table's row count; any failure stops it with nothing removed. Then it keeps the
  set under `rolerank/final/`, saves the release manifest, parks DNS at 192.0.2.1, destroys
  the host stack and audits. `STILL BILLING` and exit 1 mean something of RoleRank's still costs
  money.
- Both are safe to run again after a half-failed run.

## First time only

1. **AWS login.** The account is CodeJudge's (QUESTIONS.md, Q15); the scripts use the
   `codejudge` SSO profile unless `AWS_PROFILE` says otherwise.
2. **Secrets.** `bash scripts/prod-secrets.sh` generates the passwords, tokens, the resume
   key-encryption key and the backup key pair straight into SSM, and asks (without echo) for
   the model API key, the production OAuth clients and the Cloudflare token. Nothing is shown.
   The OAuth callbacks are `https://rolerank.dhruvgoyal.tech/api/auth/{google,github}/callback`.
3. **The host's AMI.** Pin Ubuntu 24.04 arm64 in `infra/cdk.json` `"rolerank:hostAmi"`:
   `aws ssm get-parameter --name /aws/service/canonical/ubuntu/server/24.04/stable/current/arm64/hvm/ebs-gp3/ami-id --query Parameter.Value --output text`.
   A literal ID keeps the host from being replaced on every Ubuntu release; change it only on
   purpose, while down.
4. **GitHub secrets** for the Deploy workflow, from the RoleRankProd outputs:
   `AWS_DEPLOY_ROLE_ARN` (DeployRoleArn) and `ECR_REGISTRY` (Registry).
5. **Budget tag.** Activate the cost-allocation tag `rolerank:project` in Billing once it
   appears (a day after the first tagged resource). Until then the budget counts nothing, and
   up and down print a warning.
6. **Corpus.** After the persistent stack exists, `bash scripts/seed-corpus.sh` dumps the
   local corpus (postings only, no user data), encrypts it with the backup key and uploads it
   to `rolerank/seed/`. The first `./rolerank-up.sh --first` restores the newest seed into the
   corpus, rebuilds its indexes and analyzes it; the app database starts empty. Later ups restore
   the backups taken at down instead. Measured locally on 2026-11-09 (6-CPU Docker VM): the
   corpus of 193,711 postings (21,279 searchable, 1.1 GB on disk) dumps to 259 MB in 47 s, and
   restores with all 27 indexes in 59 s plus 8 s of ANALYZE.

## Cost

On-demand, ap-south-1, USD (ARCHITECTURE.md, "Cost": AWS price files, 2026-10-31; Rs at 94.95).
P8 replaces these with Pricing API numbers and the first real bill.

| State | Cost | Detail |
|---|---|---|
| Up, per hour | ~$0.127 (~Rs 12) | m7g.xlarge $0.1166 + 40 GB gp3 $0.0050 + public IPv4 $0.005; LLM use capped at $1/day |
| Up for a month (down forgotten) | ~$93 (~Rs 8,800) | the budget's 50% email arrives within about a day |
| Down, per month | ~$0.30 (~Rs 28), to confirm | ECR ~1.5 GB, S3 backup sets, SSM, SNS, one alarm, the budget |

**Budget:** `rolerank-monthly`, $5.27 (Rs 500), emails at 50, 80 and 100% of actual spend and
at 100% forecast. It is an alert, not a cap: AWS keeps billing past it.

## Backups

- Nightly at 21:30 UTC while up (`rolerank-backup.timer`), plus the final one at every down.
- Both databases, `pg_dump` custom format, encrypted with `age` to the public key in SSM; the
  private key is an SSM SecureString only the host role and the owner can read.
- Object Lock (governance, 14 days): nothing deletes a backup within retention, the host
  included. The host role can only put and get under `rolerank/pg/`.
- The `BackupMissing` alarm emails after 26 hours without a success while up; down disables
  its actions and up re-enables them after a fresh backup.

## Incidents

None yet.
