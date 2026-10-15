# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`pine` — a Go CLI that syncs Claude Code user config (`~/.claude` skills, mods, plugin set, allowlisted settings keys, age-encrypted secrets) across machines through a GitHub repo (`ramesht007/pineapple-config`). Shell-outs to `git`, `gh`, and `claude`; no go-git.

## Commands

```bash
go build ./... && go vet ./... && go test ./...   # standard check; gofmt -l . must print nothing
go test ./internal/syncer/                        # single package
./scripts/e2e.sh                                  # end-to-end: two fake machines + local bare repo, must print PASS
```

- **This macOS aborts plain-built binaries** (`missing LC_UUID`, Go 1.22 internal linker). Build runnable binaries with `go build -ldflags=-linkmode=external -o pine . && codesign -s - -f pine`. The e2e script already does this.
- **Release**: `GOTOOLCHAIN=go1.24.0 GITHUB_TOKEN=$(gh auth token) ./scripts/release.sh vX.Y.Z` — the toolchain pin gives released darwin binaries LC_UUID. goreleaser also pushes the brew formula to `ramesht007/homebrew-tap`. After releasing, rebuild the locally installed binary at `~/.local/bin/pine` (external-linkmode build above).
- **Never test against the real `~/.claude`, `~/.pineapple`, or GitHub.** Every test path goes through the env overrides `PINEAPPLE_HOME` and `CLAUDE_DIR` (see `internal/paths`) plus a local bare repo as `--from` target. Manifest tests set `PINEAPPLE_NO_CLAUDE=1` so nothing shells out to `claude`.

## Hard constraints

- `go.mod` must keep `go 1.22` and no `toolchain` line (machine runs go1.22.3). This is why `filippo.io/age` is pinned to v1.2.1 (v1.3+ forces go ≥1.24) and lipgloss stays on v1 (`github.com/charmbracelet/lipgloss`, not `charm.land/lipgloss/v2`).
- Dependencies: cobra, lipgloss, age. Nothing else without strong reason.
- Plugin/secrets failures during `pull` are **warnings, never errors** — an unattended daemon pull must not break. Only settings/file-level failures may fail the pull.
- The settings sync is **allowlist-only** (`enabledPlugins`, `extraKnownMarketplaces`, `theme`, `statusLine`); `env`, `model`, and machine paths in `settings.json` must never be written to the repo (it held API keys historically; the repo may also be public in other users' setups).
- Secret scan (`internal/scan`) runs on every push over added diff lines and aborts the push on findings; `secrets.env.age` is exempt by basename.

## Architecture

Local truth is `~/.claude`; the clone at `~/.pineapple/repo` is a disposable cache (pull = `fetch` + `reset --hard origin/HEAD` + `clean -fdq`). Sync state lives in `~/.pineapple/state.json`: `Files` (relpath→sha256 at last successful sync) and `AppliedSHA` (origin commit last fully applied).

- `cmd/*` — thin cobra wrappers. Each top-level command (`pull`, `push`, `sync`, `secrets` mutations) takes the flock via `internal/lockfile` **once**, then calls unlocked flow funcs (`pullFlow`/`pushFlow`); don't double-lock (`pushFlow`'s non-FF retry calls `pullFlow` under the same lock).
- `internal/syncer` — the sync engine. Three-way logic in `Apply`: a file is a **conflict** (backed up to `~/.pineapple/backups/<ts>/`, then overwritten — remote wins) only when remote differs AND local hash ≠ state hash. Files in state but gone from repo are deleted locally. Local-new files are left for the next push. `pullFlow` saves state **only after full success**; a failed apply must leave stale state so the next pull re-applies (AppliedSHA is what prevents false "up to date" — see the stale-state e2e section before touching this).
- `internal/manifest` — plugin set + settings fragments. `Push` regenerates `manifest.json` (plugins pinned by version + marketplace commit SHA; byte-stable output so no-op pushes stay no-ops) and `settings.sync.json` from the live `~/.claude` files; directory-marketplace paths are rewritten to `${CLAUDE_DIR}` placeholders. `Apply` merges allowlisted settings keys wholesale, then best-effort installs: refresh marketplaces (`claude plugin marketplace add/update`), pin via `git fetch origin <sha>` into the (shallow) marketplace clone + checkout, `claude plugin install`, restore the clone's original ref, warn on drift. `builtin` marketplace is always skipped.
- `internal/secrets` — one armored age file (`secrets.env.age`) in the repo; identity at `~/.pineapple/age.key` (never synced). `Apply` decrypts into a marker-delimited managed block of `~/.claude/.env`, preserving all bytes outside the markers.
- `internal/ui` — gold/green lipgloss palette, banner (ANSI Shadow art — width test caps lines at 90 runes), spinner, box. Everything degrades: non-TTY stdout → no banner; `--quiet` (ui.Quiet) → spinner fully silent (daemon logs depend on this); `NO_COLOR` → plain text passthrough.
- `internal/daemon` — launchd plist / systemd user timer / cron line that run `pine sync --quiet` every PullIntervalMinutes; `sync` itself gates pushes to PushIntervalHours (daily). Templates are pure functions tested without touching the OS.

Cadence: daemon pulls every ~15 min, pushes at most daily; `pine sync --now` forces the push.
