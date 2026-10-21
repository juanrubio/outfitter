# Outfitter — TUI Experience & Interaction Specification

- **Status:** Draft v1
- **Date:** 2026-10-19
- **Related:** [Product & Technical Specification](product.md)
- **Interactive design reference:** [docs/mockups/index.html](../mockups/index.html)

---

## 1. Experience Goal

Open `outfitter` on a Debian or Ubuntu machine—whether a fresh headless VPS or your daily desktop—and immediately see:
1. Which tools in your AI development stack are **installed**, **outdated**, or **not installed**.
2. Which backend manages each tool (`mise:aqua`, `mise:ubi`, `mise:npm`, `uv`, `apt`, `vendor`, or `source`) and whether `mise` itself is ready.
3. A fast, keyboard-first way to **select/deselect individual tools** (`Space`), **toggle an entire category** (`g`), **load a preset profile** (`p`), **queue all available updates** (`u`), and **review + execute the exact action plan** (`a`).

No disk changes ever happen on keypresses in the main list. Toggling tools builds a visible **Pending Plan** in the header and detail pane (`+3 install · ↑4 update · -1 remove`). Pressing `a` opens a dry-run **Plan & Apply** confirmation screen showing the exact commands (and whether `sudo` is needed) before anything is executed.

---

## 2. Information Hierarchy

Each tool row in the catalog list answers four questions at a glance:
1. **Desired & Current State:** Checkbox + action marker (`[✓]` installed, `[↑]` update queued, `[+]` install queued, `[-]` uninstall queued, `[ ]` not installed).
2. **Tool Name & Binary:** Prominent name (`Claude Code`, `Boardwalk`, `tmux`) and binary command if different.
3. **Version Status:** Installed version and available update arrow (`2.1.289 → 2.1.295` in amber when outdated, `0.4.2` in green when current, `—` in muted text when not installed).
4. **Backend Badge:** Quiet right-aligned badge (`vendor`, `mise:ubi`, `mise:npm`, `mise:aqua`, `apt`, `uv`) so you always know how the tool is managed.

The **Right-Hand Inspector Pane** (on wide terminals `>= 100` columns, or `Enter` on `80×24` compact terminals) shows for the focused tool:
- Full description, category, and upstream repository/package link.
- Exact installation state, binary path on disk (`~/.local/bin/boardwalk`), provenance (`Managed by mise` vs. `External PATH binary`),installed version, and latest upstream release.
- Which preset profiles include this tool (`Desktop`, `Headless Server`, `Minimal`).
- **Exact shell command(s)** Outfitter will run to install, update, or remove this tool.

---

## 3. Primary Layouts & Wireframes

### 3.1 Wide Layout (`120×40` and `160×48`)

At `100` columns or wider, the interface uses a split master-detail view:
- **Left pane (62% width):** Grouped catalog list with category headers, counts, selection checkboxes, version diffs, and backend tags.
- **Right pane (38% width):** Selected tool inspector + **Pending Action Plan** summary at the bottom right.

```text
  OUTFITTER   Profile: Full Desktop (custom) · mise 2026.10.1 ready       18/28 installed · 4 outdated · Plan: +2 ↑4
  ────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
  Filter: All (28)  │  Category: All (1-4)  │  [p] Profile  [u] Queue 4 updates  [a] Apply plan (6)       Checked 1m ago

  ▼ 1 · CODING HARNESSES (6/8 installed · 2 updates)        │  OpenAI Codex CLI                               codex
    [✓] Claude Code          2.1.289           vendor       │  OpenAI's local coding agent CLI for terminal workflows.
  > [↑] Codex CLI            0.163.0 → 0.165.0 mise:npm     │
    [↑] Muse Code            1.4.10  → 1.4.12  vendor       │  STATUS & VERSIONS
    [✓] Antigravity CLI      1.4.2             vendor       │  State          Update queued (0.163.0 → 0.165.0)
    [✓] OpenCode v2          2.0.23            vendor       │  Binary path    ~/.local/share/mise/shims/codex
    [✓] Pi Coding Agent      1.3.0             mise:npm     │  Backend        mise:npm (@openai/codex)
    [+] Gemini CLI           — → 0.43.0        mise:npm     │  Provenance     Managed via mise Node LTS runtime
    [ ] GitHub Copilot CLI   — (1.0.63)        mise:npm     │  Profiles       Desktop · Server · Harnesses · Minimal
                                                            │
  ▼ 2 · AI TUIS & PERSONAL TOOLS (4/6 installed · 1 update) │  COMMANDS FOR THIS TOOL
    [✓] Boardwalk            0.4.2             mise:ubi     │  Install        mise use --global npm:@openai/codex@latest
    [↑] Trailmark            0.3.0   → 0.3.1   mise:ubi     │  Update         mise upgrade npm:@openai/codex
    [+] Quotary              — → 0.1.0-dev     source       │  Uninstall      mise unuse --global npm:@openai/codex
    [ ] CodexBar CLI         — (0.77.0)        vendor       │
    [✓] OpenClaw             2026.9.6          mise:npm     │  ──────────────────────────────────────────────────────
    [✓] mercadona-cli        1.0.0             mise:npm     │  PENDING ACTION PLAN (6 actions ready)
                                                            │  + Install (2)  gemini-cli, quotary
  ▼ 3 · TERMINAL & SESSION MGMT (6/11 installed · 1 update) │  ↑ Update  (4)  codex, muse, trailmark, lazygit
    [✓] tmux                 3.4-1ubuntu1      apt          │  - Remove  (0)  none
    [✓] Hive                 0.1.2             mise:ubi     │
    [✓] GitHub CLI (gh)      2.61.0            apt          │  Press [a] to review commands & apply, or [x] to reset.
    [↑] Lazygit              0.44.0  → 0.45.1  mise:aqua    │
  ────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
  ↑↓/jk Move  Space Toggle  g Group  u Queue updates  p Profile  f Filter  / Search  a Apply (6)  ? Help  q Quit
```

### 3.2 Compact Layout (`80×24`)

Between `80` and `99` columns, the catalog list takes the full width so version diffs and backend badges never wrap or clip. Pressing `Enter` opens the selected tool's inspector as a full-screen card; `Esc` returns to the exact scroll position.

```text
  OUTFITTER  Desktop · mise ready        18/28 inst · 4 outdated · Plan: +2 ↑4
  ────────────────────────────────────────────────────────────────────────────
  ▼ 1 · CODING HARNESSES (6/8)
    [✓] Claude Code          2.1.289            vendor
  > [↑] Codex CLI            0.163.0 → 0.165.0  mise:npm
    [↑] Muse Code            1.4.10  → 1.4.12   vendor
    [✓] Antigravity CLI      1.4.2              vendor
    [+] Gemini CLI           — → 0.43.0         mise:npm
  ▼ 2 · AI TUIS & PERSONAL TOOLS (4/6)
    [✓] Boardwalk            0.4.2              mise:ubi
    [↑] Trailmark            0.3.0   → 0.3.1    mise:ubi
    [+] Quotary              — → 0.1.0-dev      source
  ────────────────────────────────────────────────────────────────────────────
  jk Move  Space Toggle  Enter Details  u Updates  p Profile  a Apply (6)  ?
```

---

## 4. Core Interactive Workflows & Screens

### 4.1 Checkbox & Action Marker Semantics

Every tool has an **Installed State** on disk and a **Desired State** in the TUI:

| Marker | Color Token | Meaning | What `Space` Does |
| :--- | :--- | :--- | :--- |
| `[✓]` | `good` (Green) | Installed and up to date; kept installed | Marks `[-]` (queue for removal) |
| `[!]` | `low` (Amber) | Installed, upstream update available (not yet queued) | Cycles to `[↑]` (queue update), then `[-]` (queue removal), then `[!]` |
| `[↑]` | `accent` (Cyan) | Installed; queued for update in Pending Plan | Clears update back to `[✓]` / `[!]` |
| `[ ]` | `muted` (Gray) | Not installed | Marks `[+]` (queue for install) |
| `[+]` | `accent` (Cyan) | Not installed; queued for installation in Pending Plan | Clears back to `[ ]` |
| `[-]` | `error` (Rose) | Installed; queued for uninstall in Pending Plan | Restores back to `[✓]` |

- Pressing `u` (**Queue all updates**) turns every `[!]` outdated tool into `[↑]` in one keystroke (or Pressing `U` queues an immediate update for only the currently focused tool).
- Pressing `g` (**Toggle group**) toggles selection for all tools in the focused category (`1`–`4` also jump directly to categories 1–4; `Tab` collapses the current category and pressing it again reopens the last collapsed one).
- Pressing `x` (**Reset plan**) discards all queued `[+]`, `[↑]`, `[-]` changes and restores the view to current disk state.

### 4.2 Profile Selector Modal (`p`)

Pressing `p` opens the **Profile Selector** overlay, allowing you to outfit a machine in seconds:

1. **Full Desktop (`desktop`) — 28 tools:** All Coding Harnesses, AI TUIs (`boardwalk`, `trailmark`, `quotary`, `codexbar`, `openclaw`, `mercadona-cli`), Terminal tools (`tmux`, `hive`, `lazygit`, `gh`, `fzf`, `delta`, `glow`, `ripgrep`, `fd`, `jq`, `btop`), and Agent Utilities (`browser-use`, `mcporter`, `oracle`, `summarize`, `wacli`, `hcloud`).
2. **Headless Server / VPS (`server`) — 19 tools:** Tuned for remote Ubuntu agent hosts (like a Hetzner VPS or home server): Harnesses (`claude-code`, `codex`, `muse`, `antigravity-cli`, `opencode`), TUIs (`boardwalk`, `trailmark`, `openclaw`, `mercadona-cli`), Terminal (`tmux`, `hive`, `gh`, `lazygit`, `fzf`, `delta`, `ripgrep`, `fd`, `jq`, `btop`), and Utilities (`wacli`, `hcloud`). Omits desktop-only GUI/browser tools.
3. **Coding Harnesses Only (`harnesses`) — 8 tools:** `claude-code`, `codex`, `muse`, `antigravity-cli`, `opencode`, `pi`, `gemini-cli`, `copilot-cli`.
4. **Minimal Core (`minimal`) — 9 tools:** `tmux`, `gh`, `fzf`, `ripgrep`, `jq`, `claude-code`, `codex`, `boardwalk`, `trailmark`.

Selecting a profile queues `[+]` for any missing tools in that profile (without automatically queuing `[-]` removals for extra tools already installed unless the user presses `P` for strict profile sync).

The selector lists whatever profiles the effective catalog defines — built-in, extend, and replacement catalogs alike. A replacement catalog may define no profiles at all: then no profile is active, the header labels the profile as **All tools**, and pressing `p` shows a notice ("This catalog defines no profiles; all tools are shown.") instead of an empty selector. When profiles exist but the default `desktop` profile does not, the first profile in the catalog starts active.

The header also identifies the catalog in effect. On wide terminals a compact label follows `OUTFITTER` for user catalogs and the demo (`[extend · <name>]`, `[replace · <name>]`, `[demo]`; the built-in catalog carries no label). Pressing `c` shows the full catalog source: demo snapshot, built-in, or the extend/replace user file with its name, path, and tool/category/profile counts.

### 4.3 Plan & Apply Dry-Run Screen (`a`)

Pressing `a` opens the **Plan & Apply** confirmation screen before any command runs:
- Lists every queued operation grouped by phase:
  1. **Bootstrap Prerequisite** (if `mise` is missing: install `~/.local/bin/mise v2026.10.1` with SHA-256 verification).
  2. **System Packages (`apt`)** — badged `[sudo]` if any `apt` package is queued, noting that Outfitter will verify `sudo` credentials once before starting.
  3. **Installations (`+`)** — shows tool, target version, backend, and exact shell command.
  4. **Updates (`↑`)** — shows tool, `old → new` version diff, backend, and exact shell command.
  5. **Removals (`-`)** — highlighted in rose with confirmation warning.
- **`Enter` / `y`:** Starts execution and transitions to the **Live Execution Progress** view.
- **`Esc` / `n`:** Returns to the catalog without running anything.
- **`s`:** Toggles removing stray binaries (unmanaged copies left behind by migrating updates) after applying — off by default, per plan. Only strays of steps that succeed are removed.

### 4.4 Live Execution Progress View

While applying the plan:
- Shows a progress bar (`3 of 6 completed`) and per-tool status rows (`✓ Completed in 2.1s`, `⠋ Running...`, `· Queued`, `✗ Failed`).
- Displays a live tail of the active tool's stdout/stderr at the bottom of the screen so you are never staring at an opaque spinner.
- If a tool fails, Outfitter records the error, continues with independent tools, and stays open on the summary screen so you can inspect the failure log (`l`) or retry failed items (`r`).

### 4.5 First-Launch / Missing `mise` Banner

When launched on a fresh Debian/Ubuntu machine where `~/.local/bin/mise` is not yet installed:
- The top status bar shows `mise: not installed (will auto-bootstrap)` in amber.
- Pressing `b` (or applying any plan that uses `mise`) shows the verified bootstrap step (`https://mise.jdx.dev` release tarball + SHA-256 check → `~/.local/bin/mise`).
- When `mise` is installed but not active in the user's shell (no `mise activate` hook and no `~/.local/share/mise/shims` on `PATH`), a persistent amber header line warns that mise-managed tools will not resolve in the terminal. Pressing `B` offers (after `y` confirmation) to append a marked `eval "$(mise activate bash|zsh)"` block to the shell rc file; `outfitter doctor --fix [--shims]` does the same from the CLI. Fish and other shells get the manual line only.

---

## 5. Keyboard Reference

| Key | Context | Action |
| :--- | :--- | :--- |
| `↑` / `↓` or `j` / `k` | Catalog list | Move selection between tools |
| `1` / `2` / `3` / `4` | Catalog list | Jump to Category 1 (Harnesses), 2 (AI TUIs), 3 (Terminal), 4 (Utilities) |
| `Tab` | Catalog list | Collapse the focused category; pressing Tab again before navigating reopens the most recently collapsed category (LIFO stack). Number keys `1`–`4` also expand and jump to a category |
| `Space` | Catalog list | Toggle selected tool (`[ ]` ↔ `[+]`, `[!]` → `[↑]` → `[-]`, `[✓]` ↔ `[-]`) |
| `u` | Global | Queue updates (`[↑]`) for **all** outdated tools |
| `U` | Catalog list | Queue update (`[↑]`) for the currently focused tool only |
| `g` | Catalog list | Toggle all tools in the focused category |
| `p` | Global | Open **Profile Selector** for the effective catalog's profiles (notice only when the catalog defines none) |
| `a` | Global | Open **Plan & Apply** review screen to confirm and run queued changes |
| `x` | Global | Reset/clear all pending changes in the current plan |
| `f` | Global | Cycle view filter: `All` → `Installed` → `Outdated` → `Pending Changes` → `Not Installed` |
| `/` | Global | Incremental search by tool name, binary, backend, or description (`Esc` clears) |
| `r` | Global | Refresh installed states and upstream version checks |
| `B` | Global | Fix mise shell integration (confirmed; appends the marked `mise activate` block to the shell rc) |
| `S` | Global | Remove all shadowed binaries (confirmed; status bar shows `⚠ N shadowed` while any exist; sudo pre-flight for system paths) |
| `c` | Global | View the effective catalog source (built-in, demo, or extend/replace user catalog with name, path, and counts) |
| `Enter` | Compact / Plan | Open tool detail card (in `80×24` mode) or confirm Apply (in Plan modal) |
| `s` | Plan modal | Toggle removing stray binaries after applying (opt-in per plan; succeeded steps only) |
| `?` | Global | Toggle keyboard help overlay |
| `q` | Global | Open the **Quit Outfitter?** confirmation modal (Cancel/Quit buttons; warns when a Pending Plan has unapplied changes) |

---

## 6. Visual & Accessibility Rules

Following the same visual standard as Quotary and Trailmark:
- **Color Palette Tokens:**
  - `canvas`: `#0b1020` (Midnight) / `#f8fafc` (Daylight) / `#0e1117` (Monochrome)
  - `surface`: `#151d2c` (Midnight) / `#ffffff` (Daylight) / `#161b22` (Monochrome)
  - `border`: `#344155` (Midnight) / `#cbd5e1` (Daylight) / `#8c929d` (Monochrome)
  - `text`: `#e6edf3` / `#0f172a` / `#f1f1f1`
  - `muted`: `#a6b2c3` / `#475569` / `#c2c2c2`
  - `accent`: `#7dd3fc` / `#0369a1` / `#ffffff`
  - `good`: `#86efac` / `#166534` / `#f1f1f1`
  - `low`: `#fcd34d` / `#92400e` / `#ffffff`
  - `error`: `#fda4af` / `#be123c` / `#ffffff`
  - `stale`: `#c4b5fd` / `#6d28d9` / `#dddddd`
- **No-Color / Monochrome Safety:** Every state is encoded with distinct ASCII/Unicode text symbols (`[✓]`, `[!]`, `[↑]`, `[+]`, `[-]`, `[ ]`, and `>` for the focused row), so `NO_COLOR=1` and monochrome terminals retain 100% of the information hierarchy.
- **No Nerd Font Requirement:** Uses standard Unicode box-drawing and geometric characters available in DejaVu Sans Mono / Liberation Mono.
