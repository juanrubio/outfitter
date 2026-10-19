# Goal: Implement Outfitter through all planned phases using Pi + DeepSeek Flash subagents

Use this file as the session objective. Read it fully, then execute it phase by phase without stopping until all phases (Phase 0 through Phase 6) are implemented, reviewed, verified, and committed.

> Suggested invocation:
> `/goal Follow docs/prompts/implement-all-phases.md exactly. Implement Phases 0 through 6 of Outfitter by spawning a pi subagent with openrouter model deepseek/deepseek-v4.1-flash-1017 and high reasoning effort (--thinking high) for each phase, reviewing and verifying the code yourself after every phase before committing.`

---

## 1. Role & Architecture

You are the **Lead Architect and Code Reviewer** for **Outfitter** (`outfitter` in `/home/joni/repos/ai-dev-toolkit`).
Your job is to orchestrate the complete implementation of Outfitter across **Phases 0 through 6** as defined in:
- `IMPLEMENTATION_PLAN.md` (moved to `docs/implementation-plan.md` in Phase 0)
- `SPEC.md` (moved to `docs/specs/product.md` in Phase 0)
- `UX.md` (moved to `docs/specs/tui.md` in Phase 0)
- `docs/mockups/index.html`, `docs/mockups/fixtures.js`, `docs/mockups/mockup.js`

For **every phase**, you MUST delegate the implementation work to a **`pi` coding harness subagent** running the **`deepseek/deepseek-v4.1-flash-1017`** model on OpenRouter (`https://openrouter.ai/deepseek/deepseek-v4.1-flash-1017`) with **high reasoning effort (`--thinking high`)**, and then **personally review, test, and gate** the subagent's code before committing the phase and advancing to the next one.

---

## 2. How to Spawn the `pi` Subagent for Each Phase

For each Phase `<N>` (`0` through `6`):

1. Write a clear, self-contained task specification for Phase `<N>` to `/tmp/outfitter-pi-phase-<N>-prompt.md` that tells the `pi` agent:
   - Which files to read first (`IMPLEMENTATION_PLAN.md` / `docs/implementation-plan.md`, `SPEC.md` / `docs/specs/product.md`, `UX.md` / `docs/specs/tui.md`, and sibling conventions in `../gh-boardwalk`, `../trailmark`, `../quotary`).
   - Every concrete deliverable, package path, type signature, and test required for Phase `<N>`.
   - To run `make check` and `go test -race ./...` before finishing, and NOT to run `git commit` (the lead agent commits only after review).

2. Invoke `pi` in non-interactive mode using `run_command` with `--provider openrouter --model deepseek/deepseek-v4.1-flash-1017 --thinking high`:

```bash
mkdir -p /tmp/outfitter-pi
pi --print \
  --provider openrouter \
  --model deepseek/deepseek-v4.1-flash-1017 \
  --thinking high \
  --approve \
  "@/tmp/outfitter-pi-phase-<N>-prompt.md" \
  | tee "/tmp/outfitter-pi/phase-<N>-run.log"
```

*(Note: `pi` emits a benign `Warning: Model "deepseek/deepseek-v4.1-flash-1017" not found for provider "openrouter". Using custom model id.` and routes directly to `deepseek/deepseek-v4.1-flash-1017` on OpenRouter with `thinking: high`. Do not substitute any other model ID or lower thinking level.)*

---

## 3. Mandatory Lead Agent Code Review Gate (After Every Phase)

Never trust a subagent's self-report that a phase is done. After the `pi` subagent completes Phase `<N>`, you MUST perform a rigorous code review before committing:

1. **Inspect the Workspace & Diff:**
   - Run `git status --porcelain` and `git diff` (and view all newly created files).
   - Check package boundaries: `internal/domain` and `internal/catalog` must never import `internal/tui` or `internal/adapter`; `internal/tui` must use semantic theme tokens from `internal/theme` (no hardcoded hex strings).
2. **Run Independent Verification:**
   - Run `make check` (`go vet ./...`, `golangci-lint`, `docs-check`, `go test ./...`).
   - Run `go test -race ./...`.
   - Run `./outfitter --version` and `./outfitter --demo` (or non-interactive CLI subcommands) to verify actual runtime behavior.
3. **Audit Against Spec & Exit Gate:**
   - Verify every item in the Phase `<N>` Exit Gate in `docs/implementation-plan.md`.
   - Ensure tests are genuine, non-vacuous, and offline-safe (no `t.Skip` hacks, no unmocked network/subprocess calls in unit tests).
   - Check security & safety invariants: direct `exec.CommandContext` argument arrays (no unescaped `sh -c` interpolation of user strings), SHA-256 checksum verification for `mise` bootstrap, and dry-run/confirmation protections.
4. **Fix & Re-Verify Loop:**
   - If you find **P0** (broken build/tests, spec violation, safety bug) or **P1** (missing phase requirement, weak test coverage, lint failure) issues:
     - Either invoke `pi` again (`--provider openrouter --model deepseek/deepseek-v4.1-flash-1017 --thinking high`) with a targeted fix prompt listing the exact file/line findings, or fix the defect directly and re-run `make check` and `go test -race ./...`.
5. **Record Review Evidence & Commit:**
   - Write a review report to `docs/evidence/reviews/phase-<N>.md` recording:
     - Phase number and title
     - `pi` model used (`openrouter` / `deepseek/deepseek-v4.1-flash-1017` with `--thinking high`)
     - Summary of files created/modified
     - Findings discovered during your review and how they were resolved
     - Verification command outputs (`make check`, `go test -race ./...`)
     - Final Verdict: `PASS`
   - Update `docs/evidence/implementation-progress.md`.
   - Commit the completed phase with a clean conventional commit message (`feat(phase-<N>): ...`).

---

## 4. Phase-by-Phase Execution Checklist

Execute all 7 phases strictly in order:

- [ ] **Phase 0 — Repository, Documentation Layout, and Tooling Foundation**
  - Initialize git repo and `go.mod` (`github.com/juanrubio/outfitter`).
  - Move `SPEC.md` → `docs/specs/product.md`, `UX.md` → `docs/specs/tui.md`, `IMPLEMENTATION_PLAN.md` → `docs/implementation-plan.md`, add `docs/README.md`, and update all links.
  - Add `Makefile`, `.golangci.yml`, `internal/tools/docscheck`, `AGENTS.md`, `CLAUDE.md`, `CONTRIBUTING.md`, `LICENSE`, and `cmd/outfitter/main.go` + `internal/cli/root.go`.
  - Verify `make check` and `make build` pass, write `docs/evidence/reviews/phase-0.md`, and commit.

- [ ] **Phase 1 — Domain Model, Embedded Catalog, and `catalog.toml` Merger**
  - Implement `internal/domain/` (`Tool`, `Category`, `Profile`, `ToolState`, `DesiredAction` state machine, `ActionPlan`).
  - Implement `internal/catalog/` with embedded `default_catalog.toml` (all 31 tools, 4 categories, 4 profiles) and user `catalog.toml` merger + `ExportDefault()`.
  - Verify unit tests and import-boundary tests pass under `-race`, write `docs/evidence/reviews/phase-1.md`, and commit.

- [ ] **Phase 2 — Theme System, Demo Backend, and TUI Layouts (`120×40` Split & `80×24` Compact)**
  - Implement `internal/theme/` (`Midnight`, `Daylight`, `Monochrome`).
  - Implement `internal/demo/` offline deterministic state matching `docs/mockups/fixtures.js`.
  - Implement `internal/tui/` header, filter bar, collapsible category list, right-hand Inspector + Pending Action Plan panes (`w >= 100`), and compact `80×24` layout.
  - Verify rendering and demo isolation tests pass under `-race`, write `docs/evidence/reviews/phase-2.md`, and commit.

- [ ] **Phase 3 — TUI State Machine, Modals (Profiles, Plan & Apply, Live Progress, Help), and Golden Tests**
  - Implement all keyboard bindings (`j`/`k`, `Space`, `u`, `U`, `g`, `1`–`4`, `Tab`, `p`, `a`, `x`, `f`, `/`, `?`, `q`).
  - Implement Profile Selector modal (`p`), Dry-Run Plan & Apply screen (`a`), Live Execution Progress screen, and Help overlay (`?`).
  - Verify golden snapshot and state-transition tests at `120×40` and `80×24`, write `docs/evidence/reviews/phase-3.md`, and commit.

- [ ] **Phase 4 — State Discovery, Upstream Version Inspector, and `mise` Bootstrapper**
  - Implement SHA-256 verified `BootstrapMise` (`~/.local/bin/mise`).
  - Implement `InspectLocal` and bounded-concurrency `InspectUpstream` (with `gh auth token` support and `apt-cache policy` / `mise` / npm checks).
  - Implement `~/.cache/outfitter/state.json` snapshot cache.
  - Verify offline unit tests with `httptest.Server` and fake runners pass under `-race`, write `docs/evidence/reviews/phase-4.md`, and commit.

- [ ] **Phase 5 — Execution Adapters (`mise`, `apt`, `vendor`, `source`) & Live Runner**
  - Implement `internal/adapter/` for `mise` (`aqua`, `ubi`, `npm`, `uv`), `apt` (with `sudo -v` handoff), `vendor` (`claude`, `muse`, `agy`, `opencode`, `codexbar` bundle), and `source` (`go build`).
  - Implement `internal/engine/runner.go` with streaming output, process-group cleanup, per-tool failure isolation, and `~/.local/state/outfitter/logs/` run logs.
  - Verify adapter and runner tests pass under `-race`, write `docs/evidence/reviews/phase-5.md`, and commit.

- [ ] **Phase 6 — Non-Interactive CLI Subcommands, VHS Tapes, and Release Hardening**
  - Implement `status`, `check`, `profile list/apply`, `install`, `update`, `remove`, and `catalog export` with `--dry-run`, `--yes`, `--json`, and exit codes `0`/`1`/`2`.
  - Add `tapes/showcase.tape` and complete `docs/evidence/traceability.md` and `docs/evidence/implementation-progress.md`.
  - Run final `make all` and `make race`, write `docs/evidence/reviews/phase-6.md`, and commit.
