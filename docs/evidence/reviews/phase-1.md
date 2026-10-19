# Phase 1 Review — Domain Model, Embedded Catalog, and `catalog.toml` Merger

- **Date:** 2026-10-19
- **Subagent:** `pi` (`openrouter` / `deepseek/deepseek-v4.1-flash-1017`, `--thinking high`)
- **Reviewer:** Lead Agent (Antigravity)
- **Verdict:** `PASS`

## 1. Summary of Changes Reviewed

- **`internal/domain` (`domain.go`, `plan.go`, `domain_test.go`):**
  - Implemented `BackendType` (`mise:aqua`, `mise:ubi`, `mise:github`, `mise:npm`, `uv`, `apt`, `vendor`, `source`), `IsValid()`, and `RequiresMise()`.
  - Implemented `Provenance`, `CheckStatus`, `Category`, `Profile`, `Settings`, `Tool`, and `ToolState` (`IsInstalled()`, `IsOutdated()`).
  - Implemented `DesiredAction` state machine (`NextAction` and `Marker` matching `docs/specs/tui.md` §4.1).
  - Implemented `BuildActionPlan` with deterministic ordering (`BootstrapStep` when `!miseInstalled` and any `mise`/`uv` step is queued, `apt` before user-space steps within `Installations`, `Updates`, and `Removals`), `NeedsSudo()`, `TotalSteps()`, and `Counts()`.
- **`internal/catalog` (`default_catalog.toml`, `catalog.go`, `catalog_test.go`, `boundary_test.go`):**
  - Embedded `default_catalog.toml` with all 31 built-in tools, 4 categories (`harnesses`, `ai-tuis`, `terminal`, `utilities`), and 4 preset profiles (`desktop`, `server`, `harnesses`, `minimal`).
  - Implemented `LoadDefault`, `Load`, `LoadFromBytes`, and `ExportDefault`.
  - Implemented partial override merging for `[settings]`, `[[tools]]` (including `backend = "source"` + `build_cmd` synthesis, `disabled = true` filtering, custom tools, and custom categories), and `[profiles.<id>]` / `[[profiles]]` with bidirectional tool-profile membership synchronization.
  - Added AST import-boundary tests verifying `internal/domain` has zero internal dependencies and `internal/catalog` depends only on `internal/domain`.

## 2. Review Findings

- **P0 / P1 Blockers:** None.
- **Observations:** All 31 built-in tools and 4 profiles match `docs/specs/product.md` §4 and `docs/mockups/fixtures.js`. User override merging handles both `[profiles.vps-worker]` table maps and `[[profiles]]` arrays cleanly.

## 3. Verification Evidence

```text
$ make check
go vet ./...
go tool github.com/golangci/golangci-lint/v2/cmd/golangci-lint run
0 issues.
go run ./internal/tools/docscheck .
docs-check: ok
go test ./...
ok  	github.com/juanrubio/outfitter/internal/catalog
ok  	github.com/juanrubio/outfitter/internal/cli
ok  	github.com/juanrubio/outfitter/internal/domain
ok  	github.com/juanrubio/outfitter/internal/tools/docscheck

$ make race
go test -race ./...
ok  	github.com/juanrubio/outfitter/internal/catalog
ok  	github.com/juanrubio/outfitter/internal/cli
ok  	github.com/juanrubio/outfitter/internal/domain
ok  	github.com/juanrubio/outfitter/internal/tools/docscheck
```
