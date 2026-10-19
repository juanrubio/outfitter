// Package demo provides Outfitter's deterministic, fully offline demonstration
// snapshot. It performs no filesystem discovery, subprocess execution, or
// network requests.
package demo

import (
	"fmt"

	"github.com/juanrubio/outfitter/internal/catalog"
	"github.com/juanrubio/outfitter/internal/domain"
)

// Snapshot is the complete fictional state used by `outfitter --demo`.
type Snapshot struct {
	Catalog        *catalog.Catalog
	States         map[string]domain.ToolState
	Actions        map[string]domain.DesiredAction
	MiseInstalled  bool
	MiseVersion    string
	PlatformNotice string
}

const quotaryOverride = `
[[tools]]
id = "quotary"
backend = "source"
package = "~/repos/quotary (local override)"
source_dir = "~/repos/quotary"
install_cmd = "go -C ~/repos/quotary build -o ~/.local/bin/quotary ./cmd/quotary"
update_cmd = "go -C ~/repos/quotary build -o ~/.local/bin/quotary ./cmd/quotary"
remove_cmd = "rm -f ~/.local/bin/quotary"
`

type stateSpec struct {
	installed  string
	latest     string
	path       string
	provenance domain.Provenance
	detail     string
}

var stateSpecs = map[string]stateSpec{
	"claude-code": {
		installed: "2.1.289", latest: "2.1.289", path: "~/.local/bin/claude",
		provenance: domain.ProvenanceVendor,
		detail:     "Vendor self-updater (~/.local/share/claude/versions)",
	},
	"codex": {
		installed: "0.163.0", latest: "0.165.0", path: "~/.local/share/mise/shims/codex",
		provenance: domain.ProvenanceMise,
		detail:     "Managed by mise (npm:@openai/codex on node@24 LTS)",
	},
	"muse": {
		installed: "1.4.10-R6427.1", latest: "1.4.12-R6501.0", path: "~/.local/bin/muse",
		provenance: domain.ProvenanceVendor,
		detail:     "Vendor OIDC launcher + ~/.local/bin/.muse-version",
	},
	"antigravity-cli": {
		installed: "1.4.2", latest: "1.4.2", path: "~/.local/bin/agy",
		provenance: domain.ProvenanceVendor,
		detail:     "Vendor standalone binary (~/.local/bin/agy)",
	},
	"opencode": {
		installed: "2.0.23", latest: "2.0.23", path: "~/.opencode/bin/opencode",
		provenance: domain.ProvenanceVendor,
		detail:     "Vendor installer symlinked in ~/.local/bin/opencode",
	},
	"pi": {
		installed: "1.3.0", latest: "1.3.0", path: "~/.local/share/mise/shims/pi",
		provenance: domain.ProvenanceMise,
		detail:     "Managed by mise (npm:@earendil-works/pi-coding-agent)",
	},
	"gemini-cli": {
		latest: "0.43.0", provenance: domain.ProvenanceNone, detail: "Not installed",
	},
	"copilot-cli": {
		latest: "1.0.63", provenance: domain.ProvenanceNone, detail: "Not installed",
	},
	"boardwalk": {
		installed: "0.4.2", latest: "0.4.2", path: "~/.local/bin/boardwalk",
		provenance: domain.ProvenanceMise,
		detail:     "Managed by mise (ubi:juanrubio/boardwalk)",
	},
	"trailmark": {
		installed: "0.3.0", latest: "0.3.1", path: "~/.local/bin/trailmark",
		provenance: domain.ProvenanceMise,
		detail:     "Managed by mise (ubi:juanrubio/trailmark)",
	},
	"quotary": {
		latest: "0.1.0-dev", provenance: domain.ProvenanceNone,
		detail: "Not installed (catalog.toml local source override)",
	},
	"codexbar": {
		latest: "0.77.0", provenance: domain.ProvenanceNone,
		detail: "Not installed (bundle + executable kept together)",
	},
	"openclaw": {
		installed: "2026.9.6", latest: "2026.9.6", path: "~/.local/share/mise/shims/openclaw",
		provenance: domain.ProvenanceMise,
		detail:     "Managed by mise (npm:openclaw)",
	},
	"mercadona-cli": {
		installed: "1.0.0", latest: "1.0.0", path: "~/.local/share/mise/shims/mercadona",
		provenance: domain.ProvenanceMise,
		detail:     "Managed by mise (npm:mercadona-cli)",
	},
	"tmux": {
		installed: "3.4-1ubuntu1", latest: "3.4-1ubuntu1", path: "/usr/bin/tmux",
		provenance: domain.ProvenanceAPT, detail: "System package (apt)",
	},
	"byobu": {
		installed: "6.11", latest: "6.11", path: "/usr/bin/byobu",
		provenance: domain.ProvenanceAPT, detail: "System package (apt)",
	},
	"hive": {
		installed: "0.1.2", latest: "0.1.2", path: "/usr/local/bin/hive",
		provenance: domain.ProvenanceExternal, detail: "External binary (/usr/local/bin/hive)",
	},
	"gh": {
		installed: "2.61.0", latest: "2.61.0", path: "/usr/bin/gh",
		provenance: domain.ProvenanceAPT, detail: "System package (GitHub official APT repo)",
	},
	"lazygit": {
		installed: "0.44.0", latest: "0.45.1", path: "~/.local/bin/lazygit",
		provenance: domain.ProvenanceMise,
		detail:     "Managed by mise (aqua:jesseduffield/lazygit)",
	},
	"fzf": {
		installed: "0.62.0", latest: "0.62.0", path: "~/.local/share/mise/shims/fzf",
		provenance: domain.ProvenanceMise,
		detail:     "Managed by mise (aqua:junegunn/fzf)",
	},
	"delta": {
		installed: "0.18.2", latest: "0.18.2", path: "~/.local/bin/delta",
		provenance: domain.ProvenanceMise,
		detail:     "Managed by mise (aqua:dandavison/delta)",
	},
	"glow": {
		installed: "2.0.0", latest: "2.0.0", path: "~/.local/bin/glow",
		provenance: domain.ProvenanceMise,
		detail:     "Managed by mise (aqua:charmbracelet/glow)",
	},
	"ripgrep": {
		installed: "14.1.1", latest: "14.1.1", path: "/usr/bin/rg",
		provenance: domain.ProvenanceMise,
		detail:     "Managed by mise (aqua:BurntSushi/ripgrep)",
	},
	"fd": {
		installed: "10.2.0", latest: "10.2.0", path: "~/.local/share/mise/shims/fd",
		provenance: domain.ProvenanceMise,
		detail:     "Managed by mise (aqua:sharkdp/fd)",
	},
	"jq": {
		installed: "1.7.1", latest: "1.7.1", path: "/usr/bin/jq",
		provenance: domain.ProvenanceMise,
		detail:     "Managed by mise (aqua:jqlang/jq)",
	},
	"btop": {
		latest: "1.3.2", provenance: domain.ProvenanceNone, detail: "Not installed",
	},
	"zoxide": {
		installed: "0.9.6", latest: "0.9.6", path: "~/.local/share/mise/shims/zoxide",
		provenance: domain.ProvenanceMise,
		detail:     "Managed by mise (aqua:ajeetdsouza/zoxide)",
	},
	"eza": {
		installed: "0.20.2", latest: "0.20.2", path: "~/.local/share/mise/shims/eza",
		provenance: domain.ProvenanceMise,
		detail:     "Managed by mise (aqua:eza-community/eza)",
	},
	"bat": {
		installed: "0.25.0", latest: "0.25.0", path: "~/.local/share/mise/shims/bat",
		provenance: domain.ProvenanceMise,
		detail:     "Managed by mise (aqua:sharkdp/bat)",
	},
	"dust": {
		installed: "1.1.1", latest: "1.1.1", path: "~/.local/share/mise/shims/dust",
		provenance: domain.ProvenanceMise,
		detail:     "Managed by mise (aqua:bootandy/dust)",
	},
	"broot": {
		installed: "1.44.2", latest: "1.44.2", path: "~/.local/share/mise/shims/broot",
		provenance: domain.ProvenanceMise,
		detail:     "Managed by mise (ubi:Canop/broot)",
	},
	"tig": {
		installed: "2.5.8", latest: "2.5.8", path: "/usr/bin/tig",
		provenance: domain.ProvenanceAPT, detail: "System package (apt)",
	},
	"htop": {
		installed: "3.3.0", latest: "3.3.0", path: "/usr/bin/htop",
		provenance: domain.ProvenanceAPT, detail: "System package (apt)",
	},
	"browser-use": {
		installed: "0.12.9", latest: "0.13.1", path: "~/.local/bin/browser-use",
		provenance: domain.ProvenanceUV,
		detail:     "Managed by uv tool (~/.local/share/uv/tools/browser-use)",
	},
	"mcporter": {
		installed: "0.11.3", latest: "0.11.3", path: "~/.local/share/mise/shims/mcporter",
		provenance: domain.ProvenanceMise,
		detail:     "Managed by mise (npm:mcporter)",
	},
	"oracle": {
		installed: "0.14.1", latest: "0.14.1", path: "~/.local/share/mise/shims/oracle",
		provenance: domain.ProvenanceMise,
		detail:     "Managed by mise (npm:@steipete/oracle)",
	},
	"summarize": {
		installed: "0.16.3", latest: "0.16.3", path: "~/.local/share/mise/shims/summarize",
		provenance: domain.ProvenanceMise,
		detail:     "Managed by mise (npm:@steipete/summarize)",
	},
	"wacli": {
		installed: "0.12.0", latest: "0.12.0", path: "~/.local/bin/wacli",
		provenance: domain.ProvenanceMise,
		detail:     "Managed by mise (ubi:steipete/wacli)",
	},
	"hcloud": {
		installed: "1.50.0", latest: "1.50.0", path: "~/.local/bin/hcloud",
		provenance: domain.ProvenanceMise,
		detail:     "Managed by mise (aqua:hetznercloud/cli)",
	},
	"visidata": {
		installed: "3.1.1", latest: "3.1.1", path: "~/.local/bin/vd",
		provenance: domain.ProvenanceUV,
		detail:     "Managed by uv tool (~/.local/share/uv/tools/visidata)",
	},
}

// Load builds the deterministic demo snapshot. It reads only compiled-in
// constants and never touches external commands, files, or the network.
func Load() (Snapshot, error) {
	cat, err := catalog.LoadFromBytes([]byte(quotaryOverride))
	if err != nil {
		return Snapshot{}, fmt.Errorf("load demo catalog: %w", err)
	}

	states := make(map[string]domain.ToolState, len(cat.Tools))
	for _, tool := range cat.Tools {
		spec, ok := stateSpecs[tool.ID]
		if !ok {
			return Snapshot{}, fmt.Errorf("demo state missing for tool %q", tool.ID)
		}
		states[tool.ID] = domain.ToolState{
			InstalledVersion: spec.installed,
			LatestVersion:    spec.latest,
			BinaryPath:       spec.path,
			Provenance:       spec.provenance,
			ProvenanceDetail: spec.detail,
			CheckStatus:      domain.StatusReady,
		}
	}

	return Snapshot{
		Catalog: cat,
		States:  states,
		Actions: map[string]domain.DesiredAction{
			"gemini-cli": domain.ActionInstall,
			"quotary":    domain.ActionInstall,
		},
		MiseInstalled:  true,
		MiseVersion:    "2026.10.1",
		PlatformNotice: "Ubuntu 24.04 · amd64",
	}, nil
}
