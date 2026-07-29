#!/usr/bin/env bash
set -euo pipefail

SKILL_LEAF="openproject-cli"

# Normalize any prefix of <root>/.claude/skills/openproject-cli to the
# canonical target. Reject a .claude/skills/<other> leaf. Prints target on stdout.
resolve_deploy_dir() {
  local input="${1%/}"                 # strip one trailing slash
  input="${input/#\~/$HOME}"           # expand leading ~
  case "$input" in
    */.claude/skills/"$SKILL_LEAF") printf '%s\n' "$input" ;;
    */.claude/skills/*)
      printf 'ERROR: refusing to deploy: %s — skill folder must be named %s\n' \
        "$input" "$SKILL_LEAF" >&2
      return 1 ;;
    */.claude/skills) printf '%s\n' "$input/$SKILL_LEAF" ;;
    */.claude)        printf '%s\n' "$input/skills/$SKILL_LEAF" ;;
    *)                printf '%s\n' "$input/.claude/skills/$SKILL_LEAF" ;;
  esac
}

# Version the source tree declares — the single source of truth we build to.
source_version() {
  sed -n 's/^const version = "\(.*\)"/\1/p' "$1/internal/cli/root.go" | head -n1
}

# Echo the version a prebuilt binary reports, or nothing if it cannot run on
# this host.
binary_version() {
  local bin="$1"
  [[ -x "$bin" ]] || return 0
  "$bin" --version 2>/dev/null | awk '{print $NF}' || true
}

# Copy binary + docs into the deploy dir.
deploy_files() {
  local src="$1" dst="$2"
  mkdir -p "$dst/bin"
  cp -f "$src/bin/$SKILL_LEAF" "$dst/bin/$SKILL_LEAF"
  cp -f "$src/SKILL.md" "$src/README.md" "$src/.env.example" "$dst/"
  cat > "$dst/.gitignore" <<'EOF'
# Live OpenProject credentials — never commit.
.env
EOF
}

print_syncthing_hint() {
  local dst="$1" dir="$1"
  while [[ "$dir" != "/" && -n "$dir" ]]; do
    if [[ -e "$dir/.stfolder" ]]; then
      local rel="${dst#"$dir"/}"
      printf '\n>> Syncthing folder detected at %s\n' "$dir"
      printf '   Credentials are per-device and should NOT sync.\n'
      printf '   Add these lines to %s/.stignore (per device):\n\n' "$dir"
      printf '       /%s/.env\n' "$rel"
      return 0
    fi
    dir="$(dirname "$dir")"
  done
}

main() {
  local repo; repo="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  local default="$HOME/.claude/skills/$SKILL_LEAF"

  printf 'Deploy %s skill to which directory?\n' "$SKILL_LEAF"
  printf '  (you may give <path>, <path>/.claude, <path>/.claude/skills, or the full path)\n'
  printf '  [default: %s]: ' "$default"
  local answer; read -r answer
  [[ -z "$answer" ]] && answer="$default"

  local dst; dst="$(resolve_deploy_dir "$answer")" || exit 1

  # Reuse the committed binary when it already runs on this host and matches the
  # source version; only fall back to a `go build` otherwise. FORCE_BUILD=1
  # skips the reuse check entirely.
  local want have
  want="$(source_version "$repo")"
  have="$(binary_version "$repo/bin/$SKILL_LEAF")"

  if [[ "${FORCE_BUILD:-0}" != 1 && -n "$have" && "$have" == "$want" ]]; then
    printf '\n>> Reusing prebuilt binary (v%s, runs on this %s host) — skipping build.\n' \
      "$have" "$(uname -m)"
  else
    if [[ "${FORCE_BUILD:-0}" == 1 ]]; then
      printf '\n>> Building binary via go build (FORCE_BUILD=1)...\n'
    elif [[ -z "$have" ]]; then
      printf '\n>> No usable prebuilt binary for this architecture — building via go build...\n'
    else
      printf '\n>> Prebuilt binary is v%s but source is v%s — rebuilding via go build...\n' \
        "$have" "$want"
    fi
    if ! command -v go >/dev/null 2>&1; then
      printf 'ERROR: go is required to build but was not found on PATH.\n' >&2
      printf '       Install Go: https://go.dev/dl/  then re-run.\n' >&2
      exit 1
    fi
    ( cd "$repo" && go build -o "bin/$SKILL_LEAF" ./cmd/openproject-cli )
    if ! "$repo/bin/$SKILL_LEAF" --version >/dev/null 2>&1; then
      printf 'ERROR: built binary failed to run (%s --version).\n' "$repo/bin/$SKILL_LEAF" >&2
      exit 1
    fi
    printf '>> Built bin/%s (smoke test OK)\n' "$SKILL_LEAF"
  fi

  printf '\n>> Deploying to %s\n' "$dst"
  deploy_files "$repo" "$dst"

  if ! "$dst/bin/$SKILL_LEAF" --version >/dev/null 2>&1; then
    printf 'ERROR: deployed binary failed to run (%s/bin/%s --version).\n' "$dst" "$SKILL_LEAF" >&2
    exit 1
  fi

  printf '\nSkill deployed to %s.\n' "$dst"
  printf '  1. Add your credentials one of two ways:\n'
  printf '       - cp %s/.env.example %s/.env  and fill it in, or\n' "$dst" "$dst"
  printf '       - export OPENPROJECT_API_KEY in your shell\n'
  printf '  2. Run: %s/bin/%s auth status\n' "$dst" "$SKILL_LEAF"

  print_syncthing_hint "$dst"
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
