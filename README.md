# Outfitter

**Your AI development toolkit, installed and kept current.**

Outfitter lets you bootstrap, inspect, select, and bulk-update the command-line coding agents, personal AI TUIs, and terminal tools you rely on for AI-assisted software development on Debian and Ubuntu Linux — all from your shell, at keyboard speed.

It wraps [`mise`](https://mise.jdx.dev) (managing `aqua`, `ubi`, GitHub Releases, `npm`, and `uv` tools plus isolated Node and Python runtimes) alongside thin adapters for Debian/Ubuntu `apt` packages, local Go source builds, and self-updating vendor harnesses (`claude`, `muse`, `agy`, `opencode`, `codexbar`).

![Outfitter in action: browsing the AI tool catalog, switching categories and profiles, queuing updates, reviewing the dry-run execution plan, and applying changes](docs/media/showcase.gif)

*Recorded from the application using fictional tool states (`outfitter --demo`). [Recreate the demo](tapes/README.md).*

## Install

1. Download the binary for your Linux system (`amd64` or `arm64`) from the [latest release](https://github.com/juanrubio/outfitter/releases/latest), verify it against `SHA256SUMS`, make it executable, and place it on your `PATH` (for example `~/.local/bin/outfitter`).
2. Confirm with `outfitter --version`. Release binaries are standalone and do not require Go or `mise` pre-installed.

If `mise` is not yet installed on a fresh Debian or Ubuntu machine, Outfitter automatically downloads the official `mise` release archive, verifies its SHA-256 digest against `SHASUMS256.txt`, and installs `~/.local/bin/mise` (`0755`) as Step 0 of your first plan. When `gh` (GitHub CLI) is installed and logged in, Outfitter also borrows `gh auth token` for upstream GitHub Release checks so version discovery never hits anonymous rate limits.

Prefer building from source? With Go installed: `go build -o outfitter ./cmd/outfitter` (or `make build`).

Outfitter also watches its own releases. When a newer version is available, the TUI shows an update line and `outfitter check` reports it separately from catalog tools. Press `O` in the TUI or run `outfitter self-update` to confirm and install it; mise-managed copies update through `mise`, while standalone binaries are downloaded, SHA-256-verified, and atomically replaced. Restart Outfitter afterward to run the new version.

## Quick start

```sh
outfitter                                  # open the interactive TUI
outfitter --demo                           # explore with offline fictional data
```

Inside the TUI, everything is keyboard-driven and the footer always shows the keys that matter right now. Press `?` any time for the full shortcut reference. The keys you will use most:

| Key | What it does |
|---|---|
| `↑↓` or `j` / `k` | Move between tools |
| `1` – `5` | Jump to a category (`All`, `Coding Harnesses`, `AI TUIs`, `Terminal & Shell`, `CLI Utilities`) |
| `Tab` / `Shift+Tab` | Switch focus between the tool list, category bar, and detail pane |
| `[` / `]` | Cycle preset profiles (`Full Desktop`, `Headless Server / VPS`, `Coding Harnesses Only`, `Minimal Core`) |
| `Space` | Toggle action for the selected tool (`[+] Install`, `[↑] Update`, `[-] Remove`, or clear) |
| `u` | Queue updates for all outdated tools |
| `i` | Queue installs for all missing tools in the current view |
| `a` | Toggle all visible tools in the current view |
| `/` | Filter tools by name, binary, package, or description |
| `Enter` | Open the **Dry-Run Plan Review** modal to inspect exact `$ <cmd>` steps and confirm execution |
| `S` | Review shadowed binaries and choose which unmanaged copies to remove |
| `t` | Cycle color themes (`Midnight`, `Daylight`, `Dracula`, `Nord`, `Monochrome`) |
| `O` | Update Outfitter itself when a newer release is available |
| `r` | Refresh installed and upstream versions |
| `?` | Open the keyboard shortcut reference |
| `q` | Quit Outfitter |

Outfitter opens immediately from a local state snapshot (`~/.cache/outfitter/state.json`) and refreshes in the background, so you are never staring at a blank terminal. Selecting or deselecting tools only mutates an in-memory **Pending Action Plan** (`+N install`, `↑N update`, `-N remove`) — nothing on disk is touched until you press `Enter` and confirm on the Dry-Run Plan Review screen.

## From the command line

The same catalog, profile, and update workflows work as scriptable commands with deterministic exit codes (`0` success, `1` partial/complete tool failure, `2` usage error or unconfirmed non-interactive pipe):

```sh
outfitter status                           # list all tools, installed versions, provenance, and updates
outfitter check                            # query upstream registries for available updates
outfitter profile list                     # list built-in and custom profiles
outfitter profile apply server --yes       # bootstrap a headless VPS profile non-interactively
outfitter install claude-code codex --yes  # install specific tools
outfitter update --all --yes               # bulk-update every outdated tool
outfitter remove aider --yes               # uninstall specific tools
outfitter self-update                      # update Outfitter itself (or use --check / --dry-run first)
outfitter catalog show                     # which catalog is in effect (built-in, extend, or replace)
outfitter catalog validate                 # check your catalog.toml without touching any tools
```

Every mutating command first builds the ordered execution plan. Preview safely with `--dry-run`; in scripts, pass `--yes` to confirm (without it, a piped command that would modify your system prints the dry-run plan and exits with code `2`). Add `--json` when a script needs structured machine-readable output.

## Customizing your catalog (`catalog.toml`)

Outfitter ships with a built-in catalog of **49 curated tools** across 4 categories and 4 profiles (`desktop`, `server`, `harnesses`, `minimal`), embedded in the binary. There is nothing to generate before your first run: Outfitter works out of the box with those defaults.

A **user catalog** is optional. Use one only when you want to override a built-in tool, add your own tools, disable a tool, or define a custom profile. A user catalog declares how it combines with the built-in catalog in an optional header:

```toml
[catalog]
schema_version = 1
mode = "extend"        # or "replace"
name = "Workstation"   # optional label shown by `catalog show` and the TUI
```

Files without a header are treated as `extend`, so existing overlays keep working. There are two ways to create a user catalog.

### Option A — Extend: write a small overlay (recommended for a few tweaks)

Run `outfitter catalog init --extend`, or create `~/.config/outfitter/catalog.toml` by hand containing only your changes. Outfitter picks this file up automatically when it exists. At startup it loads the built-in catalog first, then merges your file over it by tool `id`:

- A `[[tools]]` entry whose `id` matches a built-in tool overrides only the fields you specify; everything else keeps the built-in value.
- An entry with a new `id` adds your tool to the catalog.
- An entry with `disabled = true` hides a built-in tool.
- A `[profiles.<name>]` table adds (or replaces) a machine profile.

```toml
# Override a built-in tool to build from local source while developing
[[tools]]
id = "quotary"
backend = "source"
source_dir = "~/repos/quotary"
build_cmd = "go build -o ~/.local/bin/quotary ./cmd/quotary"

# Add a custom tool to the catalog
[[tools]]
id = "ttyd"
name = "ttyd"
category = "terminal"
binary = "ttyd"
backend = "mise:ubi"
package = "tsl0922/ttyd"
profiles = ["desktop", "server"]
description = "Share terminal sessions over the web"

# Define a custom profile
[profiles.vps-worker]
name = "VPS Worker"
short = "VPS"
description = "Headless agent worker for Hetzner VPS"
tools = ["tmux", "gh", "claude-code", "codex", "openclaw", "boardwalk", "ripgrep", "jq"]
```

Because the overlay is merged at every startup, it keeps tracking future built-in defaults for everything you did not specify. Note that deleting an entry from an extend catalog only removes your override — the built-in tool comes back. To hide a built-in tool in extend mode, keep an entry with `disabled = true`.

### Option B — Replace: own the whole catalog

To maintain a complete catalog of your own — only your tools, nothing inherited — create a replacement catalog:

```sh
outfitter catalog init     # seeded from the complete built-in catalog
```

This writes `~/.config/outfitter/catalog.toml` with a `mode = "replace"` header plus every built-in setting, category, profile, and tool (it refuses to overwrite an existing file; `--force` keeps the old file as a `.bak` sibling, and `--output PATH` writes elsewhere). Edit it down to the tools you actually want: in replace mode, the file is the whole catalog, so **deleting an entry really deletes it**, and built-in tools, categories, and profiles are never merged in. Prefer starting empty? `outfitter catalog init --blank` writes a skeleton with commented examples — but a replacement catalog needs at least one active tool, so it stays unusable until you add a `[[tools]]` entry.

For a dotfiles-managed catalog, write it anywhere and select it explicitly (note that `--catalog` is a global flag), or symlink it to the default path:

```sh
outfitter catalog init --output ~/dotfiles/outfitter-catalog.toml
outfitter --catalog ~/dotfiles/outfitter-catalog.toml status
# or: ln -s ~/dotfiles/outfitter-catalog.toml ~/.config/outfitter/catalog.toml
```

Without `--catalog`, only `~/.config/outfitter/catalog.toml` is picked up automatically. Keep in mind that a seeded replacement freezes today's built-in values: after an Outfitter upgrade it will not pick up new built-in tools or changed defaults — prefer Option A unless you really want to own the whole catalog. (`outfitter catalog export` prints the raw built-in catalog for inspection; on its own it has no header, so saving it as your user catalog would make it an extend overlay, not a replacement — use `catalog init` for that.)

### Checking your catalog

```sh
outfitter catalog validate   # parse and validate the selected catalog (or: validate <path>)
outfitter catalog show       # effective catalog: mode, origin, path, and tool/category/profile counts
outfitter profile list       # built-in and custom profiles
outfitter status             # every tool in the effective catalog
```

If the user catalog cannot be parsed or fails validation, Outfitter reports an error instead of silently falling back to the defaults. `catalog validate`, `catalog init`, and `catalog export` still work so you can inspect and fix the file, and `--ignore-user-catalog` runs any command against the built-in catalog alone as an escape hatch:

```sh
outfitter --ignore-user-catalog status
```

## Updating Outfitter itself

Outfitter is deliberately **not** one of its own catalog tools, so replacement catalogs cannot hide its update state. During upstream refreshes it checks GitHub's latest Outfitter release at most once a day and caches the result in `~/.cache/outfitter/self-check.json`. `outfitter status` shows the cached notice and `outfitter check` performs a fresh check, reporting Outfitter separately from catalog tools (including a `self` object with `--json`).

```sh
outfitter self-update --check    # report only
outfitter self-update --dry-run  # show what would happen
outfitter self-update            # confirm, update, then restart Outfitter
```

The update method follows how the running binary was installed: mise-managed copies update through `mise`; writable standalone binaries are replaced only after checksum and `--version` verification, with the previous binary kept beside the new one as `<binary>.previous`. Read-only/system installs and development builds report the release but are never modified automatically. To silence background notices, set `self_update_check = false` under `[settings]` in your catalog or set `OUTFITTER_NO_UPDATE_CHECK=1`; an explicit `outfitter self-update` still checks when you ask it to.

## Good to know

- **No disappearing NPM harnesses.** Node-based coding agents (`codex`, `pi`, `gemini-cli`, `copilot-cli`, `openclaw`, `mcporter`, `oracle`, `summarize`) run on a `mise`-managed Node LTS runtime with stable shims, so switching project Node versions in `nvm` never breaks your global AI CLIs.
- **Per-tool failure isolation and structured logs.** If one upstream installer fails during a bulk update, Outfitter records its exit code and continues updating the remaining independent tools. Every run writes a full timestamped log to `~/.local/state/outfitter/logs/run-<timestamp>.log`.
- **Shadowed binaries are opt-in by item.** When an unmanaged binary on `PATH` hides a managed one, the status bar counts it; pressing `S` lists every path and version first, and only the copies you leave checked are removed.
- **Companion bundle support.** Native vendor installers handle special layouts automatically — including extracting `codexbar`'s `CodexBarCLI` executable alongside its required `CodexBarCLI_CodexBarCore.bundle` into `~/.local/share/outfitter/codexbar/` and symlinking `~/.local/bin/codexbar`.

## Contributing — humans and AI tools welcome

Outfitter itself was built with AI assistance, and contributions made that way are welcome here. Point your coding agent — or yourself — at [AGENTS.md](AGENTS.md): it has the map of the repo, how to build and verify, and the rules that aren't obvious from the code. Contributions are judged on the change itself: specs respected, `make check` green. For anything bigger than a small fix, an issue first saves everyone time; [CONTRIBUTING.md](CONTRIBUTING.md) walks through the loop.

## Documentation and development

- [Product & technical specification](docs/specs/product.md) and the [TUI specification](docs/specs/tui.md) describe the exact architecture, catalog schema, keyboard state machine, and CLI contract; the [documentation index](docs/README.md) links all project records.
- [Interactive HTML mockup](docs/mockups/index.html) — open locally in a browser to explore the character-grid reference across terminal sizes and themes (`Midnight`, `Daylight`, `Monochrome`) with zero network dependencies.
- [Traceability matrix](docs/evidence/traceability.md) maps every product and TUI requirement to its implementation and offline tests.
- [VHS tapes](tapes/README.md) hold the scripted showcase recording used for the README animation above.
- Hacking on it: `make check` runs `go vet`, the pinned `golangci-lint`, `docs-check`, and the full offline test suite; `make race` runs tests under the Go race detector; `make build` builds `./outfitter`.

## License

[MIT](LICENSE) © Juan A. Rubio
