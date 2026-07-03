# gh-ghs — switch GitHub accounts like nvm switches node

`ghs` switches your active GitHub account (via `gh`), keeps your git commit
identity in sync, always shows which account is active, and lets you **pin
directory trees to a profile** so the right identity *and* the right push
token apply there — statelessly, in every terminal, no matter what's
globally active.

```
$ ghs list
-> personal     mattylight22 <68877341+mattylight22@users.noreply.github.com>
   work         matt-thesis <matt@use-thesis.com>
     pin: /Users/you/work

$ ghs use work
Now using work (matt-thesis <matt@use-thesis.com>)
```

## Install

```sh
gh extension install OSSMafia/gh-ghs
gh ghs link      # adds `ghs` and `git-ghs` symlinks -> bare `ghs` and `git ghs` work
```

All three spellings are the same binary: `gh ghs status`, `git ghs status`, `ghs status`.

## Quick start

```sh
ghs add personal          # interactive: maps a gh account to a git identity
ghs add work              # runs `gh auth login` if the account isn't logged in yet
ghs use personal          # switch account + commit identity
ghs pin work ~/work       # everything under ~/work commits AND pushes as work, forever
ghs status                # what would happen if I commit/push right here?
```

## Why pinning is the point

Most "account switcher" tools mutate one global active account — which means
two terminals in different projects fight over it. ghs pins are **stateless**:
a pinned tree gets

- its profile's `user.name`/`user.email` via a native git `includeIf "gitdir:..."` include, and
- its profile's **push token** via `credential.username`, which gh's
  credential helper honors per-account.

So pushes from a pinned directory can't use the wrong account, even when the
global active account is something else. Nothing switches; nothing races.

**Honesty note:** switching accounts never touches branches, remotes, or
uncommitted work. The risk ghs manages is identity/auth mismatch — committing
or pushing as the wrong account.

## Always-visible active account

```sh
ghs init zsh --install    # prompt segment: ⎇ mattylight22 (red + "≠work" on mismatch)
```

`ghs prompt` does no subprocess work (two file reads) and is safe to call on
every prompt render.

## Guardrails, layered

1. **Pinned dirs (automatic):** wrong-token pushes are impossible — see above.
2. **Visible:** `ghs status` / the prompt segment go red on mismatch.
3. **Enforced (opt-in):** `ghs guard install` adds a pre-push hook to the
   current repo only; `ghs init claude --install` adds a Claude Code
   PreToolUse hook that blocks agent-initiated `git push`/`git commit` on
   mismatch; `ghs init cursor --install` writes a Cursor rules file.

## For agents (Claude Code, Cursor, scripts)

- Every command supports `--json` with stable schemas.
- Exit codes: `0` ok · `1` mismatch/check failure · `2` usage · `3` environment.
- Fully non-interactive when not on a TTY (`--yes`, `--username/--name/--email`).
- `ghs context` emits a snippet for CLAUDE.md describing the directory's
  account rules.

## Doctor

`ghs doctor` diagnoses the whole chain — gh version, account/profile mapping,
include ordering, **whether `git push` actually authenticates through gh**
(a fresh macOS setup usually uses osxkeychain instead: `gh auth switch` alone
would silently NOT change push auth), stale pins, broken symlinks. `--fix`
repairs what it safely can and asks before anything that changes auth.

## Uninstall — clean by design

```sh
ghs uninstall            # reverts every change ghs made, in order, and says so
gh extension remove gh-ghs
```

ghs never edits your `[user]` block: it adds exactly one `include.path` line
to your global gitconfig, and all its config lives in `~/.config/ghs/`.
Uninstall removes that one line — your original identity is live again
instantly — then unwinds credential wiring (only if ghs added it), guard
hooks, agent hooks, the zshrc snippet, and symlinks. gh accounts and tokens
are never touched. A verbatim backup of your original gitconfig is kept at
`~/.config/ghs/backup/gitconfig.orig` (use `--keep-backup` to retain it).

## How it works

| Concern | Mechanism |
|---|---|
| Account/token switching | `gh auth switch` (gh ≥ 2.40 multi-account) |
| Commit identity | one `include.path` → ghs-owned `ghs.gitconfig` (shadows, never edits, your `[user]`) |
| Folder pinning | `includeIf "gitdir:/path/"` blocks, regenerated from `~/.config/ghs/config.toml` |
| Pinned push auth | `credential."https://github.com".username` — gh's helper returns that user's token |
| `git ghs` | git's native `git-<name>` subcommand convention (a symlink) |

Nested pins: the deepest pin wins (blocks are ordered shallow→deep;
git's last include wins). Worktrees: a pin covers all worktrees of repos
under it; `ghs status` warns about the one trap (a worktree checked out
inside a pin whose main repo is outside).
