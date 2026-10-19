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
| 3 | TUI State Machine, Modals, and Golden Tests | Not started | — |
| 4 | State Discovery, Version Inspector, and `mise` Bootstrapper | Not started | — |
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

## Phase Status Legend

- **Not started:** No implementation work has begun.
- **In progress:** Work is underway but the exit gate has not passed.
- **Complete:** The exit gate passed and a review record exists.
