# Outfitter

**Your AI development toolkit, installed and kept current.**

Outfitter (`outfitter`) is a keyboard-first Go TUI and scriptable CLI for bootstrapping, selecting, and bulk-updating the command-line harnesses and terminal tools used for AI-assisted software development on Debian and Ubuntu Linux.

It wraps [`mise`](https://mise.jdx.dev) (which manages `aqua`, `ubi`, GitHub Releases, `npm`, and `uv` tools plus isolated Node/Python runtimes) alongside thin adapters for Debian/Ubuntu `apt` packages and self-updating vendor harnesses (`claude`, `muse`, `agy`, `opencode`, `codexbar`).

**Project status:** Specifications ([`docs/specs/product.md`](docs/specs/product.md), [`docs/specs/tui.md`](docs/specs/tui.md)) and interactive terminal mockups ([`docs/mockups/index.html`](docs/mockups/index.html)) are ready for review.

---

## Preview the Interactive TUI Design

Open [`docs/mockups/index.html`](docs/mockups/index.html) directly in a browser (no server or network required). Use the controls above the terminal to switch screens, terminal sizes (`160×48`, `120×40`, `100×32`, `80×24`), and themes (`Midnight`, `Daylight`, `Monochrome`), or click the terminal and drive it with the keyboard (`j`/`k`, `Space`, `u`, `g`, `p`, `a`, `f`, `/`, `?`).

See the [mockup implementer guide](docs/mockups/README.md) for the screen index and keyboard walkthrough.

---

## Highlights

- **Zero-prerequisite Debian/Ubuntu bootstrap:** If `mise` is not yet installed on a fresh machine, Outfitter downloads and verifies `~/.local/bin/mise` (SHA-256 checked) automatically.
- **No more disappearing NPM harnesses:** Node-based coding agents (`codex`, `pi`, `gemini-cli`, `copilot-cli`, `openclaw`, `mcporter`, `oracle`, `summarize`) run on a `mise`-managed Node LTS runtime with stable shims, so switching project Node versions in `nvm` never breaks your global AI tools.
- **Curated default catalog + user `catalog.toml`:** Ships with 31 tools across 4 categories (**Coding Harnesses**, **AI TUIs & Personal Tools**, **Terminal & Session Management**, **Agent & CLI Utilities**) and 4 profiles (**Full Desktop**, **Headless Server / VPS**, **Coding Harnesses Only**, **Minimal Core**). Extend or override any entry in `~/.config/outfitter/catalog.toml`.
- **Two-phase Plan & Apply:** Toggle tools visually (`Space`), queue all available updates (`u`), and inspect the exact shell commands and `[sudo]` requirements on the dry-run **Plan & Apply** screen (`a`) before anything on disk is modified.

---

## Project Documents

- [**Documentation Index (`docs/README.md`)**](docs/README.md) — Entry point and map of all project documentation.
- [**Product & Technical Specification (`docs/specs/product.md`)**](docs/specs/product.md) — Architecture, `mise`/`apt`/`vendor`/`source` adapters, built-in catalog, `catalog.toml` schema, CLI contract, and implementation stages.
- [**TUI Experience & Interaction Specification (`docs/specs/tui.md`)**](docs/specs/tui.md) — Layouts (`120×40` split view and `80×24` compact view), checkbox state machine, profile selector, dry-run plan review, live execution view, and color tokens.
- [**Implementation Plan (`docs/implementation-plan.md`)**](docs/implementation-plan.md) — Phased execution plan (Phases 0–6), subagent delegation with `pi` + DeepSeek Flash, review gates, and traceability matrix.
- [**Goal Execution Prompt (`docs/prompts/implement-all-phases.md`)**](docs/prompts/implement-all-phases.md) — Autonomous `/goal` prompt for implementing and reviewing all phases end-to-end.
- [**Interactive Mockups (`docs/mockups/index.html`)**](docs/mockups/index.html) — Offline character-grid browser reference with 8 interactive scenarios.
