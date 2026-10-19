# Implementation Progress

Track the implementation status of Outfitter against the
[Implementation Plan](../implementation-plan.md). Each phase is complete only
after its exit gate passes and a review record is written under
`docs/evidence/reviews/`.

| Phase | Title | Status | Verification |
| :--- | :--- | :--- | :--- |
| 0 | Repository, Documentation Layout, and Tooling Foundation | Complete | `make check`, `make build` |
| 1 | Domain Model, Embedded Catalog, and `catalog.toml` Merger | Not started | — |
| 2 | Theme System, Demo Backend, and TUI Layouts | Not started | — |
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

## Phase Status Legend

- **Not started:** No implementation work has begun.
- **In progress:** Work is underway but the exit gate has not passed.
- **Complete:** The exit gate passed and a review record exists.
