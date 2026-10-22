package catalog

const seededHeader = `# Outfitter replacement catalog, seeded from the built-in catalog.
# This file REPLACES the built-in catalog entirely: only the tools,
# categories, and profiles listed below exist while it is selected.
# Deleting an entry deletes it from your catalog. A replacement catalog
# must contain at least one active tool.
# Check your edits with: outfitter catalog validate
# Inspect the result with: outfitter catalog show

[catalog]
schema_version = 1
mode = "replace"
# name = "My workstation"

`

// SeededCatalogTemplate returns a replacement catalog containing the
// complete built-in catalog.
func SeededCatalogTemplate() string {
	return seededHeader + defaultCatalogTOML
}

const blankTemplate = `# Outfitter replacement catalog (blank).
# This file REPLACES the built-in catalog entirely: built-in tools,
# categories, and profiles are NOT inherited.
#
# WARNING: this skeleton is not usable yet. A replacement catalog must
# contain at least one active tool, so add a [[tools]] entry (see the
# commented example below), then run: outfitter catalog validate

[catalog]
schema_version = 1
mode = "replace"
# name = "My workstation"

[settings]
install_dir = "~/.local/bin"
auto_bootstrap_mise = true
github_token_from_gh = true
self_update_check = true

# Categories are optional: a tool's category is created automatically
# when it is not declared here.
# [[categories]]
# id = "terminal"
# num = 1
# title = "TERMINAL"

# Example tool (uncomment and adapt). Backends: mise:aqua, mise:ubi,
# mise:github, mise:npm, uv, apt, vendor, source. The mise, uv, and apt
# backends require a package; install/update/remove commands are then
# derived automatically. Vendor tools must set all three commands.
# [[tools]]
# id = "ttyd"
# name = "ttyd"
# binary = "ttyd"
# category = "terminal"
# backend = "mise:ubi"
# package = "tsl0922/ttyd"
# description = "Share terminal sessions over the web"
# version_args = ["--version"]
# version_regex = 'ttyd version ([0-9.]+)'
# profiles = ["workstation"]

# Profiles are optional. Both this [profiles.<name>] form and the
# [[profiles]] array form (with an explicit id) are supported.
# [profiles.workstation]
# name = "Workstation"
# description = "My everyday tools"
# tools = ["ttyd"]
`

// BlankCatalogTemplate returns a replacement catalog skeleton with no
// tools. It is intentionally not valid until a tool is added.
func BlankCatalogTemplate() string {
	return blankTemplate
}

const extendTemplate = `# Outfitter extension catalog (overlay).
# This file EXTENDS the built-in catalog: entries below are merged over
# the built-in tools by id. Anything you do not specify keeps its
# built-in value, so this file is a no-op until you uncomment or add
# entries. Note that deleting an entry only removes your override; to
# remove a built-in tool in extend mode, set disabled = true on it.
# Check your edits with: outfitter catalog validate

[catalog]
schema_version = 1
mode = "extend"
# name = "My overrides"

# Override settings field by field (unspecified fields keep defaults).
# [settings]
# install_dir = "~/.local/bin"
# self_update_check = false   # stop Outfitter update notices

# Override a built-in tool: only the fields you set change.
# [[tools]]
# id = "quotary"
# backend = "source"
# source_dir = "~/repos/quotary"
# build_cmd = "go build -o ~/.local/bin/quotary ./cmd/quotary"

# Add a new tool.
# [[tools]]
# id = "ttyd"
# name = "ttyd"
# binary = "ttyd"
# category = "terminal"
# backend = "mise:ubi"
# package = "tsl0922/ttyd"
# description = "Share terminal sessions over the web"
# profiles = ["desktop", "server"]

# Hide a built-in tool.
# [[tools]]
# id = "btop"
# disabled = true

# Define a custom profile.
# [profiles.vps-worker]
# name = "VPS Worker"
# description = "Headless agent worker"
# tools = ["tmux", "gh", "claude-code", "codex", "ripgrep", "jq"]
`

// ExtendCatalogTemplate returns a valid overlay catalog that changes
// nothing until entries are added.
func ExtendCatalogTemplate() string {
	return extendTemplate
}
