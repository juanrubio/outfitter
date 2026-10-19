# Implementation Progress

Track the implementation status of Outfitter against the
[Implementation Plan](../implementation-plan.md). Each phase is complete only
after its exit gate passes and a review record is written under
`docs/evidence/reviews/`.

| Phase | Title | Status | Verification |
| :--- | :--- | :--- | :--- |
| 0 | Repository, Documentation Layout, and Tooling Foundation | Complete | `make check`, `make build` |
| 1 | Domain Model, Embedded Catalog, and `catalog.toml` Merger | Complete | `make check`, `make race`, `make build` |
| 2 | Theme System, Demo Backend, and TUI Layouts | Complete | `make check`, `make race`, `make build` |
| 3 | TUI State Machine, Modals, and Golden Tests | Complete | `make check`, `make race`, `make build` |
| 4 | State Discovery, Version Inspector, and `mise` Bootstrapper | Complete | `make check`, `make race`, `make build` |
| 5 | Execution Adapters and Live Runner | Complete | `make check`, `make race`, `make build` |
| 6 | Non-Interactive CLI, VHS Tapes, and Release Hardening | Complete | `make check`, `make race`, `make all`, CLI integration tests |

## Phase 0 Evidence

- Go module `github.com/juanrubio/outfitter` with the Charm v2, Cobra, TOML,
  and `golang.org/x/term` dependencies.
- Root specifications moved to `docs/specs/product.md` and
  `docs/specs/tui.md`; the implementation plan moved to
  `docs/implementation-plan.md`.
- Documentation index, docs-check tool, house files, CI workflow, and a stub
  Cobra/TUI entrypoint are in place.
- Verification: `make check`, `make race`, and `make build` exit 0.

## Phase 1 Evidence

- `internal/domain` defines backend and provenance identifiers, categories,
  profiles, settings, tools, observed tool state, the `Space` checkbox state
  machine and marker semantics, command synthesis, and ordered `ActionPlan`
  phases with `mise` bootstrap detection.
- `internal/catalog/default_catalog.toml` embeds all 31 built-in tools across
  the `harnesses`, `ai-tuis`, `terminal`, and `utilities` categories plus the
  `desktop` (31), `server` (21), `harnesses` (8), and `minimal` (9) profiles.
- `internal/catalog` merges user `catalog.toml` overrides by tool ID, supports
  custom tools and profiles (`[profiles.<id>]` and `[[profiles]]`), applies
  `disabled = true`, regenerates commands for backend/source changes, and
  validates active tools.
- Tests cover action transitions, plan ordering and sudo/bootstrap rules,
  default catalog contents, the product specification's user-catalog example,
  disabling tools, settings and profile merging, validation failures, and
  package import boundaries.
- Verification: `make check`, `make race`, and `make build` exit 0.

## Phase 2 Evidence

- `internal/theme` defines the `Midnight`, `Daylight`, and `Monochrome`
  palettes with the ten semantic tokens, `NO_COLOR` resolution, Lip Gloss v2
  styles, and `ByToken` lookup for `domain.Marker` tokens.
- `internal/demo` loads the 31-tool catalog with the `quotary` source override
  and the deterministic fictional fixture state (26 installed, 5 outdated, and
  `gemini-cli`/`quotary` pre-queued for install) without filesystem discovery,
  subprocess execution, or network access.
- `internal/tui` provides the exported `NewModel` model, header and action
  bars, scrollable collapsible catalog, wide `99+`-column Inspector + Pending
  Action Plan split, compact `80×24` full-width list with `Enter`/`Esc`
  inspector navigation, and exact-width rendering across `160×48`, `120×40`,
  `100×32`, and `80×24`.
- Tests verify palette values, monochrome grayscale tokens, `NO_COLOR` mode
  resolution, fixture counts and versions, offline demo imports, screen
  dimensions, all marker glyphs across themes, compact detail navigation, and
  the absence of raw hex color literals under `internal/tui/`.
- Verification: `make check`, `make race`, and `make build` exit 0.

## Phase 3 Evidence

- `internal/tui` implements the complete keyboard state machine: `j`/`k`,
  arrow keys, `PgUp`/`PgDn`, `1`–`4` category jumps with expansion, `Tab`
  collapse/expand, `Space` action cycling, `u`/`U` update queuing, `g`
  category toggling, `x` plan reset, `f` five-mode filtering, `/` incremental
  search that captures printable keys without firing global shortcuts, `b`
  bootstrap simulation, `r` refresh notices, `c` catalog-source notice, and
  plan-safe `q` quit confirmation (`Ctrl+C` always exits).
- Overlay screens include the Profile Selector (`p`), Dry-Run Plan & Apply
  (`a` or `Enter` on wide layouts) with bootstrap, sudo, command, and summary
  sections, Live Execution Progress with deterministic demo advancement via
  `StepProgressMsg`/`AdvanceExecution` for Phase 5 reuse, and the Keyboard Help
  reference (`?`).
- `internal/tui/tui_test.go` covers every keybinding and state transition,
  all five filters, search key capture and clearing, server/minimal profile
  application, plan/apply transitions, missing `mise` bootstrap, help, quit
  safety, and exact screen geometry for `catalog`, `profiles`, `plan`,
  `applying`, `help`, and `detail` at `120×40` and `80×24`.
- Verification: `make check`, `make race`, and `make build` exit 0.

## Phase 4 Evidence

- `internal/engine/bootstrap.go` implements `FindMise`, `MiseEnv`, and a
  fail-closed `BootstrapMise`: releases are downloaded from an overridable
  `BaseURL`, matched against `SHASUMS256.txt`, verified with SHA-256, extracted
  with zip-slip and decompression-bomb guards, installed atomically as
  `<install-dir>/mise` with `0755` permissions, and optionally smoke-tested
  through `mise --version`.
- `internal/engine/inspector.go` performs a local scan across
  `mise ls --installed --json` (map-of-slices, map-of-objects, and slice
  formats), `PATH` and `~/.local/bin` fallbacks, symlink target provenance
  (`mise`, `uv`, outfitter vendor bundles), `dpkg-query`, `.muse-version`, and
  per-tool version regexes.
  It normalizes `v` prefixes, Debian epochs, and Debian revisions so installed
  and upstream versions compare consistently.
- `InspectUpstream` uses a bounded worker pool (default four), borrows
  `gh auth token` once for GitHub API requests, prefers `mise latest`, and
  falls back to the npm registry, GitHub Releases, PyPI, and `apt-cache policy`
  without `sudo`. Vendor and source tools preserve known state when no upstream
  source applies rather than reporting false errors, and `onUpdate` is invoked
  from a single goroutine as each tool completes.
- `internal/engine/cache.go` persists `~/.cache/outfitter/state.json` through a
  temporary file in the destination directory followed by an atomic rename,
  with `0755` parent directories and `0644` snapshots. Missing and corrupt
  caches return explicit errors alongside an empty snapshot.
- Offline tests in `internal/engine/*_test.go` cover successful SHA-256 verified
  installs, checksum mismatches, missing checksum entries, HTTP 404/500
  responses, unsupported platforms, corrupt archives, path traversal, all
  backend provenance paths, version normalization, slice-format `mise` output,
  GitHub token injection, npm/PyPI/apt fallbacks, `mise latest` parsing,
  vendor/source preservation, worker-pool concurrency, update callbacks, and
  cache round-trips. No test makes a real network request or depends on host
  binaries.
- Verification: `make check`, `make race`, and `make build` exit 0.

## Phase 5 Evidence

- `internal/adapter` translates `domain.PlanStep` values into ordered
  `CommandSpec` invocations. It covers `mise:aqua`, `mise:ubi`, `mise:github`,
  `mise:npm`, `uv`, `apt`, `vendor`, and `source` backends for install, update,
  and remove actions.
- The mise adapter resolves `mise` from `PATH`, `~/.local/bin/mise`, or the
  `mise` command name; prepends `mise use --global node@lts` once per run before
  npm tool changes; prepends `mise use --global uv@latest` once per run before
  uv tool changes; and emits `mise use`, `mise upgrade`, `mise unuse`, and
  `mise x uv -- uv tool ...` command arguments without a shell.
- The apt adapter builds `sudo apt-get install`, `--only-upgrade`, and `remove`
  commands plus `VerifySudoCommand()` for the TUI `tea.ExecProcess` handoff.
- The vendor adapter runs configured vendor commands through
  `bash -o pipefail -c` and implements the native CodexBar bundle installer:
  release archive discovery (overridable `ArchiveURL`, GitHub API base URL, and
  HTTP client), tar.gz extraction of `CodexBarCLI` and `*.bundle` resources into
  `<home>/.local/share/outfitter/codexbar/`, `0755` executable permissions, and
  the `<home>/.local/bin/codexbar` symlink. Removal deletes both paths.
- The source adapter expands `~/` source and install paths, runs `go build` in
  the repository with output at `<home>/.local/bin/<binary>` (or a custom
  `build_cmd`), and removes the installed binary on uninstall.
- `internal/engine/runner.go` executes plans with a streaming `StepEvent` API,
  `StepResult`/`PlanResult` summaries, per-tool failure isolation, fail-fast
  dependency errors after a failed mise bootstrap, and independent apt/vendor/
  source execution.
- The default `ProcessExecutor` uses `MiseEnv`, per-command environments, direct
  argv execution (no shell), `Setpgid` process groups, and a `cmd.Cancel` group
  kill so cancellation terminates children and grandchildren. Combined stdout
  and stderr are streamed line by line and written to
  `<home>/.local/state/outfitter/logs/run-<YYYYMMDD-HHMMSS>.log` with `0644`
  permissions, timestamps, exact commands, durations, exit codes, and summary.
- `internal/tui` separates the deterministic demo timer from live execution:
  `Options.OnRefresh`/`OnExecute` accept injected engine callbacks, live mode
  defaults to the real `engine.Inspector` and `engine.Runner`, streams runner
  events into the existing progress view, verifies sudo through
  `tea.ExecProcess`, exposes the log path with `l`, and cancels the runner on
  quit.
- Offline tests verify exact commands and prerequisites for every backend and
  action, CodexBar archive extraction/symlink layout and GitHub asset
  resolution with `httptest.Server`, source path expansion, mise environment
  `PATH`, streaming stdout/stderr events, per-tool failure isolation and
  structured logs, mise bootstrap dependency failures, and process-group
  cancellation of a child and grandchild. No test uses the public network or
  depends on host-installed tools.
- Verification: `make check`, `make race`, and `make build` exit 0.

## Phase 6 Evidence

- `internal/cli` implements the complete non-interactive command tree: root
  TUI launch, `status`, `check`, `profile list`/`apply`, `install`, `update`,
  `remove`, `catalog export`, and the hidden `internal-bootstrap mise` helper.
  `--demo`, `--catalog`, `--json`, `--dry-run`, and `--yes` are wired through
  every relevant command.
- `internal/cli/deps.go` provides injectable `Deps` with `NewRootCmdWithDeps`
  for fully offline tests. Defaults use the live inspector, runner, TUI, and
  `mise` finder; demo execution uses a deterministic no-op runner.
- Deterministic exit codes follow the product contract via `ExitError` and
  `ExitCode`: `0` success, `1` execution failures, `2` usage errors,
  non-interactive mutation refusal, and interactive confirmation refusal.
  `cmd/outfitter/main.go` prints the error and exits with `cli.ExitCode(err)`.
- `status` merges cached upstream versions from
  `~/.cache/outfitter/state.json`; `check` refreshes local and upstream state,
  writes the cache outside demo mode, and supports human/JSON summaries.
- Mutating commands print the exact plan and `$ <command>` lines, stream
  progress for confirmed runs, emit structured JSON results (`plan`, `steps`,
  `succeeded`, `failed`, `log_path`), and isolate partial failures behind exit
  code `1`.
- `tapes/showcase.tape` records the deterministic `./outfitter --demo` tour at
  1440×900 with a dark theme, including navigation, category jumps, profile
  cycling (`]`), update queuing (`u`), Plan & Apply (`Enter`), simulated
  execution, help (`?`), theme cycling (`t`), and quit (`q`).
- `docs/evidence/traceability.md` maps every product and TUI requirement to
  implementation files and passing tests. `internal/cli/cli_test.go` covers
  human/JSON output, dry runs, confirmation refusal, confirmed success and
  failure, unknown tools/profiles, cache merging, demo isolation, and catalog
  export round-trips.
- Verification: `make check`, `make race`, and `make all` exit 0.

## Phase Status Legend

- **Not started:** No implementation work has begun.
- **In progress:** Work is underway but the exit gate has not passed.
- **Complete:** The exit gate passed and a review record exists.
