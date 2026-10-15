# pineapple-cli

`pine` syncs your Claude Code skills, mods, plugin set, settings and secrets across machines through a GitHub repo.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/ramesht007/pineapple-cli/main/install.sh | bash
# or
brew install ramesht007/tap/pine
# or
go install github.com/ramesht007/pineapple-cli@latest
```

## Quick start

```sh
pine init                                      # first machine
pine init --from ramesht007/pineapple-config   # every other machine
pine daemon install                            # background sync
```

`pine init` creates the GitHub repo as private by default (`--public` to opt out).
On later machines plain `pine init` also works: it detects the existing repo and joins it.

## Auth

On first `pine init`, pine installs `git`/`gh` itself if missing (it asks first; `--yes` auto-approves), runs `gh auth login` for you, and runs `gh auth setup-git`. The only interaction needed is the browser/device-code auth step; the device flow works headless and no SSH keys are needed.

## Commands

| Command | What it does |
|---|---|
| `pine init` | Set up the sync repo (or clone an existing one with `--from`) |
| `pine push` | Copy local config into the repo and push |
| `pine pull` | Fetch the repo and apply it locally |
| `pine sync` | Pull, then push |
| `pine status` | Show what differs between local and repo |
| `pine secrets` | Manage age-encrypted secrets |
| `pine daemon` | Install/remove the background sync job |
| `pine doctor` | Check setup and report problems |

## How it works

- Copy-based sync: a clone of your config repo lives at `~/.pineapple/repo`; files are copied in and out, not symlinked.
- Conflicts: remote wins. Overwritten local files are backed up to `~/.pineapple/backups`.
- Settings are synced by allowlist only.
- Plugins are pinned by version and commit SHA.
- Secrets are age-encrypted (`secrets.env.age`), so a public repo stays safe.
- Daemon: launchd (macOS) or a systemd user timer (Linux/WSL). Pulls about every 15 minutes, pushes daily.

## Security

- Only allowlisted settings keys are ever synced.
- A pre-push secret scan aborts the push on any hit.
- The age key lives at `~/.pineapple/age.key`. Copy it between machines manually; it is never committed.
