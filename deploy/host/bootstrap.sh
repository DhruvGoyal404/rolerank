#!/usr/bin/env bash
# RoleRank host bootstrap (Ubuntu 24.04, arm64), run once by cloud-init from the host stack's
# user data. Idempotent: a second run changes nothing. Adapted from CodeJudge's.
#
# Installs: Docker Engine and the compose plugin (pinned, Docker's apt repository), gVisor runsc
# registered with Docker as runtime "runsc" with --oci-seccomp (the extractor selects it; the
# compose services stay on runc), age for backups, a swap file, unattended security upgrades,
# the AWS CLI, and the /opt/rolerank and /etc/rolerank layout. No secrets: deploy.sh renders
# them from SSM at deploy time.
set -euo pipefail

DOCKER_VERSION="5:29.8.2-1~ubuntu.24.04~noble"  # 2026-09-30
CONTAINERD_VERSION="2.3.6-1~ubuntu.24.04~noble" # 2026-09-25
COMPOSE_VERSION="5.5.1-1~ubuntu.24.04~noble"    # 2026-09-03
# The release CI proved on arm64 (PHASE1.md, .github/workflows/ci.yml "arm64 and gVisor").
GVISOR_RELEASE="20261012.0"
GVISOR_SHA512="3459ca3e496465e5b6a1318563ede4bb079bdf11ae93f4bb8d71df3cd2f19bfa6db7c1035d2f9e97b4f4e57c298a4f18b42af7297c048ec7e14ec819223684e4"
AWSCLI_VERSION="2.37.7" # 2026-09-30
AWSCLI_SHA256="07fb845602caf0a1fa72d860c95f5538e96f1cfef3b659642d1ca383ec3e79d6"
SWAP_FILE=/swapfile
SWAP_SIZE=4G

[ "$(id -u)" -eq 0 ] || { echo "run as root" >&2; exit 1; }
# shellcheck disable=SC1091
. /etc/os-release
[ "${VERSION_ID:-}" = "24.04" ] || { echo "Ubuntu 24.04 expected, found ${PRETTY_NAME:-unknown}" >&2; exit 1; }
[ "$(uname -m)" = aarch64 ] || { echo "arm64 expected (m7g), found $(uname -m)" >&2; exit 1; }
log() { printf '%s bootstrap: %s\n' "$(date -u +%FT%TZ)" "$*"; }
export DEBIAN_FRONTEND=noninteractive
changed=0
note() { changed=1; log "$*"; }

# --- base packages -------------------------------------------------------------
need=()
for p in ca-certificates curl gnupg jq unzip zstd age unattended-upgrades; do
  dpkg-query -W -f='${Status}' "$p" 2>/dev/null | grep -q "install ok installed" || need+=("$p")
done
if [ ${#need[@]} -gt 0 ]; then
  apt-get update -qq
  apt-get install -y -qq "${need[@]}"
  note "installed ${need[*]}"
fi

# --- Docker Engine (pinned) ----------------------------------------------------
keyring=/etc/apt/keyrings/docker.asc
if [ ! -s "$keyring" ]; then
  install -m 0755 -d /etc/apt/keyrings
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o "$keyring"
  chmod a+r "$keyring"
  note "added Docker's apt key"
fi
list=/etc/apt/sources.list.d/docker.list
want_list="deb [arch=$(dpkg --print-architecture) signed-by=$keyring] https://download.docker.com/linux/ubuntu ${VERSION_CODENAME} stable"
if [ "$(cat "$list" 2>/dev/null)" != "$want_list" ]; then
  echo "$want_list" >"$list"
  apt-get update -qq
  note "configured Docker's apt repository"
fi
installed() { dpkg-query -W -f='${Version}' "$1" 2>/dev/null || true; }
if [ "$(installed docker-ce)" != "$DOCKER_VERSION" ] || [ "$(installed containerd.io)" != "$CONTAINERD_VERSION" ] ||
  [ "$(installed docker-compose-plugin)" != "$COMPOSE_VERSION" ]; then
  apt-get install -y -qq --allow-downgrades \
    "docker-ce=$DOCKER_VERSION" "docker-ce-cli=$DOCKER_VERSION" \
    "containerd.io=$CONTAINERD_VERSION" "docker-compose-plugin=$COMPOSE_VERSION"
  apt-mark hold docker-ce docker-ce-cli containerd.io docker-compose-plugin >/dev/null
  note "installed Docker $DOCKER_VERSION (held)"
fi
systemctl enable --now docker >/dev/null 2>&1

# --- gVisor runsc, registered with --oci-seccomp -------------------------------
if [ "$(runsc --version 2>/dev/null | head -1)" != "runsc version release-${GVISOR_RELEASE}" ]; then
  tmp=$(mktemp -d)
  curl -fsSL -o "$tmp/gvisor.tar.zstd" "https://storage.googleapis.com/gvisor/releases/release/${GVISOR_RELEASE}/aarch64/gvisor.tar.zstd"
  echo "${GVISOR_SHA512}  $tmp/gvisor.tar.zstd" | sha512sum -c --quiet
  mkdir "$tmp/x" && tar --zstd -xf "$tmp/gvisor.tar.zstd" -C "$tmp/x"
  install -m 0755 "$(find "$tmp/x" -type f -name runsc -print -quit)" /usr/local/bin/runsc
  install -m 0755 "$(find "$tmp/x" -type f -name containerd-shim-runsc-v1 -print -quit)" /usr/local/bin/containerd-shim-runsc-v1
  rm -rf "$tmp"
  note "installed gVisor $GVISOR_RELEASE"
fi
daemon=/etc/docker/daemon.json
if ! jq -e '.runtimes.runsc.runtimeArgs | index("--oci-seccomp")' "$daemon" >/dev/null 2>&1; then
  runsc install -- --oci-seccomp
  systemctl restart docker
  note "registered runsc with --oci-seccomp"
fi
if ! jq -e '."log-opts"."max-size"' "$daemon" >/dev/null 2>&1; then
  tmpd=$(mktemp)
  jq '. + {"log-driver": "json-file", "log-opts": {"max-size": "10m", "max-file": "3"}}' "$daemon" >"$tmpd"
  install -m 0644 "$tmpd" "$daemon" && rm -f "$tmpd"
  systemctl restart docker
  note "capped container logs"
fi

# --- swap ------------------------------------------------------------------------
if [ ! -f "$SWAP_FILE" ]; then
  fallocate -l "$SWAP_SIZE" "$SWAP_FILE"
  chmod 0600 "$SWAP_FILE"
  mkswap "$SWAP_FILE" >/dev/null
  note "created $SWAP_SIZE swap file"
fi
swapon --show=NAME --noheadings | grep -qx "$SWAP_FILE" || { swapon "$SWAP_FILE"; note "enabled swap"; }
grep -q "^$SWAP_FILE " /etc/fstab || { echo "$SWAP_FILE none swap sw 0 0" >>/etc/fstab; note "swap in fstab"; }

# --- layout ----------------------------------------------------------------------
install -d -m 0755 /opt/rolerank /opt/rolerank/releases /opt/rolerank/state
install -d -m 0700 /etc/rolerank /var/lib/rolerank/backup

# --- unattended security upgrades ---------------------------------------------------
auto=/etc/apt/apt.conf.d/20auto-upgrades
want_auto=$'APT::Periodic::Update-Package-Lists "1";\nAPT::Periodic::Unattended-Upgrade "1";'
if [ "$(cat "$auto" 2>/dev/null)" != "$want_auto" ]; then
  echo "$want_auto" >"$auto"
  note "enabled unattended upgrades"
fi

# --- AWS CLI v2 (pinned, checksum) ---------------------------------------------------
if [ "$(aws --version 2>/dev/null | cut -d' ' -f1)" != "aws-cli/${AWSCLI_VERSION}" ]; then
  tmp=$(mktemp -d)
  curl -fsSL -o "$tmp/awscli.zip" "https://awscli.amazonaws.com/awscli-exe-linux-aarch64-${AWSCLI_VERSION}.zip"
  echo "${AWSCLI_SHA256}  $tmp/awscli.zip" | sha256sum -c --quiet
  unzip -q "$tmp/awscli.zip" -d "$tmp"
  "$tmp/aws/install" --update >/dev/null
  rm -rf "$tmp"
  note "installed AWS CLI $AWSCLI_VERSION"
fi

if [ "$changed" = 0 ]; then log "already up to date"; else log "done"; fi
