#!/usr/bin/env bash
#
# Agnivo one-shot deployment script (run ON the server).
#
# Bootstraps a single host into a full Agnivo platform:
#   - installs Docker Engine + compose plugin if missing
#   - generates a production .env with strong secrets (idempotent)
#   - builds all service images and starts the full stack
#   - waits for the API to report healthy
#
# Usage (from the repo root on the server):
#   ./scripts/deploy.sh [options]
#
# Options:
#   --domain <domain>   Public platform domain (e.g. agnivo.example.com).
#                       Sets proxy-manager platform/preview domains.
#   --with-web          Also build and start the Next.js dashboard (frontend).
#   --no-build          Skip image build; only (re)start containers.
#   --pull              Pull base images (postgres/redis/caddy/...) before up.
#   --env-only          Only create/refresh .env, do not touch containers.
#   --down              Stop and remove the stack (keeps volumes), then exit.
#   --destroy           Stop the stack AND delete volumes (DESTROYS DATA), then exit.
#   -h, --help          Show this help.
#
set -euo pipefail

# ── locate repo root ──────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT"

ENV_FILE="$ROOT/.env"
ENV_EXAMPLE="$ROOT/.env.example"
COMPOSE_FILE="$ROOT/docker-compose.yml"

# ── options ───────────────────────────────────────────────────────────────────
DOMAIN="app.invorra.tech"   # default platform domain; override with --domain
WITH_WEB=0
DO_BUILD=1
PULL_IMAGES=0
ENV_ONLY=0
ACTION="up"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --domain)   DOMAIN="${2:-}"; shift 2 ;;
    --with-web) WITH_WEB=1; shift ;;
    --no-build) DO_BUILD=0; shift ;;
    --pull)     PULL_IMAGES=1; shift ;;
    --env-only) ENV_ONLY=1; shift ;;
    --down)     ACTION="down"; shift ;;
    --destroy)  ACTION="destroy"; shift ;;
    -h|--help)  grep '^#' "$0" | sed 's/^# \{0,1\}//' | sed '1d'; exit 0 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

# ── logging helpers ───────────────────────────────────────────────────────────
c_blue=$'\033[1;34m'; c_green=$'\033[1;32m'; c_yellow=$'\033[1;33m'; c_red=$'\033[1;31m'; c_off=$'\033[0m'
log()  { printf '%s==>%s %s\n' "$c_blue"   "$c_off" "$*"; }
ok()   { printf '%s ✓%s %s\n'  "$c_green"  "$c_off" "$*"; }
warn() { printf '%s ! %s %s\n' "$c_yellow" "$c_off" "$*"; }
die()  { printf '%sERROR:%s %s\n' "$c_red" "$c_off" "$*" >&2; exit 1; }

# ── privilege escalation ──────────────────────────────────────────────────────
SUDO=""
if [[ "$(id -u)" -ne 0 ]]; then
  if command -v sudo >/dev/null 2>&1; then SUDO="sudo"; fi
fi

# ── docker / compose resolution ───────────────────────────────────────────────
DC=""  # docker compose invocation

resolve_compose() {
  if docker compose version >/dev/null 2>&1; then
    DC="docker compose"
  elif command -v docker-compose >/dev/null 2>&1; then
    DC="docker-compose"
  else
    return 1
  fi
  return 0
}

install_docker() {
  log "Docker not found — installing Docker Engine..."
  if ! command -v curl >/dev/null 2>&1; then
    if command -v apt-get >/dev/null 2>&1; then $SUDO apt-get update -y && $SUDO apt-get install -y curl; fi
  fi
  curl -fsSL https://get.docker.com | $SUDO sh
  $SUDO systemctl enable --now docker 2>/dev/null || true
  # let the current (non-root) user talk to the daemon for this session
  if [[ -n "$SUDO" ]]; then $SUDO usermod -aG docker "$USER" 2>/dev/null || true; fi
}

ensure_docker() {
  if ! command -v docker >/dev/null 2>&1; then
    install_docker
  fi
  # daemon reachable?
  if ! docker info >/dev/null 2>&1; then
    if [[ -n "$SUDO" ]]; then
      warn "Docker daemon not reachable as this user; using sudo for docker commands."
      DOCKER_SUDO="$SUDO"
    else
      die "Docker daemon not reachable. Start it (systemctl start docker) and re-run."
    fi
  fi
  if ! resolve_compose; then
    die "Docker Compose v2 plugin not available. Install 'docker-compose-plugin'."
  fi
  ok "Using: $($DC version --short 2>/dev/null || echo "$DC")"
}

# docker compose wrapper honouring optional sudo + frontend profile
DOCKER_SUDO=""
compose() {
  local profile=()
  if [[ "$WITH_WEB" -eq 1 ]]; then profile=(--profile frontend); fi
  # shellcheck disable=SC2086
  $DOCKER_SUDO $DC -f "$COMPOSE_FILE" "${profile[@]}" "$@"
}

# returns 0 if compose supports multiline env_file values (>= 2.24)
compose_supports_multiline() {
  local v major minor
  v="$($DC version --short 2>/dev/null | tr -d 'v')"
  major="${v%%.*}"; minor="${v#*.}"; minor="${minor%%.*}"
  [[ -z "$major" || -z "$minor" ]] && return 1
  (( major > 2 )) && return 0
  (( major == 2 && minor >= 24 )) && return 0
  return 1
}

# ── .env management ───────────────────────────────────────────────────────────
env_has() { [[ -f "$ENV_FILE" ]] && grep -q "^$1=" "$ENV_FILE"; }

# set/replace a single-line key
env_set() {
  local key="$1" val="$2" tmp
  touch "$ENV_FILE"
  if env_has "$key"; then
    tmp="$(mktemp)"
    grep -v "^$key=" "$ENV_FILE" > "$tmp" && mv "$tmp" "$ENV_FILE"
  fi
  printf '%s=%s\n' "$key" "$val" >> "$ENV_FILE"
}

# set a single-line key only if it is currently absent or empty
env_default() {
  local key="$1" val="$2" cur
  cur="$(grep "^$key=" "$ENV_FILE" 2>/dev/null | head -1 | cut -d= -f2-)"
  if [[ -z "$cur" ]]; then env_set "$key" "$val"; fi
}

# append a quoted multi-line value from a file, only if key absent
env_set_multiline() {
  local key="$1" file="$2"
  env_has "$key" && return 0
  { printf '%s="' "$key"; cat "$file"; printf '"\n'; } >> "$ENV_FILE"
}

rand_hex() { # bytes
  if command -v openssl >/dev/null 2>&1; then openssl rand -hex "$1"
  else head -c "$1" /dev/urandom | od -An -tx1 | tr -d ' \n'; fi
}

generate_env() {
  log "Configuring environment (.env)..."
  if [[ ! -f "$ENV_FILE" ]]; then
    if [[ -f "$ROOT/.env.production" ]]; then
      cp "$ROOT/.env.production" "$ENV_FILE"
      ok "Created .env from .env.production (pre-generated secrets)"
    elif [[ -f "$ENV_EXAMPLE" ]]; then
      cp "$ENV_EXAMPLE" "$ENV_FILE"
      ok "Created .env from .env.example"
    else
      touch "$ENV_FILE"
      ok "Created empty .env"
    fi
  else
    ok ".env already present — filling in only what is missing"
  fi

  # Production posture (these are also overridable in docker-compose via ${VAR}).
  env_set "AGNIVO_APP_ENVIRONMENT" "production"
  env_default "AGNIVO_LOG_LEVEL"  "info"
  env_default "AGNIVO_LOG_FORMAT" "json"
  env_set "AGNIVO_DATABASE_ENABLED" "true"
  env_set "AGNIVO_REDIS_ENABLED"    "true"

  if [[ -n "$DOMAIN" ]]; then
    env_set "AGNIVO_PROXY_MANAGER_PLATFORM_DOMAIN" "$DOMAIN"
    env_set "AGNIVO_PROXY_MANAGER_PREVIEW_DOMAIN"  "preview.$DOMAIN"
    env_set "NEXT_PUBLIC_API_URL" "https://$DOMAIN"
    ok "Platform domain set to $DOMAIN"
  fi

  # ── secrets (generated once, never overwritten) ──
  if ! env_has "AGNIVO_SECURITY_INTERNAL_SERVICE_TOKEN"; then
    env_set "AGNIVO_SECURITY_INTERNAL_SERVICE_TOKEN" "$(rand_hex 48)"
    ok "Generated internal service token"
  fi
  if ! env_has "AGNIVO_SECURITY_METRICS_BEARER_TOKEN"; then
    env_set "AGNIVO_SECURITY_METRICS_BEARER_TOKEN" "$(rand_hex 32)"
    ok "Generated metrics bearer token"
  fi
  if ! env_has "AGNIVO_CONTROLPLANE_ENCRYPTION_KEY"; then
    env_set "AGNIVO_CONTROLPLANE_ENCRYPTION_KEY" "$(rand_hex 32)" # 32 bytes hex
    ok "Generated control-plane encryption key"
  fi

  # ── JWT RS256 keypair (PEM, multi-line) ──
  if ! env_has "AGNIVO_IDENTITY_JWT_PRIVATE_KEY_PEM"; then
    if ! command -v openssl >/dev/null 2>&1; then
      warn "openssl missing — JWT keys left empty (api will generate EPHEMERAL keys; tokens reset on restart)."
    elif ! compose_supports_multiline; then
      warn "Docker Compose < 2.24 cannot read multi-line env values."
      warn "JWT keys left empty (EPHEMERAL). Upgrade compose, or set the PEM keys via real env vars."
    else
      local tmpd; tmpd="$(mktemp -d)"
      openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "$tmpd/priv.pem" 2>/dev/null
      openssl pkey -in "$tmpd/priv.pem" -pubout -out "$tmpd/pub.pem" 2>/dev/null
      env_set_multiline "AGNIVO_IDENTITY_JWT_PRIVATE_KEY_PEM" "$tmpd/priv.pem"
      env_set_multiline "AGNIVO_IDENTITY_JWT_PUBLIC_KEY_PEM"  "$tmpd/pub.pem"
      rm -rf "$tmpd"
      ok "Generated persistent JWT RS256 keypair"
    fi
  fi

  chmod 600 "$ENV_FILE" || true
}

# ── health check ──────────────────────────────────────────────────────────────
admin_port() {
  local p; p="$(grep '^ADMIN_PORT=' "$ENV_FILE" 2>/dev/null | head -1 | cut -d= -f2-)"
  echo "${p:-9090}"
}

wait_for_health() {
  local port; port="$(admin_port)"
  local url="http://127.0.0.1:${port}/health/live"
  log "Waiting for API health at ${url} ..."
  local i
  for i in $(seq 1 60); do
    if curl -fsS "$url" >/dev/null 2>&1; then ok "API is healthy"; return 0; fi
    sleep 5
  done
  warn "API did not report healthy in time. Check logs: $DC -f docker-compose.yml logs -f api"
  return 1
}

# ── lifecycle actions ─────────────────────────────────────────────────────────
do_down()    { log "Stopping stack..."; compose down; ok "Stack stopped (volumes kept)."; }
do_destroy() { log "Destroying stack + volumes (DATA LOSS)..."; compose down -v; ok "Stack and volumes removed."; }

do_up() {
  if [[ "$PULL_IMAGES" -eq 1 ]]; then log "Pulling base images...";  compose pull --ignore-buildable || true; fi
  if [[ "$DO_BUILD"   -eq 1 ]]; then log "Building service images..."; compose build; fi
  log "Starting stack..."
  compose up -d --remove-orphans
  ok "Containers started"
  compose ps
  wait_for_health || true

  local port; port="$(admin_port)"
  echo
  ok "Deployment complete."
  echo "   API:     http://<host>:${BACKEND_PORT:-8080}/api/v1"
  echo "   Health:  http://<host>:${port}/health/live"
  if [[ "$WITH_WEB" -eq 1 ]]; then echo "   Web:     http://<host>:${WEB_PORT:-3000}"; fi
  echo
  echo "   Logs:    $DC -f docker-compose.yml logs -f api"
  echo "   Status:  $DC -f docker-compose.yml ps"
}

# ── main ──────────────────────────────────────────────────────────────────────
log "Agnivo deploy — root: $ROOT"
ensure_docker

case "$ACTION" in
  down)    do_down;    exit 0 ;;
  destroy) do_destroy; exit 0 ;;
esac

generate_env
[[ "$ENV_ONLY" -eq 1 ]] && { ok "--env-only set; skipping containers."; exit 0; }
do_up
