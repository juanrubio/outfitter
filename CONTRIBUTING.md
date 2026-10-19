# Contributing to Outfitter

Use Go 1.27.2 or newer. Read [AGENTS.md](AGENTS.md) and the
[documentation index](docs/README.md) before changing behavior.

1. Create a focused branch.
2. Add or update tests for behavior changes.
3. Run `make check` and, for execution or concurrency work, `make race`.
4. Update the relevant specification or plan when behavior or scope changes.
5. Keep demo data fictional and fixtures redacted.

Do not commit credentials, cookies, tokens, account emails, raw provider
payloads, or user filesystem paths.
