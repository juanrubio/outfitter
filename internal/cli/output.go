package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/juanrubio/outfitter/internal/domain"
)

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func writeStatusTable(w io.Writer, catTools []domain.Tool, states map[string]domain.ToolState) error {
	table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "ID\tNAME\tINSTALLED\tLATEST\tPROVENANCE\tSTATUS"); err != nil {
		return err
	}
	for _, tool := range catTools {
		state := states[tool.ID]
		installed := state.InstalledVersion
		if installed == "" {
			installed = "—"
		}
		latest := state.LatestVersion
		if latest == "" {
			latest = "—"
		}
		provenance := string(state.Provenance)
		if provenance == "" {
			provenance = string(domain.ProvenanceNone)
		}
		if _, err := fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\n",
			tool.ID, tool.Name, installed, latest, provenance, statusLabel(state)); err != nil {
			return err
		}
	}
	return table.Flush()
}
