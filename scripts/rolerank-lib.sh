#!/usr/bin/env bash
# Shared by rolerank-up.sh and rolerank-down.sh (sourced, not run). Bash for Git Bash on
# Windows (and Linux). Adapted from CodeJudge's scripts/codejudge-lib.sh, which ran a real cycle.
# shellcheck disable=SC2034 # constants used by the scripts that source this

export MSYS_NO_PATHCONV=1 # "/rolerank/..." SSM names must reach aws.exe unchanged
# The same AWS account as CodeJudge (QUESTIONS.md, Q15), so the same SSO profile by default.
export AWS_PROFILE=${AWS_PROFILE:-codejudge}
export AWS_REGION=ap-south-1 AWS_DEFAULT_REGION=ap-south-1 AWS_PAGER=""
# aws.exe prints in the Windows code page and dies on characters such as "->" in host output.
export PYTHONUTF8=1

# pwd -W: the Windows form (E:/...) that native git and aws.exe accept with MSYS_NO_PATHCONV.
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && { pwd -W 2>/dev/null || pwd; })
PERSISTENT=RoleRankProd
HOST_STACK=RoleRankHost
HOST_TAG_KEY=rolerank:host
HOST_TAG_VALUE=prod
PROJECT_TAG_KEY=rolerank:project # every RoleRank resource (infra/lib/stacks.ts tagProject)
PROJECT_TAG_VALUE=rolerank
PARKED_IP=192.0.2.1 # TEST-NET-1: never routable, so the name never follows a released IP
CF_TOKEN_PARAM=/rolerank/ops/cloudflare-token
REPO=DhruvGoyal404/rolerank
COMPOSE_PROJECT=rolerank-prod # deploy/compose.prod.yaml "name:"
NIGHTLY_PREFIX=rolerank/pg/   # the only prefix the host may read and write
FINAL_PREFIX=rolerank/final/  # rolerank-down.sh's verified backups; never expire
SEED_PREFIX=rolerank/seed/    # scripts/seed-corpus.sh: the corpus for the first up
RELEASE_KEY=rolerank/final/release.json
KEEP_FINAL=2
ECR_MAX_BYTES=$((3 * 1073741824)) # 2 releases of 4 images are ~1.5 GB; more means a pile-up
S3_MAX_BYTES=$((6 * 1073741824))  # 2 final sets of ~1.3 GB, plus locked old versions for 15 days
# On-demand, ap-south-1 (ARCHITECTURE.md, "Cost": AWS price files, 2026-10-31).
PRICE_HOUR_USD=0.127 # m7g.xlarge 0.1166 + gp3 40 GB 0.0050 + public IPv4 0.005
USD_INR=94.95
HOST_COMPOSE="docker compose -f /opt/rolerank/current/app/deploy/compose.prod.yaml --env-file /etc/rolerank/prod.env --env-file /opt/rolerank/current/release.env"

# Windows form too (C:/...): native jq, curl and aws.exe must open these files.
WORK=$(cd "$(mktemp -d)" && { pwd -W 2>/dev/null || pwd; })
trap 'rm -rf "$WORK"' EXIT
T0=$(date +%s)

log() { printf '\n\033[1m[%s +%ss] %s\033[0m\n' "$(date +%H:%M:%S)" "$(($(date +%s) - T0))" "$*" >&2; }
die() { printf '\n\033[31mERROR: %s\033[0m\n' "$*" >&2; exit 1; }
winpath() { if command -v cygpath >/dev/null; then cygpath -w "$1"; else echo "$1"; fi; }

# retry CMD...: runs CMD up to 6 times while the error looks transient (throttling, 5xx,
# "in progress", connection trouble); prints its output on success only.
retry() {
  local n=0 out
  while :; do
    if out=$("$@" 2>&1); then
      [ -z "$out" ] || printf '%s\n' "$out"
      return 0
    fi
    n=$((n + 1))
    if [ "$n" -ge 6 ] || ! grep -qiE 'thrott|toomanyrequests|requestlimitexceeded|internalerror|internal error|serviceunavailable|unavailable|\b50[0234]\b|in ?progress|timed? ?out|could not connect|connection (reset|refused|was closed)|incorrectinstancestate|invalidinstanceid|pending' <<<"$out"; then
      printf '%s\n' "$out" >&2
      return 1
    fi
    sleep $((n * 5))
  done
}
aws_() { retry aws "$@"; }
param() { aws_ ssm get-parameter --name "$1" --with-decryption --query Parameter.Value --output text; }

# --- preflight ------------------------------------------------------------------
preflight() {
  local c plan tag
  for c in aws jq npm gh git curl openssl; do command -v "$c" >/dev/null || die "$c is not installed"; done
  if ! aws sts get-caller-identity >/dev/null 2>&1; then
    log "AWS session expired: approve the login in the browser"
    aws sso login --profile "$AWS_PROFILE" || die "aws sso login failed"
  fi
  ACCOUNT=$(aws_ sts get-caller-identity --query Account --output text)
  # The right account is the one that holds the project's SSM parameters (prod-secrets.sh).
  aws_ ssm get-parameter --name /rolerank/prod/ROLERANK_DOMAIN --query Parameter.Name --output text >/dev/null ||
    die "account ${ACCOUNT:0:4}******** has no /rolerank/prod parameters: wrong AWS_PROFILE, or run scripts/prod-secrets.sh first"
  # Billing state from the API, not a cache: the account must be active.
  plan=$(aws_ freetier get-account-plan-state --query '[accountPlanType,accountPlanStatus]' --output text) ||
    die "could not read the account's plan state"
  [[ "$plan" == *ACTIVE* ]] || die "account plan is not active: $plan"
  gh auth status --hostname github.com >/dev/null 2>&1 || die "gh is not logged in: gh auth login"
  DOMAIN=$(param /rolerank/prod/ROLERANK_DOMAIN)
  ALARM_EMAIL=$(param /rolerank/ops/alarm_email)
  # The budget counts only costs tagged rolerank:project, so the tag must be active in Billing.
  tag=$(aws ce list-cost-allocation-tags --tag-keys "$PROJECT_TAG_KEY" --query 'CostAllocationTags[0].Status' --output text 2>/dev/null || echo unknown)
  [ "$tag" = Active ] ||
    printf '\033[33mwarning: cost-allocation tag %s is %s: the budget alert counts nothing until it is activated in Billing (MAINTENANCE.md)\033[0m\n' "$PROJECT_TAG_KEY" "$tag" >&2
  echo "account ${ACCOUNT:0:4}********, plan: $plan, region $AWS_REGION, site $DOMAIN" >&2
}

# --- CDK ---------------------------------------------------------------------------
cdk_() {
  (
    cd "$ROOT/infra" || exit 1
    [ -d node_modules ] || npm ci --no-audit --no-fund >&2
    CDK_DEFAULT_ACCOUNT=$ACCOUNT npx cdk "$@" -c alarmEmail="$ALARM_EMAIL" --require-approval never --progress events
  )
}
stack_exists() {
  local s
  s=$(aws cloudformation describe-stacks --stack-name "$1" --query 'Stacks[0].StackStatus' --output text 2>/dev/null) || return 1
  [ "$s" != DELETE_COMPLETE ]
}
stack_output() { aws_ cloudformation describe-stacks --stack-name "$1" --query "Stacks[0].Outputs[?OutputKey=='$2'].OutputValue" --output text; }
bucket() { stack_output "$PERSISTENT" BackupBucket; }

# --- the host ------------------------------------------------------------------------
host_id() {
  local id
  id=$(aws_ ec2 describe-instances --filters "Name=tag:$HOST_TAG_KEY,Values=$HOST_TAG_VALUE" Name=instance-state-name,Values=pending,running \
    --query 'Reservations[].Instances[].InstanceId' --output text)
  [ "$id" = None ] && id=""
  echo "$id"
}
# ssm_send ID DOCUMENT PARAMS_FILE: runs a command document on the host, prints its output,
# fails if it fails.
ssm_send() {
  local id=$1 doc=$2 f=$3 cid st out err
  cid=$(aws_ ssm send-command --instance-ids "$id" --document-name "$doc" --timeout-seconds 120 \
    --parameters "file://$(winpath "$f")" --query Command.CommandId --output text) || return 1
  while :; do
    sleep 5
    st=$(aws ssm get-command-invocation --command-id "$cid" --instance-id "$id" --query Status --output text 2>/dev/null || echo Pending)
    case "$st" in Pending | InProgress | Delayed) ;; *) break ;; esac
  done
  out=$(aws_ ssm get-command-invocation --command-id "$cid" --instance-id "$id" --query StandardOutputContent --output text)
  err=$(aws_ ssm get-command-invocation --command-id "$cid" --instance-id "$id" --query StandardErrorContent --output text)
  [ -z "$out" ] || printf '%s\n' "$out"
  [ -z "$err" ] || printf '%s\n' "$err" >&2
  [ "$st" = Success ] || { echo "host command $st" >&2; return 1; }
}
# ssm_run ID SCRIPT [TIMEOUT_S]: runs SCRIPT with bash as root on the host.
ssm_run() {
  local f="$WORK/ssm-params.json"
  jq -n --arg s "$2" --arg t "${3:-1800}" \
    '{commands: ["cat > /root/rolerank-op.sh <<'"'"'ROLERANK_OP'"'"'\n" + $s + "\nROLERANK_OP", "exec bash /root/rolerank-op.sh"], executionTimeout: [$t]}' >"$f"
  ssm_send "$1" AWS-RunShellScript "$f"
}
# result KEY OUTPUT: the value of a "RESULT KEY=value" line the host printed.
result() { sed -n "s/^RESULT $1=//p" <<<"$2" | tail -n 1 | tr -d '\r'; }

# --- Cloudflare DNS (the zone is on Cloudflare: nslookup -type=NS dhruvgoyal.tech) -------------
cf_api() { # cf_api METHOD PATH [JSON]; the token goes in on stdin, never in argv
  local tok
  tok=$(param "$CF_TOKEN_PARAM") || die "Cloudflare token missing: store it in SSM $CF_TOKEN_PARAM (scripts/prod-secrets.sh)"
  if [ -n "${3:-}" ]; then
    printf 'Authorization: Bearer %s\n' "$tok" | retry curl -sS --fail-with-body -X "$1" -H @- -H 'Content-Type: application/json' --data "$3" "https://api.cloudflare.com/client/v4$2"
  else
    printf 'Authorization: Bearer %s\n' "$tok" | retry curl -sS --fail-with-body -X "$1" -H @- "https://api.cloudflare.com/client/v4$2"
  fi
}
cf_check() {
  cf_api GET /user/tokens/verify | jq -e '.result.status == "active"' >/dev/null || die "the Cloudflare token is not active"
}
# cf_set_a IP: points DOMAIN's A record at IP (DNS only, TTL 60).
cf_set_a() {
  local ip=$1 zone rec cur body
  zone=$(cf_api GET "/zones?name=${DOMAIN#*.}" | jq -r '.result[0].id // empty')
  [ -n "$zone" ] || die "Cloudflare zone ${DOMAIN#*.} not found (the token needs Zone:Read and DNS:Edit on it)"
  rec=$(cf_api GET "/zones/$zone/dns_records?type=A&name=$DOMAIN")
  body=$(jq -nc --arg ip "$ip" --arg n "$DOMAIN" '{type: "A", name: $n, content: $ip, ttl: 60, proxied: false}')
  cur=$(jq -r '.result[0].content // empty' <<<"$rec")
  if [ "$cur" = "$ip" ] && [ "$(jq -r '.result[0].proxied' <<<"$rec")" = false ]; then
    echo "DNS: $DOMAIN A $ip already" >&2
  elif [ -n "$cur" ]; then
    cf_api PATCH "/zones/$zone/dns_records/$(jq -r '.result[0].id' <<<"$rec")" "$body" | jq -e .success >/dev/null || die "DNS update failed"
    echo "DNS: $DOMAIN A $cur -> $ip (DNS only, TTL 60)" >&2
  else
    cf_api POST "/zones/$zone/dns_records" "$body" | jq -e .success >/dev/null || die "DNS create failed"
    echo "DNS: $DOMAIN A $ip created (DNS only, TTL 60)" >&2
  fi
}

backup_alarm() { aws_ cloudwatch describe-alarms --alarm-name-prefix "$PERSISTENT-BackupMissing" --query 'MetricAlarms[0].AlarmName' --output text; }

# newest_stamp PREFIX: the newest backup set's stamp under PREFIX (names carry a UTC stamp).
newest_stamp() {
  aws_ s3api list-objects-v2 --bucket "$BUCKET" --prefix "$1" --query 'Contents[].Key' --output text | tr '\t' '\n' | tr -d '\r' |
    sed -n 's/^.*rolerank-\([0-9TZ]*\)-app\.dump\.age$/\1/p' | sort | tail -n 1
}

# --- audit -------------------------------------------------------------------------------
# audit: everything of RoleRank's that can still bill, with its idle cost. Resources of other
# projects in the account (CodeJudge) are listed for information and never fail it. Exits
# non-zero with "STILL BILLING" if anything of ours bills beyond the documented allowance.
audit() {
  local bad=0 n v line region tagf
  log "Audit: what of RoleRank's can still bill"
  report() { if [ "$1" = yes ]; then printf '  \033[31m%-6s\033[0m %s\n' BILLS "$2"; bad=1; else printf '  %-6s %s\n' ok "$2"; fi; }
  tagf="Name=tag:$PROJECT_TAG_KEY,Values=$PROJECT_TAG_VALUE"
  n=$(aws_ ec2 describe-instances --filters "$tagf" Name=instance-state-name,Values=pending,running,stopping,stopped --query 'length(Reservations[].Instances[])' --output text)
  report "$([ "$n" = 0 ] && echo no || echo yes)" "EC2 instances: $n (m7g.xlarge \$0.1166/h each while running)"
  n=$(aws_ ec2 describe-volumes --filters "$tagf" --query 'length(Volumes)' --output text)
  report "$([ "$n" = 0 ] && echo no || echo yes)" "EBS volumes: $n (gp3 \$0.0912/GB-month)"
  n=$(aws_ ec2 describe-snapshots --owner-ids self --filters "$tagf" --query 'length(Snapshots)' --output text)
  report "$([ "$n" = 0 ] && echo no || echo yes)" "EBS snapshots: $n"
  n=$(aws_ ec2 describe-addresses --filters "$tagf" --query 'length(Addresses)' --output text)
  report "$([ "$n" = 0 ] && echo no || echo yes)" "Elastic IPs: $n (\$0.005/h each, idle or not)"
  # Untagged leftovers can't be told apart by tag: list them so a person can.
  n=$(aws_ ec2 describe-instances --filters Name=instance-state-name,Values=pending,running,stopping,stopped --query 'length(Reservations[].Instances[])' --output text)
  v=$(aws_ ec2 describe-addresses --query 'length(Addresses)' --output text)
  echo "         account-wide (any project, for information): $n instances, $v Elastic IPs" >&2
  n=$(aws_ ec2 describe-nat-gateways --filter Name=state,Values=pending,available "$tagf" --query 'length(NatGateways)' --output text)
  report "$([ "$n" = 0 ] && echo no || echo yes)" "NAT gateways: $n"
  v=0
  for line in api web worker sandbox; do
    n=$(aws ecr describe-images --repository-name "rolerank/$line" --query 'sum(imageDetails[].imageSizeInBytes)' --output text 2>/dev/null || echo 0)
    [ "$n" = None ] && n=0
    v=$((v + n))
  done
  report "$([ "$v" -le "$ECR_MAX_BYTES" ] && echo no || echo yes)" "ECR (rolerank/*): $((v / 1048576)) MiB, expected for current + previous release: ~\$$(awk "BEGIN{printf \"%.2f\", $v/1073741824*0.10}")/month (flagged above 3 GB)"
  v=$(aws_ s3api list-object-versions --bucket "$BUCKET" --query 'sum(Versions[].Size)' --output text 2>/dev/null || echo 0)
  [ "$v" = None ] && v=0
  report "$([ "$v" -le "$S3_MAX_BYTES" ] && echo no || echo yes)" "S3 backups (all versions): $((v / 1048576)) MiB: ~\$$(awk "BEGIN{printf \"%.3f\", $v/1073741824*0.025}")/month (flagged above 6 GB)"
  n=$(aws_ cloudwatch describe-alarms --query 'length(MetricAlarms)' --output text)
  report "$([ "$n" -le 10 ] && echo no || echo yes)" "CloudWatch alarms (account): $n (first 10 free)"
  for region in $(aws_ ec2 describe-regions --query 'Regions[].RegionName' --output text | tr -d '\r'); do
    [ "$region" = "$AWS_REGION" ] && continue
    n=$(aws ec2 describe-instances --region "$region" --filters "$tagf" --query 'length(Reservations[].Instances[])' --output text 2>/dev/null || echo 0)
    v=$(aws ec2 describe-volumes --region "$region" --filters "$tagf" --query 'length(Volumes)' --output text 2>/dev/null || echo 0)
    [ "$n$v" = 00 ] || report yes "$region: $n instances, $v volumes tagged RoleRank"
  done
  echo "  (other regions: nothing tagged RoleRank)" >&2
  if [ "$bad" = 1 ]; then
    printf '\n\033[41;97m STILL BILLING: see the BILLS lines above \033[0m\n' >&2
    return 1
  fi
  printf '\n\033[42;30m Audit clean: RoleRank bills only registry and backup storage (~%s/month) \033[0m\n' "\$0.30" >&2
}
