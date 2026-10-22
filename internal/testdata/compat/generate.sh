#!/bin/bash
# Generates internal/testdata/compat/<version>: builds cpm at that tag,
# installs a sample config with it and copies the result (minus symlinks),
# with the temp root replaced by $ROOT. Run from the repo root after each
# release, then add the version to compatVersions in compat_test.go:
#
#   internal/testdata/compat/generate.sh v0.8.0
#
# Versions from v0.4.0 get secrets.toml and inherit_mcp, from v0.5.0
# indicator and [[match]]. Extend the config as features are added.
set -euo pipefail
V="$1"; REPO="$(pwd)"
S="$(mktemp -d)"; trap 'git worktree remove --force "$S/src" >/dev/null 2>&1 || true; rm -rf "$S"' EXIT
git worktree add -q --detach "$S/src" "$V"
(cd "$S/src" && go build -o "$S/cpm-$V" .)
R="$S/compat-root"
rm -rf "$R"; mkdir -p "$R/home" "$R/claude/skills" "$R/bin" "$R/profiles"
echo '{"theme":"dark"}' > "$R/claude/settings.json"
echo '# shared' > "$R/claude/CLAUDE.md"
echo '{"hasCompletedOnboarding":true,"mcpServers":{"notes":{"command":"notes-mcp"}}}' > "$R/home/.claude.json"
cfg="$R/profiles/config.toml"
cat > "$cfg" <<TOML
# compat fixture for $V
source_dir = "$R/claude"
bin_dir = "$R/bin"

[profiles.work]
description = "Work"
model = "sonnet"
add_dirs = ["~/Work"]
isolate = ["projects"]
mcp_exclude = ["notes"]

[profiles.work.env]
TEAM = "core"

[profiles.work.attribution]
commit = "Co-Authored-By: Claude <noreply@anthropic.com>"

[profiles.work.settings]
effortLevel = "high"

[profiles.work.mcp_servers.jira]
command = "jira-mcp"

[profiles.personal]
description = "Personal"

# [cloud]
# remote = "git@example.com:me/settings.git"
TOML
case "$V" in
  v0.3.*) ;;
  *)
    printf '[profiles.personal.env]\nAPI_TOKEN = "fixture-secret"\n' > "$R/profiles/secrets.toml"; chmod 600 "$R/profiles/secrets.toml"
    sed -i '' 's/^\[profiles.personal\]$/[profiles.personal]\ninherit_mcp = false/' "$cfg" ;;
esac
if [[ "$V" != v0.3.* && "$V" != v0.4.* ]]; then
  sed -i '' '1a\
indicator = true
' "$cfg"
  printf '\n[[match]]\nremote = "github.com/company/*"\nprofile = "work"\n' >> "$cfg"
fi
HOME="$R/home" "$S/cpm-$V" --config "$cfg" install >/dev/null
OUT="$REPO/internal/testdata/compat/$V"
rm -rf "$OUT"; mkdir -p "$OUT"
(cd "$R" && find . -type f) | while read -r f; do
  mkdir -p "$OUT/$(dirname "$f")"
  sed "s#$R#\$ROOT#g" "$R/$f" > "$OUT/$f"
  chmod "$(stat -f %Lp "$R/$f")" "$OUT/$f"
done
echo "$V: $(cd "$OUT" && find . -type f | wc -l) files"
