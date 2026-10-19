# Outfitter Visual References & Implementer Guide

Open [`index.html`](index.html) in a browser. No server, build step, account, or network connection is required.

Start with **01 · Catalog desk** at **120×40**, then inspect **03 · Profile selector**, **04 · Dry-run Plan & Apply**, **05 · Live execution progress**, and **80×24** compact mode.

## Reference Screens

| Screen | Key / Selector | What it demonstrates |
| :--- | :--- | :--- |
| **01 · Catalog desk** | Default | Master-detail split view: 4 collapsible categories, checkboxes (`[✓]`, `[!]`, `[↑]`, `[+]`, `[ ]`), version diffs, backend tags, right-hand inspector, and Pending Action Plan box |
| **02 · Outdated & bulk update** | `u` | Filtering and queuing all outdated tools across `vendor`, `mise:npm`, `mise:ubi`, `mise:aqua`, and `uv` in a single keystroke |
| **03 · Profile selector** | `p` | Preset bundles for **Full Desktop**, **Headless Server / VPS**, **Coding Harnesses Only**, and **Minimal Core** |
| **04 · Dry-run Plan & Apply** | `a` | Exact shell command preview grouped by Bootstrap, Installations, Updates, and Removals, with explicit `[sudo]` badges for `apt` packages |
| **05 · Live execution progress** | `Enter` inside Plan | Overall progress bar, per-step elapsed timings, and live tail of the active installer's stdout/stderr |
| **06 · Missing `mise` auto-bootstrap** | `b` | Banner and prerequisite plan step when running Outfitter on a fresh Debian/Ubuntu box where `~/.local/bin/mise` is not yet installed |
| **07 · Custom `catalog.toml` override** | Selector `07` | Demonstrates user overrides (e.g., building `quotary` from local `~/repos/quotary` via the `source` backend) |
| **08 · Keyboard help** | `?` | Full keyboard navigation reference |

## Interactive Controls

Click inside the terminal frame to focus it, then use the keyboard:
- `↑` / `↓` or `j` / `k`: Move between tools
- `1`, `2`, `3`, `4`: Jump to Category 1–4
- `Tab`: Collapse / expand current category
- `Space`: Toggle selected tool's queued action (`[ ]` ↔ `[+]`, `[!]` → `[↑]` → `[-]`, `[✓]` ↔ `[-]`)
- `u`: Queue updates (`[↑]`) for all outdated tools
- `g`: Toggle all tools in the focused category
- `p`: Open Preset Profile selector
- `a`: Open Dry-Run Plan & Apply confirmation screen (`Enter` inside Plan opens Live Execution view)
- `x`: Reset/clear all pending actions
- `f`: Cycle filter (`All` → `Outdated` → `Pending` → `Installed` → `Not installed`)
- `/`: Incremental search (`Esc` clears)
- `b`: Toggle simulated missing `mise` state
- `?`: Open keyboard help
