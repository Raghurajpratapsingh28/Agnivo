#!/usr/bin/env bash
#
# Agnivo remote deploy — run this LOCALLY to deploy to a server over SSH.
#
# It copies the repository to the server and runs scripts/deploy.sh there,
# so a full platform comes up with a single command from your laptop.
#
# Usage:
#   ./scripts/ssh-deploy.sh [options] <user@host>
#
# Options:
#   -i <key>        SSH private key file (passed to ssh/rsync).
#   -p <port>       SSH port (default 22).
#   --dir <path>    Remote install directory (default /opt/agnivo).
#   --domain <d>    Public platform domain, forwarded to deploy.sh.
#   --with-web      Also build/start the dashboard, forwarded to deploy.sh.
#   --no-build      Forwarded to deploy.sh (skip image build).
#   --pull          Forwarded to deploy.sh (pull base images).
#   --git <url>     Clone from this git URL on the server instead of rsync.
#   --branch <b>    Branch to checkout when using --git (default: current).
#   -h, --help      Show this help.
#
# Examples:
#   ./scripts/ssh-deploy.sh root@1.2.3.4
#   ./scripts/ssh-deploy.sh -i ~/.ssh/id_ed25519 --domain agnivo.example.com ubuntu@1.2.3.4
#   ./scripts/ssh-deploy.sh --git https://github.com/you/agnivo.git deploy@host
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

SSH_KEY=""
SSH_PORT="22"
REMOTE_DIR="/opt/agnivo"
GIT_URL=""
GIT_BRANCH=""
TARGET=""
# flags forwarded verbatim to deploy.sh
FWD=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    -i)          SSH_KEY="${2:-}"; shift 2 ;;
    -p)          SSH_PORT="${2:-}"; shift 2 ;;
    --dir)       REMOTE_DIR="${2:-}"; shift 2 ;;
    --git)       GIT_URL="${2:-}"; shift 2 ;;
    --branch)    GIT_BRANCH="${2:-}"; shift 2 ;;
    --domain)    FWD+=(--domain "${2:-}"); shift 2 ;;
    --with-web)  FWD+=(--with-web); shift ;;
    --no-build)  FWD+=(--no-build); shift ;;
    --pull)      FWD+=(--pull); shift ;;
    -h|--help)   grep '^#' "$0" | sed 's/^# \{0,1\}//' | sed '1d'; exit 0 ;;
    -*)          echo "unknown option: $1" >&2; exit 2 ;;
    *)           TARGET="$1"; shift ;;
  esac
done

[[ -z "$TARGET" ]] && { echo "ERROR: missing <user@host>. See --help." >&2; exit 2; }

SSH_OPTS=(-p "$SSH_PORT" -o StrictHostKeyChecking=accept-new)
[[ -n "$SSH_KEY" ]] && SSH_OPTS+=(-i "$SSH_KEY")

remote() { ssh "${SSH_OPTS[@]}" "$TARGET" "$@"; }

echo "==> Target: $TARGET   dir: $REMOTE_DIR"

# Determine current branch for --git default
if [[ -n "$GIT_URL" && -z "$GIT_BRANCH" ]]; then
  GIT_BRANCH="$(git -C "$ROOT" rev-parse --abbrev-ref HEAD 2>/dev/null || echo main)"
fi

# ── 1. Get the code onto the server ───────────────────────────────────────────
remote "mkdir -p '$REMOTE_DIR'"

if [[ -n "$GIT_URL" ]]; then
  echo "==> Cloning/updating $GIT_URL ($GIT_BRANCH) on server..."
  remote "if [ -d '$REMOTE_DIR/.git' ]; then \
            cd '$REMOTE_DIR' && git fetch --all && git checkout '$GIT_BRANCH' && git pull --ff-only; \
          else \
            git clone --branch '$GIT_BRANCH' '$GIT_URL' '$REMOTE_DIR'; \
          fi"
else
  echo "==> Syncing local working tree to server (rsync)..."
  RSYNC_SSH="ssh -p $SSH_PORT -o StrictHostKeyChecking=accept-new"
  [[ -n "$SSH_KEY" ]] && RSYNC_SSH="$RSYNC_SSH -i $SSH_KEY"
  rsync -az --delete \
    -e "$RSYNC_SSH" \
    --exclude '.git/' \
    --exclude 'node_modules/' \
    --exclude 'bin/' \
    --exclude '.next/' \
    --exclude '.turbo/' \
    --exclude '.pnpm-store/' \
    --exclude '.env' \
    --exclude '*.docx' \
    --exclude '.DS_Store' \
    "$ROOT/" "$TARGET:$REMOTE_DIR/"
fi

# ── 2. Run the server-side deploy ─────────────────────────────────────────────
echo "==> Running deploy.sh on server..."
# Build a properly quoted remote argument string (empty if no flags forwarded).
FWD_STR=""
if [[ ${#FWD[@]} -gt 0 ]]; then
  printf -v FWD_STR '%q ' "${FWD[@]}"
fi
remote "cd '$REMOTE_DIR' && chmod +x scripts/deploy.sh && ./scripts/deploy.sh ${FWD_STR}"

echo "==> Remote deploy finished for $TARGET"
