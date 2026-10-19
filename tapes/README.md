# README showcase

[`showcase.tape`](showcase.tape) records the actual Outfitter TUI in `--demo` mode: category navigation, profile selection, bulk update queuing, the dry-run Plan Review modal, simulated live execution progress, the post-run summary, keyboard help, and theme cycling.

## Recreate

Install Go (the version required by [`go.mod`](../go.mod)), [VHS](https://github.com/charmbracelet/vhs), `ttyd`, and `ffmpeg`, and make them available on `PATH`. The tape uses `DejaVu Sans Mono`.

From the repository root:

```sh
make build
vhs tapes/showcase.tape
```

This writes [`docs/media/showcase.gif`](../docs/media/showcase.gif). Because `--demo` uses deterministic in-memory fixtures (`internal/demo`), the recording never inspects or modifies the host machine and makes zero network requests.
