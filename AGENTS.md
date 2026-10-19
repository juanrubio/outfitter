# Outfitter contributor and agent guide

Outfitter is a keyboard-first Go TUI and scriptable CLI for bootstrapping,
selecting, and bulk-updating AI development harnesses and terminal tools on
Debian and Ubuntu Linux. Start with the documentation index at
[docs/README.md](docs/README.md).

## Documentation

- Product behavior: [docs/specs/product.md](docs/specs/product.md)
- Visual and interaction behavior: [docs/specs/tui.md](docs/specs/tui.md)
- Delivery sequence: [docs/implementation-plan.md](docs/implementation-plan.md)
- Implementation progress: [docs/evidence/implementation-progress.md](docs/evidence/implementation-progress.md)
- Visual reference: [docs/mockups/README.md](docs/mockups/README.md)

## Commands

```sh
make build
make test
make race
make check
make run ARGS="--demo"
```

`make check` runs vet, lint, docs-check, and tests. CI runs `make check` and
`make race`.

## Conventions

- Thin `cmd/outfitter/main.go`; Cobra commands in `internal/cli`.
- Bubble Tea v2 and Lip Gloss v2 in `internal/tui`.
- Backend-agnostic domain values in `internal/domain`; catalog loading in
  `internal/catalog`.
- Execution and discovery adapters isolated under `internal/engine` and
  `internal/adapter`.
- Semantic color tokens live in `internal/theme`; no raw hex colors in
  `internal/tui`.
- Demo mode is fully offline and uses fictional data only. Tests must not make
  real network calls or depend on the host environment.
- Every document except the root collaboration files lives under `docs/`.
  Run `make docs-check` after moving or editing documentation.

## Phase reviews

Implementation phases are reviewed before they are committed. Review records
live under `docs/evidence/reviews/`, and progress is tracked in
[docs/evidence/implementation-progress.md](docs/evidence/implementation-progress.md).
