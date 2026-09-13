#!/usr/bin/env bash
#
# Bootstrap a remote host so `make deploy-ec2` can run against it: Docker
# Engine, the Compose v2 plugin, and the login user in the docker group.
#
# Streamed over SSH by `make bootstrap-ec2` and executed with `bash -s`. It is
# NOT run automatically by deploy-ec2 — installing packages mutates someone
# else's machine, so it stays an explicit, separate step.
#
# Idempotent: safe to re-run. Everything is checked before it is installed.
set -euo pipefail

COMPOSE_MIN="2.17.0"   # the version that introduced `docker compose up --wait`

log()  { printf '  %s\n' "$*"; }
fail() { printf '  FAIL: %s\n' "$*" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

WHO="$(id -un)"   # $USER is often unset in a non-interactive SSH session

# ---------------------------------------------------------------- privileges
if [ "$(id -u)" -eq 0 ]; then
  SUDO=""
elif have sudo && sudo -n true 2>/dev/null; then
  SUDO="sudo"
else
  fail "passwordless sudo is required (EC2's default users have it).
        Either run as root, or grant $WHO NOPASSWD sudo, or install Docker by hand."
fi

# ------------------------------------------------------------------- distro
# shellcheck disable=SC1091
. /etc/os-release
log "host:    ${PRETTY_NAME:-$ID} ($(uname -m))"

# Amazon Linux 2 predates dnf: it is yum + amazon-linux-extras. AL2023 and the
# RHEL family use dnf. Picking by what is actually on the box is more reliable
# than mapping distro IDs to package managers.
if have dnf; then   PKG=dnf
elif have apt-get; then PKG=apt
elif have yum; then PKG=yum
else PKG=""; fi

pkg_install() {
  case "$PKG" in
    apt) $SUDO apt-get update -qq && $SUDO apt-get install -y -qq "$@" ;;
    dnf) $SUDO dnf install -y -q "$@" ;;
    yum) $SUDO yum install -y -q "$@" ;;
    *)   fail "no supported package manager found (looked for dnf, apt-get, yum)" ;;
  esac
}

# curl is needed to fetch both the Docker installer and the compose plugin, and
# a minimal cloud image may not ship it.
have curl || { log "installing curl..."; pkg_install curl; }

# ------------------------------------------------------------------- docker
if have docker; then
  log "docker:  already installed ($(docker --version))"
else
  log "installing Docker Engine..."
  case "$ID" in
    ubuntu|debian)
      # The official convenience script. It also installs the Compose v2
      # plugin, which `apt install docker-compose` does NOT — that package is
      # the old Python v1 and rejects the --wait flag this project relies on.
      #
      # It installs from Docker's apt repo keyed on the release CODENAME, and
      # Docker publishes a codename some time after a new Ubuntu ships. On a
      # very fresh release (e.g. 26.04) it can fail outright, so fall back to
      # the distro's own docker.io package. That package does not carry the
      # Compose v2 plugin either, but the plugin step below installs it from
      # the official release regardless of how the engine arrived.
      if curl -fsSL https://get.docker.com | $SUDO sh; then
        log "engine:  installed via get.docker.com"
      else
        log "get.docker.com failed — this is expected on a release Docker has"
        log "         not published a repo for yet (${VERSION_CODENAME:-unknown codename})"
        log "         falling back to the distro package: docker.io"
        pkg_install docker.io
        log "engine:  installed via docker.io (distro package)"
      fi
      ;;
    amzn)
      # Amazon Linux 2 keeps docker in amazon-linux-extras; AL2023 has it in
      # the normal repos. Neither ships the Compose v2 plugin — it is installed
      # separately below.
      if [ "${VERSION_ID:-}" = "2" ] && have amazon-linux-extras; then
        $SUDO amazon-linux-extras install -y docker
      else
        pkg_install docker
      fi
      ;;
    rhel|centos|rocky|almalinux|fedora)
      pkg_install docker || pkg_install podman-docker
      ;;
    *) fail "unsupported distro '$ID' — install Docker manually, then re-run make preflight-ec2" ;;
  esac
fi

$SUDO systemctl enable --now docker >/dev/null 2>&1 || true

# ------------------------------------------------------------ compose plugin
compose_version() { docker compose version --short 2>/dev/null || true; }

need_plugin=0
CUR="$(compose_version)"
if [ -z "$CUR" ]; then
  need_plugin=1
  log "compose: v2 plugin not present"
elif [ "$(printf '%s\n%s\n' "$COMPOSE_MIN" "$CUR" | sort -V | head -n1)" != "$COMPOSE_MIN" ]; then
  need_plugin=1
  log "compose: v$CUR is older than the v$COMPOSE_MIN needed for --wait"
fi

if [ "$need_plugin" -eq 1 ]; then
  case "$(uname -m)" in
    x86_64)  PLUGIN_ARCH=x86_64 ;;
    aarch64|arm64) PLUGIN_ARCH=aarch64 ;;
    *) fail "unknown architecture $(uname -m) for the compose plugin" ;;
  esac
  DEST=/usr/libexec/docker/cli-plugins
  log "installing the Compose v2 plugin ($PLUGIN_ARCH)..."
  $SUDO mkdir -p "$DEST"
  $SUDO curl -fsSL \
    "https://github.com/docker/compose/releases/latest/download/docker-compose-linux-$PLUGIN_ARCH" \
    -o "$DEST/docker-compose"
  $SUDO chmod +x "$DEST/docker-compose"
fi

# -------------------------------------------------------------- docker group
# Without this every docker command needs sudo, and the Makefile does not use
# sudo. The change only takes effect on a NEW login, which is why
# `make bootstrap-ec2` re-runs preflight over a fresh SSH connection.
if [ "$(id -u)" -eq 0 ]; then
  log "group:   running as root, nothing to add"
else
  # The group may not exist if Docker was installed some other way, and
  # usermod against a missing group would abort the script under `set -e`.
  getent group docker >/dev/null 2>&1 || $SUDO groupadd docker
  if id -nG "$WHO" | tr ' ' '\n' | grep -qx docker; then
    log "group:   $WHO is already in the docker group"
  else
    $SUDO usermod -aG docker "$WHO"
    log "group:   added $WHO to docker (takes effect on the next login)"
  fi
fi

# ------------------------------------------------------------------- report
log "docker:  $(docker --version)"
CUR="$(compose_version)"
[ -n "$CUR" ] || fail "the Compose v2 plugin is still missing after install.
        It was placed in /usr/libexec/docker/cli-plugins; if this docker build
        searches elsewhere, copy it to /usr/local/lib/docker/cli-plugins."
log "compose: v$CUR"
log "bootstrap complete"
