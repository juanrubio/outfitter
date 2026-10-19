package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/juanrubio/outfitter/internal/catalog"
	"github.com/juanrubio/outfitter/internal/engine"
)

func newCatalogCmd(_ *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "catalog",
		Short: "Inspect and export the tool catalog",
	}
	cmd.AddCommand(newCatalogExportCmd())
	return cmd
}

func newCatalogExportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "export",
		Short: "Write the embedded default catalog to stdout",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprint(cmd.OutOrStdout(), catalog.ExportDefault())
			return err
		},
	}
}

func newInternalBootstrapCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "internal-bootstrap",
		Short:  "Internal bootstrap helpers",
		Hidden: true,
	}
	mise := &cobra.Command{
		Use:    "mise",
		Short:  "Install the verified mise binary",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := engine.BootstrapMise(cmd.Context(), engine.BootstrapOptions{
				HomeDir: opts.deps.HomeDir,
			})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "mise installed at %s\n", path)
			return err
		},
	}
	cmd.AddCommand(mise)
	return cmd
}
