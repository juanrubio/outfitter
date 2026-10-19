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
| 5 | Execution Adapters and Live Runner | Not started | — |
| 6 | Non-Interactive CLI, VHS Tapes, and Release Hardening | Not started | — |

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

## Phase Status Legend

- **Not started:** No implementation work has begun.
- **In progress:** Work is underway but the exit gate has not passed.
- **Complete:** The exit gate passed and a review record exists.
