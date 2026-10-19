# Outfitter Documentation

This is the entry point for all Outfitter project documentation. Product
behavior, interaction design, delivery plans, mockups, and evidence records all
live under `docs/`.

## Specifications

- [Product & Technical Specification](specs/product.md) — Architecture,
  `mise`/`apt`/`vendor`/`source` adapters, built-in catalog, `catalog.toml`
  schema, CLI contract, and implementation stages.
- [TUI Experience & Interaction Specification](specs/tui.md) — Layouts
  (`120×40` split view and `80×24` compact view), checkbox state machine,
  profile selector, dry-run plan review, live execution view, and color tokens.

## Delivery

- [Implementation Plan](implementation-plan.md) — Phased execution plan
  (Phases 0–6), subagent delegation, review gates, and traceability matrix.
- [Goal Execution Prompt](prompts/implement-all-phases.md) — Autonomous
  end-to-end implementation and review prompt.
- [Implementation Progress](evidence/implementation-progress.md) — Phase status
  and verification evidence.
- [Requirements Traceability](evidence/traceability.md) — Product and TUI
  requirements mapped to implementation files and offline tests.

## Visual Reference

- [Mockup Implementer Guide](mockups/README.md) — Screen index and keyboard
  walkthrough.
- [Interactive Mockups](mockups/index.html) — Offline character-grid browser
  reference with 8 interactive scenarios.

## Conventions

Root collaboration files (`README.md`, `AGENTS.md`, `CLAUDE.md`,
`CONTRIBUTING.md`, `LICENSE`, `Makefile`, `.golangci.yml`) stay at the
repository root. Every other document lives under `docs/`, and
`make docs-check` verifies that all relative Markdown links resolve.
