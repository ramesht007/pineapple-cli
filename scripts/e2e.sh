#!/usr/bin/env bash
# End-to-end test: two machines sync through a local bare repo. Never touches real ~/.claude or GitHub.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

export GIT_AUTHOR_NAME=e2e GIT_AUTHOR_EMAIL=e2e@example.com
export GIT_COMMITTER_NAME=e2e GIT_COMMITTER_EMAIL=e2e@example.com

(cd "$ROOT" && go build -ldflags=-linkmode=external -o "$TMP/pine" .)
# old Go toolchains on new macOS produce binaries the kernel kills unless re-signed
[ "$(uname)" = Darwin ] && codesign -s - -f "$TMP/pine" 2>/dev/null || true
git init -q --bare -b main "$TMP/remote.git"

A() { PINEAPPLE_HOME="$TMP/a-home" CLAUDE_DIR="$TMP/a-claude" "$TMP/pine" "$@"; }
B() { PINEAPPLE_HOME="$TMP/b-home" CLAUDE_DIR="$TMP/b-claude" "$TMP/pine" "$@"; }
assert_eq() { [ "$1" = "$2" ] || { echo "FAIL: $3: got '$1', want '$2'" >&2; exit 1; }; }

mkdir -p "$TMP/a-claude/skills/hello" "$TMP/a-claude/mods/m1" "$TMP/b-claude"
printf v1 > "$TMP/a-claude/skills/hello/SKILL.md"
echo '{}' > "$TMP/a-claude/mods/m1/plugin.json"

echo "== A seeds empty remote"
A init --from "$TMP/remote.git"
echo "== B joins"
B init --from "$TMP/remote.git"
assert_eq "$(cat "$TMP/b-claude/skills/hello/SKILL.md")" v1 "B initial skill"
[ -f "$TMP/b-claude/mods/m1/plugin.json" ] || { echo "FAIL: mod missing on B" >&2; exit 1; }

echo "== A edits, sync --now; B pulls"
printf v2 > "$TMP/a-claude/skills/hello/SKILL.md"
A sync --now
B pull
assert_eq "$(cat "$TMP/b-claude/skills/hello/SKILL.md")" v2 "B after pull"
B status

echo "== conflict"
printf vA > "$TMP/a-claude/skills/hello/SKILL.md"
printf vB > "$TMP/b-claude/skills/hello/SKILL.md"
A push
B pull
assert_eq "$(cat "$TMP/b-claude/skills/hello/SKILL.md")" vA "B takes remote on conflict"
backup="$(find "$TMP/b-home/backups" -type f -name SKILL.md | head -n1)"
[ -n "$backup" ] || { echo "FAIL: no backup under b-home/backups" >&2; exit 1; }
assert_eq "$(cat "$backup")" vB "backup content"

echo "== deletion"
rm -rf "$TMP/a-claude/skills/hello"
A push
B pull
[ ! -e "$TMP/b-claude/skills/hello" ] || { echo "FAIL: skill not deleted on B" >&2; exit 1; }

echo "== secret scan blocks push"
mkdir -p "$TMP/a-claude/skills/leak"; printf 'password = abcdefghijklmnop1234\n' > "$TMP/a-claude/skills/leak/SKILL.md"
if A push; then echo "FAIL: push should have been blocked" >&2; exit 1; fi

echo PASS
