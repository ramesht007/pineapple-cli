# pine

Sync your Claude Code setup across machines through a private GitHub repo.

## What it syncs

- Skills (`~/.claude/skills`) and mods (`~/.claude/mods`)
- The plugin set, pinned by version and commit SHA
- An allowlist of `settings.json` keys (`enabledPlugins`, `extraKnownMarketplaces`, `theme`, `statusLine`)
- Secrets, age-encrypted (`secrets.env.age`), applied to `~/.claude/.env`

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/ramesht007/pineapple-cli/main/install.sh | bash
# or
brew install ramesht007/tap/pine
# or
go install github.com/ramesht007/pineapple-cli@latest
```

## First machine

```sh
pine init
pine daemon install
```

`pine init` installs `git` and `gh` if missing (it asks first; `--yes` auto-approves), logs you in with the GitHub device flow (works headless, no SSH keys), creates a private repo (`pineapple-config`, `--public` to opt out), seeds it, and prints your age public key plus where `age.key` lives. `pine daemon install` then schedules background sync.

## Every other machine

```sh
# install pine, then:
pine init                                      # detects your existing repo and joins it
# or explicitly:
pine init --from ramesht007/pineapple-config
```

Copy `~/.pineapple/age.key` from another machine (keep it mode 0600) so secrets decrypt, then run `pine daemon install`.

## Commands

| Command | What it does |
|---|---|
| `pine init [--from owner/repo]` | Create the sync repo, or join an existing one |
| `pine push` | Copy local config into the repo and push (secret-scanned) |
| `pine pull` | Fetch the repo and apply it locally |
| `pine sync [--now]` | Pull, then push if the push interval has elapsed |
| `pine status` | Machine, repo, last push/pull, local changes, remote-ahead, tracked counts |
| `pine secrets init` | Create the age key, or show the public key |
| `pine secrets set K=V ...` | Add or update secrets, push, apply |
| `pine secrets rm K ...` | Delete secrets, push, apply |
| `pine secrets list` | List secret keys (names only) |
| `pine secrets edit` | Edit all secrets in `$EDITOR` |
| `pine daemon install\|uninstall\|status\|run` | Manage the background sync job |
| `pine doctor` | Check setup and report problems |
| `pine version` | Print the version (banner on a terminal) |

Global flag: `-q/--quiet` prints only errors and conflicts. The banner and spinner show only on a terminal; set `NO_COLOR` to disable colour.

## Secrets

```sh
pine secrets set OPENAI_API_KEY=sk-... GITHUB_TOKEN=ghp_...
pine secrets list
pine secrets edit
```

The age private key is `~/.pineapple/age.key`. It is never committed or uploaded; copy it between machines yourself. The repo only holds ciphertext, so a public repo stays safe. Without the key, secrets simply stay encrypted.

## How it works

- Copy-based sync, not symlinks: a clone cache lives at `~/.pineapple/repo`.
- Remote wins on conflict. Overwritten local files are backed up to `~/.pineapple/backups`.
- Only allowlisted settings keys are synced.
- A pre-push secret scan aborts the push on any hit.
- The daemon (launchd on macOS, systemd user timer on Linux/WSL) pulls about every 15 minutes and pushes daily.
- A lock file prevents concurrent pine runs.

## Troubleshooting

- Run `pine doctor`. It checks git, config, repo, remote, gh, claude, age key, daemon and sync freshness, and prints a fix for each failure.
- Daemon log: `~/.pineapple/daemon.log`. `pine daemon status` shows whether the schedule is active.
- Conflict backups: `~/.pineapple/backups`.
- "another pine process is running": a sync (often the daemon) holds the lock; wait and retry.
- Building from source on a new macOS with an older Go: use `go build -ldflags=-linkmode=external` then `codesign -s - -f ./pine`, or the kernel kills the binary.
