# Outfitter — Implementation Plan

- **Status:** Ready for phased execution
- **Source of truth:** [Product & Technical Specification](specs/product.md) and [TUI Experience & Interaction Specification](specs/tui.md)
- **Visual reference:** [Interactive Mockups](mockups/index.html)
- **Execution model:** Each phase is implemented by a spawned **`pi`** harness subagent using `openrouter` model **`deepseek/deepseek-v4.1-flash-1017`** (`https://openrouter.ai/deepseek/deepseek-v4.1-flash-1017`) with **high reasoning effort (`--thinking high`)**, followed by a strict code review and verification gate by the parent lead agent before committing and advancing to the next phase.

---

## 1. Strategy & Repository Conventions

Build Outfitter in 7 bounded, verifiable phases (Phase 0 through Phase 6) so every layer is tested offline before live adapters or shell commands are wired:

1. **Phase 0 — Repository, documentation layout, and tooling foundation:** Buildable Go module matching `../gh-boardwalk`, `../trailmark`, and `../quotary`, with `Makefile`, `.golangci.yml`, `docs-check`, and Cobra CLI root (`outfitter --version`, `outfitter --demo`).
2. **Phase 1 — Domain model, embedded catalog, and `catalog.toml` merging:** Normalized tool/category/profile/action-plan domain types, embedded 31-tool default catalog, user `~/.config/outfitter/catalog.toml` override/addition merger, and import-boundary tests.
3. **Phase 2 — Deterministic demo state & TUI shell (`120×40` split + `80×24` compact):** Semantic theme tokens (`Midnight`, `Daylight`, `Monochrome`), master-detail split layout, compact layout, collapsible categories, and Inspector + Pending Action Plan rendering backed by offline demo fixtures.
4. **Phase 3 — TUI state machine, modals, and golden tests:** Interactive key handling (`j`/`k`, `Space` checkbox cycle, `u` bulk updates, `g` category toggle, `1`–`4`, `Tab`, `f` filter, `/` search, `x` reset), Profile Selector modal (`p`), Dry-Run Plan & Apply review screen (`a`), Live Execution Progress view, and Help overlay (`?`).
5. **Phase 4 — State discovery, version inspector, and `mise` bootstrapper:** Local binary/version detection (`PATH`, `mise ls --json`, `dpkg-query`, `.muse-version`), bounded-concurrency upstream version checker (`mise ls-remote`, GitHub Releases via `gh auth token`, npm registry), snapshot cache (`~/.cache/outfitter/state.json`), and SHA-256 verified `~/.local/bin/mise` bootstrapper.
6. **Phase 5 — Execution adapters (`mise`, `apt`, `vendor`, `source`) & live runner:** Direct-argument process runner with owned process groups, streaming stdout/stderr ring buffer, `tea.ExecProcess` `sudo -v` handoff for `apt`, `codexbar` bundle installer, per-tool failure isolation, and structured run logs (`~/.local/state/outfitter/logs/`).
7. **Phase 6 — Non-interactive CLI subcommands, VHS tapes, and release hardening:** Full CLI suite (`status`, `check`, `profile list/apply`, `install`, `update --all`, `remove`, `catalog export`) with `--dry-run`, `--yes`, `--json`, deterministic exit codes (`0`, `1`, `2`), race-detector pass, and traceability closeout.

### House Conventions (shared with `gh-boardwalk`, `trailmark`, `quotary`)
- Thin `cmd/outfitter/main.go` calling `internal/cli.Execute()`.
- Cobra CLI in `internal/cli/`.
- Bubble Tea v2 (`charm.land/bubbletea/v2`) and Lip Gloss v2 (`charm.land/lipgloss/v2`) in `internal/tui/`.
- Backend-agnostic domain types in `internal/domain/` and catalog loader in `internal/catalog/`.
- Execution and discovery adapters isolated under `internal/engine/` and `internal/adapter/` (`mise`, `apt`, `vendor`, `source`).
- Semantic color tokens in `internal/theme/`.
- Root collaboration files: `README.md`, `AGENTS.md`, `CLAUDE.md`, `CONTRIBUTING.md`, `LICENSE`, `Makefile`, `.golangci.yml`. All specs, plans, mockups, and review records live under `docs/` (`docs/specs/product.md`, `docs/specs/tui.md`, `docs/implementation-plan.md`, `docs/evidence/reviews/`).

---

## 2. Subagent Execution & Lead Review Protocol

For each phase `N`:
1. **Spawn `pi` Subagent:** The lead agent invokes `pi` in non-interactive print mode with the exact OpenRouter model `deepseek/deepseek-v4.1-flash-1017` and high reasoning effort (`--thinking high`):
   ```bash
   pi --print \
      --provider openrouter \
      --model deepseek/deepseek-v4.1-flash-1017 \
      --thinking high \
      --approve \
      "$(cat /tmp/outfitter-phase-N-prompt.md)"
   ```
2. **Independent Code Review by Lead Agent:** When the `pi` subagent finishes, the lead agent:
   - Inspects `git status --porcelain` and `git diff` across all modified and untracked files.
   - Runs `make check` (`go vet ./...`, `golangci-lint`, `docs-check`, `go test ./...`) and `go test -race ./...`.
   - Audits every file against the Phase `N` exit gate, `docs/specs/product.md`, and `docs/specs/tui.md` (checking for spec drift, swallowed errors, shell-injection risks, raw ANSI colors outside `internal/theme`, or weakened tests).
   - If any `P0` or `P1` issue is found, either sends a targeted fix prompt to `pi` or repairs and re-verifies before recording the review in `docs/evidence/reviews/phase-<N>.md` and committing the phase.

---

## 3. Phases, Deliverables, and Exit Gates

### Phase 0 — Repository, Documentation Layout, and Tooling Foundation

**Goal:** A clean, buildable Go module matching the sibling repositories, with documentation organized under `docs/` and automated checks wired up.

**Tasks:**
- Initialize git repository (if not initialized) and `go.mod` (`github.com/juanrubio/outfitter`) with the Go version and Charm v2 dependencies (`charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `github.com/spf13/cobra`, `github.com/pelletier/go-toml/v2`).
- Move root `SPEC.md` → `docs/specs/product.md`, `UX.md` → `docs/specs/tui.md`, and `IMPLEMENTATION_PLAN.md` → `docs/implementation-plan.md`. Create `docs/README.md` indexing all docs and update all relative links in `README.md` and `docs/mockups/README.md`.
- Create `internal/tools/docscheck` (verifies all relative Markdown links resolve and ensures no duplicate `SPEC.md`/`UX.md`/`IMPLEMENTATION_PLAN.md` remain at the repo root).
- Add root files: `Makefile` (`all`, `build`, `test`, `race`, `vet`, `lint`, `docs-check`, `check`, `run`, `clean`), `.golangci.yml`, `.gitignore`, `.editorconfig`, `LICENSE` (MIT), `AGENTS.md`, `CLAUDE.md`, and `CONTRIBUTING.md`.
- Add `cmd/outfitter/main.go` and `internal/cli/root.go` with `--version`, `--demo`, `--catalog`, and a stub TUI entrypoint that cleanly initializes and exits when given an immediate quit message in tests.

**Exit Gate:**
- `make check` (`vet`, `lint`, `docs-check`, `test`) and `make build` pass with zero warnings.
- `./outfitter --version` prints version metadata cleanly.
- All Markdown links resolve via `make docs-check`.

---

### Phase 1 — Domain Model, Embedded Catalog, and `catalog.toml` Merger

**Goal:** A rock-solid, backend-agnostic domain model (`internal/domain`) and catalog loader (`internal/catalog`) covering all 31 built-in tools, 4 categories, 4 preset profiles, and user `catalog.toml` overrides.

**Tasks:**
- Implement `internal/domain/`:
  - `Tool`: `ID`, `Name`, `Binary`, `CategoryID`, `Backend` (`mise:aqua`, `mise:ubi`, `mise:github`, `mise:npm`, `uv`, `apt`, `vendor`, `source`), `Package`, `Sudo`, `Profiles`, `Description`, `InstallCmd`, `UpdateCmd`, `RemoveCmd`, `SourceDir`, `VersionArgs`, `VersionRegex`, `Disabled`.
  - `ToolState`: `InstalledVersion`, `LatestVersion`, `BinaryPath`, `Provenance` (`mise`, `apt`, `vendor`, `uv`, `source`, `external`, `none`), `CheckStatus` (`unknown`, `checking`, `ready`, `error`), `LastError`.
  - `DesiredAction`: `ActionNone`, `ActionInstall`, `ActionUpdate`, `ActionRemove`, plus state machine transitions (`Toggle(installed, outdated)` matching `docs/specs/tui.md` §4.1: `[ ]` ↔ `[+]`, `[!]` → `[↑]` → `[-]` → `[!]`, `[✓]` ↔ `[-]`).
  - `ActionPlan`: computes ordered steps (0: `mise` bootstrap if needed, 1: `apt` installs/updates, 2: `mise`/`uv`/`vendor`/`source` installs, 3: updates, 4: removals), `NeedsSudo()`, and summary counts (`+install`, `↑update`, `-remove`).
- Implement `internal/catalog/`:
  - Embed `default_catalog.toml` containing all 31 tools, 4 categories (`harnesses`, `ai-tuis`, `terminal`, `utilities`), and 4 preset profiles (`desktop`, `server`, `harnesses`, `minimal`) matching `docs/specs/product.md` §4 and `docs/mockups/fixtures.js`.
  - Implement `Load(userCatalogPath string)`: merges user `catalog.toml` by tool `id` (overriding specified fields, supporting `disabled = true`, appending custom tools, and merging custom `[profiles.<id>]`).
  - Implement `ExportDefault() string` for `outfitter catalog export`.
- Write comprehensive unit tests in `internal/domain/domain_test.go` and `internal/catalog/catalog_test.go` (including import-boundary tests verifying `internal/domain` and `internal/catalog` never import `internal/tui` or `internal/adapter`).

**Exit Gate:**
- `make check` and `make race` pass.
- Embedded default catalog loads all 31 tools and 4 profiles deterministically.
- User `catalog.toml` override tests verify field overrides (e.g., switching `quotary` to `backend = "source"`), adding custom tools (`ttyd`), disabling built-in tools, and rejecting invalid backend types or duplicate malformed IDs.

---

### Phase 2 — Theme System, Demo Backend, and TUI Layouts (`120×40` Split & `80×24` Compact)

**Goal:** Visual parity with `docs/mockups/index.html` using Bubble Tea v2 and Lip Gloss v2, running 100% offline in `--demo` mode across `160×48`, `120×40`, `100×32`, and `80×24` terminals.

**Tasks:**
- Implement `internal/theme/`:
  - Semantic tokens matching `docs/specs/tui.md` §6 (`Canvas`, `Surface`, `Border`, `Text`, `Muted`, `Accent`, `Good`, `Low`, `Error`, `Stale`) across `Midnight` (dark), `Daylight` (light), and `Monochrome` (`NO_COLOR` safe).
  - Zero raw hex colors allowed inside `internal/tui/`.
- Implement `internal/demo/`:
  - Deterministic demo state provider mirroring `docs/mockups/fixtures.js` (same installed/outdated/uninstalled versions, paths, provenance, and pre-queued demo items) with zero subprocess, filesystem, or network calls.
- Implement `internal/tui/` core layout and rendering:
  - Header bar: brand, active profile name, `mise` readiness status, installed/outdated summary, and live `Plan: +N ↑N -N` counter.
  - Filter & action bar: active filter (`All`, `Outdated`, `Pending`, `Installed`, `Not installed`), quick hints (`[p] Profile`, `[u] Queue updates`, `[a] Apply plan`), and notice/platform status.
  - Left catalog list: collapsible category headers (`▾ 1 · CODING HARNESSES (6/8 · 2 outdated)`), selection cursor (`>`), checkbox marker (`[✓]`, `[!]`, `[↑]`, `[+]`, `[-]`, `[ ]`), padded tool name, version diff (`0.163.0 → 0.165.0`), and right-aligned backend badge.
  - Right pane (`w >= 100`): **Inspector** box (tool name, backend + `[sudo]` badge, wrapped description, State/Installed/Latest/Binary path/Package/Provenance/Profiles key-value table, and exact shell command) + **Pending Action Plan** box at bottom right.
  - Compact layout (`80 <= w < 100`): full-width catalog list without clipping; pressing `Enter` opens the full-width Inspector card.
  - Footer bar with context-sensitive keyboard shortcuts and scroll position indicator.

**Exit Gate:**
- `outfitter --demo` renders cleanly at `160×48`, `120×40`, `100×32`, and `80×24` without line wrap corruption or panel overflow.
- Monochrome (`NO_COLOR=1`) preserves all state distinctions via `[✓]`, `[!]`, `[↑]`, `[+]`, `[-]`, `[ ]`, and `>`.
- Demo isolation tests prove zero external command or network execution in `--demo` mode.

---

### Phase 3 — TUI State Machine, Modals (Profiles, Plan & Apply, Live Progress, Help), and Golden Tests

**Goal:** Complete interactive keyboard workflow in `internal/tui/` matching `docs/specs/tui.md` §4–§5, verified by state-transition and golden snapshot tests.

**Tasks:**
- Implement full keyboard state machine in `internal/tui/`:
  - Navigation: `↑`/`↓`/`j`/`k`, `PgUp`/`PgDn`, `1`/`2`/`3`/`4` category jump, `Tab` category collapse/expand.
  - Selection & Plan mutations: `Space` toggle selected tool, `u` queue all updates, `U` queue single update, `g` toggle focused category, `x` reset pending plan.
  - Filtering & Search: `f` cycle filter (`All` → `Outdated` → `Pending` → `Installed` → `Not Installed`), `/` incremental search bar (capturing printable keys, `Backspace`, `Esc` to clear, `Enter` to commit).
  - Quit safety: `q` quits immediately if plan is clean, or prompts to confirm discarding unapplied plan changes (`Ctrl+C` always exits cleanly).
- Implement the 4 overlay views/modals:
  1. **Profile Selector Modal (`p`):** Lists `Full Desktop`, `Headless Server / VPS`, `Coding Harnesses Only`, `Minimal Core` (plus any custom profiles from `catalog.toml`), showing tool counts and descriptions; `Enter` queues missing profile tools into the plan.
  2. **Dry-Run Plan & Apply Screen (`a`):** Displays grouped execution plan (`0. Bootstrap Prerequisite` if `mise` missing, `Installations (+)`, `Updates (↑)`, `Removals (-)`) with target versions, `[sudo]` badges, and exact `$ <cmd>` strings. `Enter`/`y` starts execution; `Esc`/`n` cancels.
  3. **Live Execution Progress View:** Overall progress bar, per-step status (`✓`, `⠋`, `·`, `✗`) with elapsed durations, and a 4-line live output tail for the currently running command. Supports simulated execution in `--demo` mode and real channel updates in live mode.
  4. **Keyboard Help Modal (`?`):** Scrollable shortcut reference.
- Add golden render tests and interactive state transition tests (`internal/tui/tui_test.go`) covering `120×40` and `80×24` across all views.

**Exit Gate:**
- All keyboard shortcuts in `docs/specs/tui.md` §5 are tested and functional.
- Golden snapshot tests verify `Catalog`, `Outdated`, `Profile Selector`, `Plan & Apply`, `Live Execution Progress`, `Missing mise banner`, `Compact 80×24`, and `Help` views.
- `make check` and `make race` pass.

---

### Phase 4 — State Discovery, Upstream Version Inspector, and `mise` Bootstrapper

**Goal:** Fast, accurate local installation discovery, snapshot caching, upstream version checking, and SHA-256 verified `~/.local/bin/mise` bootstrapping.

**Tasks:**
- Implement `internal/engine/ bootstrap.go`:
  - Detect `mise` on `PATH` or `~/.local/bin/mise`.
  - Implement verified `BootstrapMise(ctx)` downloading the official `mise` release archive for `linux-x64` / `linux-arm64`, verifying its SHA-256 digest against `SHASUMS256.txt`, installing `~/.local/bin/mise` (`0755`), and ensuring `~/.local/bin` and `~/.local/share/mise/shims` are configured for child invocations.
- Implement `internal/engine/inspector.go`:
  - Fast local scan (`InspectLocal`): checks `exec.LookPath`, symlink targets, `mise ls --installed --json` (when `mise` is available), `~/.local/bin/.muse-version` for `muse`, `dpkg-query -W` for `apt` packages, and `<binary> --version` regex extraction to determine `InstalledVersion`, `BinaryPath`, and `Provenance` (`mise`, `apt`, `vendor`, `uv`, `source`, `external`).
  - Concurrent upstream scan (`InspectUpstream` with worker limit = 4):
    - `mise:aqua`, `mise:ubi`, `mise:github`, `mise:npm`: queries `mise latest <pkg>` or GitHub Releases / NPM registry JSON API.
    - Borrow `gh auth token` when `gh` is present and authenticated (`github_token_from_gh = true`) so GitHub release checks never hit anonymous rate limits.
    - `apt`: parses `apt-cache policy <pkg>` candidate version without requiring `sudo`.
    - `vendor`: checks `.muse-release-info.json` / vendor version endpoints or `mise` registry metadata where available.
- Implement `internal/engine/cache.go`:
  - Read/write `~/.cache/outfitter/state.json` atomically so TUI startup paints in `<50 ms` from cached state before background refresh completes.
- Write offline unit tests using fake command runners and HTTP test servers (`httptest.Server`) to test checksum verification, mismatch rejection, version parsing, and cache persistence.

**Exit Gate:**
- Inspector accurately identifies installed versions and provenance across synthetic fixtures for all backend types (`mise:aqua`, `mise:ubi`, `mise:npm`, `uv`, `apt`, `vendor`, `source`).
- `BootstrapMise` test verifies SHA-256 validation and fails closed on corrupted archives or checksum mismatches.
- `make check` and `make race` pass with zero real network calls in `go test`.

---

### Phase 5 — Execution Adapters (`mise`, `apt`, `vendor`, `source`) & Live Runner

**Goal:** Safe, streaming execution of queued installs, updates, and removals with per-tool failure isolation and structured logging.

**Tasks:**
- Implement `internal/adapter/` for each backend:
  - `mise.go`: ensures Node LTS (`node@lts`) and `uv` (`uv@latest`) are available in `mise` when installing `mise:npm` or `uv` tools, then runs `mise use --global <spec>@latest`, `mise upgrade <spec>`, or `mise unuse --global <spec>`.
  - `apt.go`: builds `sudo apt-get install -y <pkg>`, `--only-upgrade`, or `remove -y`. Includes a pre-flight `VerifySudo` helper wired to `tea.ExecProcess("sudo", "-v")` in the TUI when `plan.NeedsSudo()` is true.
  - `vendor.go`: handles `claude`, `muse`, `agy`, `opencode`, and `codexbar` (including extracting `codexbar`'s executable + companion resource bundle into `~/.local/share/outfitter/codexbar/` and symlinking `~/.local/bin/codexbar`).
  - `source.go`: builds local Go repositories (such as `~/repos/quotary`) into `~/.local/bin/<binary>` cleanly.
- Implement `internal/engine/runner.go`:
  - Executes an `ActionPlan` step-by-step, streaming stdout/stderr lines to a channel consumed by the TUI's **Live Execution Progress** view (or CLI output).
  - Uses context cancellation and process-group cleanup so pressing `Esc`/`Ctrl+C` leaves no orphan processes.
  - Writes structured execution logs to `~/.local/state/outfitter/logs/run-<timestamp>.log`.
  - Continues executing independent tools if one tool fails, recording exact exit codes and error summaries.

**Exit Gate:**
- Adapter contract tests verify exact generated commands, environment variables (`PATH` including `~/.local/bin` and `~/.local/share/mise/shims`), `codexbar` bundle layout, failure isolation, and process-group cancellation.
- `make check` and `make race` pass.

---

### Phase 6 — Non-Interactive CLI Subcommands, VHS Tapes, and Release Hardening

**Goal:** Full scriptable CLI parity (`status`, `check`, `profile`, `install`, `update`, `remove`, `catalog`) with `--dry-run`, `--yes`, and `--json`, plus final documentation and verification closeout.

**Tasks:**
- Wire all CLI subcommands in `internal/cli/` per `docs/specs/product.md` §7:
  - `outfitter` (default interactive TUI; `--demo` runs offline demo TUI).
  - `outfitter status [--json]`
  - `outfitter check [--json]`
  - `outfitter profile list [--json]` and `outfitter profile apply <name> [--dry-run] [--yes] [--json]`
  - `outfitter install <tool-id>... [--dry-run] [--yes] [--json]`
  - `outfitter update [<tool-id>...] [--all] [--dry-run] [--yes] [--json]`
  - `outfitter remove <tool-id>... [--dry-run] [--yes] [--json]`
  - `outfitter catalog export`
- Enforce safety contract: any mutating CLI command without `--yes` in a non-TTY (piped) environment prints the dry-run plan and exits with code `2`. Partial or complete tool failures exit with code `1`.
- Add `tapes/showcase.tape` for VHS recording against `outfitter --demo`.
- Complete `docs/evidence/implementation-progress.md` and `docs/evidence/traceability.md` mapping every requirement in `docs/specs/product.md` and `docs/specs/tui.md` to passing tests.

**Exit Gate:**
- CLI integration tests verify `--dry-run`, `--yes`, `--json` output schemas, and exit codes `0`, `1`, `2` across all subcommands.
- `make all` (`vet`, `lint`, `docs-check`, `test`, `build`) and `make race` pass cleanly.
- Working tree is clean with all phase commits and review records under `docs/evidence/reviews/phase-0.md` … `phase-6.md`.

---

## 4. Traceability Matrix

| Requirement | Phase | Verification Artifact |
| :--- | :--- | :--- |
| Buildable Go module, Charm v2 stack, `docs/` layout, `make docs-check` | P0 | `Makefile`, `internal/tools/docscheck`, CI workflow |
| 31 built-in tools across 4 categories & 4 profiles (`desktop`, `server`, `harnesses`, `minimal`) | P1 | `internal/catalog/catalog_test.go` |
| User `~/.config/outfitter/catalog.toml` merging, overrides, custom tools & profiles | P1 | `internal/catalog/merge_test.go` |
| Checkbox state machine (`[✓]`, `[!]`, `[↑]`, `[+]`, `[-]`, `[ ]`) & `ActionPlan` ordering | P1, P3 | `internal/domain/domain_test.go`, `internal/tui/tui_test.go` |
| `120×40` split master-detail layout & `80×24` compact layout | P2, P3 | `internal/tui/tui_test.go` golden snapshots |
| `Midnight`, `Daylight`, and `Monochrome` (`NO_COLOR`) semantic themes | P2 | `internal/theme/theme_test.go` |
| Profile selector (`p`), Dry-Run Plan & Apply (`a`), Live Progress, Help (`?`) | P3 | `internal/tui/tui_test.go` modal interaction tests |
| Verified `mise` auto-bootstrapper (`~/.local/bin/mise` + SHA-256 check) | P4 | `internal/engine/bootstrap_test.go` |
| Local & upstream version inspector with `gh auth token` support & snapshot cache | P4 | `internal/engine/inspector_test.go`, `cache_test.go` |
| `mise` (`aqua`/`ubi`/`npm`/`uv`), `apt` (`sudo`), `vendor` (`codexbar` bundle, `muse`), `source` adapters | P5 | `internal/adapter/adapter_test.go`, `internal/engine/runner_test.go` |
| CLI subcommands (`status`, `check`, `profile`, `install`, `update`, `remove`, `catalog`) + `--json`/`--dry-run`/`--yes` | P6 | `internal/cli/cli_test.go` |
