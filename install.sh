#!/bin/bash
# aurscan installer / updater / uninstaller.
#
#   ./install.sh              build (needs Go) and install/update into PREFIX/bin
#   ./install.sh --uninstall  remove aurscan, syay, sparu, aurscan-edit from PREFIX/bin
#   ./install.sh --version    show what version would be built, then exit
#
# PREFIX defaults to /usr/local. Set SUDO= to install without sudo.
set -euo pipefail
cd "$(dirname "$0")"

PREFIX="${PREFIX:-/usr/local}"
BINDIR="$PREFIX/bin"
NAMES=(aurscan syay sparu aurscan-edit)
SUDO="${SUDO-sudo}"
PKGPATH="github.com/manticore-projects/aurscan/internal/version"

# Resolve version from git when available, else fall back to "dev". A release
# tarball without .git still yields a usable build via Go's embedded buildinfo.
resolve_version() {
  if git rev-parse --git-dir >/dev/null 2>&1; then
    VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
    COMMIT="$(git rev-parse --short=12 HEAD 2>/dev/null || true)"
  else
    VERSION="${AURSCAN_VERSION:-dev}"
    COMMIT=""
  fi
  DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}

build() {
  command -v go >/dev/null || { echo "Go is required to build aurscan"; exit 1; }
  resolve_version
  # Build offline from the committed vendor/ tree when present (and force it, so
  # a GOFLAGS=-mod=mod in the environment can't pull a non-vendored version).
  # A source tarball shipped without vendor/ falls back to normal module mode.
  local modflag=()
  [ -d vendor ] && modflag=(-mod=vendor)
  CGO_ENABLED=0 go build "${modflag[@]}" -trimpath \
    -ldflags="-s -w -X ${PKGPATH}.Version=${VERSION} -X ${PKGPATH}.Commit=${COMMIT} -X ${PKGPATH}.Date=${DATE}" \
    -o aurscan ./cmd/aurscan
}

uninstall() {
  local removed=0
  for n in "${NAMES[@]}"; do
    if [ -e "$BINDIR/$n" ] || [ -L "$BINDIR/$n" ]; then
      $SUDO rm -f "$BINDIR/$n"; removed=1
    fi
  done
  [ "$removed" = 1 ] && echo "Removed ${NAMES[*]} from $BINDIR" || echo "Nothing to remove in $BINDIR"
  echo "Note: if you enabled a native hook, remove it too:"
  echo "  aurscan --uninstall-yay-hook   /   aurscan --uninstall-paru-hook"
  echo "  and drop any 'alias yay=syay' / 'alias paru=sparu' (fish: functions -e yay; funcsave yay)."
}

install_update() {
  local action="Installed"
  [ -e "$BINDIR/aurscan" ] && action="Updated"
  build
  $SUDO install -Dm755 aurscan "$BINDIR/aurscan"
  $SUDO ln -sf "$BINDIR/aurscan" "$BINDIR/syay"
  $SUDO ln -sf "$BINDIR/aurscan" "$BINDIR/sparu"
  $SUDO ln -sf "$BINDIR/aurscan" "$BINDIR/aurscan-edit"
  echo "$action $("$BINDIR/aurscan" --version | head -1) -> $BINDIR"
  echo
  if command -v claude >/dev/null; then
    echo "  Backend: Claude Code CLI found — no API key needed."
  elif command -v codex >/dev/null; then
    echo "  Backend: Codex CLI found — no API key needed."
  elif [ -n "${ANTHROPIC_API_KEY:-}" ]; then
    echo "  Backend: ANTHROPIC_API_KEY is set."
  elif [ -n "${AURSCAN_OPENAI_URL:-}" ]; then
    echo "  Backend: local OpenAI-compatible endpoint ($AURSCAN_OPENAI_URL)."
  else
    echo "  Backend: none — install Claude Code/Codex CLI, set ANTHROPIC_API_KEY, or AURSCAN_OPENAI_URL."
    echo "           (static rules still run with no backend.)"
  fi
  echo
  echo "Enable the scanner — one command per helper:"
  local yaymajor=0
  if command -v yay >/dev/null; then
    yaymajor="$(yay --version 2>/dev/null | grep -oE 'v?[0-9]+' | head -1 | tr -d v)"
    yaymajor="${yaymajor:-0}"
  fi
  if [ "$yaymajor" -ge 13 ] 2>/dev/null; then
    echo "  yay (v$yaymajor):  aurscan --install-yay-hook   (native Lua hook)"
  elif command -v yay >/dev/null; then
    echo "  yay (v$yaymajor):  alias yay=syay   (fish: funcsave yay)   # v13+ supports --install-yay-hook"
  else
    echo "  yay v13+:  aurscan --install-yay-hook    ·   older yay:  alias yay=syay"
  fi
  echo "  paru:      aurscan --install-paru-hook   (native PreBuildCommand hook)"
}

case "${1:-}" in
  --uninstall|-u|uninstall) uninstall ;;
  --version|-v)             build; ./aurscan --version ;;
  ""|--install|install)     install_update ;;
  -h|--help)                sed -n '2,9p' "$0" | sed 's/^# \{0,1\}//' ;;
  *) echo "unknown option: $1 (try --help)"; exit 2 ;;
esac
