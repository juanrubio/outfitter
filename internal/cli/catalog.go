package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/juanrubio/outfitter/internal/catalog"
	"github.com/juanrubio/outfitter/internal/engine"
)

func newCatalogCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "catalog",
		Short: "Create, inspect, validate, and export tool catalogs",
	}
	cmd.AddCommand(
		newCatalogInitCmd(opts),
		newCatalogShowCmd(opts),
		newCatalogValidateCmd(opts),
		newCatalogExportCmd(),
	)
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

func newCatalogInitCmd(opts *options) *cobra.Command {
	var blank, extend, force bool
	var output string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Write a starter user catalog (replacement seeded from the built-in catalog by default)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if blank && extend {
				return &ExitError{Code: 2, Err: fmt.Errorf("--blank and --extend cannot be used together")}
			}

			templateName := "seeded replacement"
			mode := catalog.ModeReplace
			content := catalog.SeededCatalogTemplate()
			switch {
			case blank:
				templateName = "blank replacement"
				content = catalog.BlankCatalogTemplate()
			case extend:
				templateName = "extend overlay"
				mode = catalog.ModeExtend
				content = catalog.ExtendCatalogTemplate()
			}
			// The blank skeleton is intentionally not usable yet, so only
			// seeded and extend templates are validated before writing.
			if !blank {
				if _, err := catalog.LoadFromBytes([]byte(content)); err != nil {
					return fmt.Errorf("generated %s catalog is invalid: %w", templateName, err)
				}
			}

			path := output
			if path == "" {
				path = opts.defaultCatalogPath()
			}
			if path == "" {
				return fmt.Errorf("cannot determine the default catalog path; pass --output")
			}
			path = filepath.Clean(path)

			backup, err := writeCatalogFile(path, content, force)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if _, err := fmt.Fprintf(out, "Created %s catalog (%s mode) at %s\n", templateName, mode, path); err != nil {
				return err
			}
			if backup != "" {
				if _, err := fmt.Fprintf(out, "Previous catalog backed up to %s\n", backup); err != nil {
					return err
				}
			}
			if blank {
				_, err := fmt.Fprintln(out, "Warning: this blank catalog has no tools yet and is not usable "+
					"until you add at least one [[tools]] entry. Then run: outfitter catalog validate")
				return err
			}
			_, err = fmt.Fprintln(out, "Next steps: edit the file, then run "+
				"`outfitter catalog validate` and `outfitter catalog show`.")
			return err
		},
	}
	cmd.Flags().BoolVar(&blank, "blank", false, "write an empty replacement skeleton instead of the seeded catalog")
	cmd.Flags().BoolVar(&extend, "extend", false, "write a small overlay catalog that extends the built-in catalog")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing file (the old file is kept as a .bak sibling)")
	cmd.Flags().StringVar(&output, "output", "", "where to write the catalog (default ~/.config/outfitter/catalog.toml)")
	return cmd
}

// writeCatalogFile writes content to path, creating parent directories. It
// refuses to overwrite an existing file unless force is set, in which case
// the old file is first renamed to a unique .bak sibling whose path is
// returned.
func writeCatalogFile(path, content string, force bool) (string, error) {
	backup := ""
	if _, err := os.Stat(path); err == nil {
		if !force {
			return "", fmt.Errorf("catalog already exists at %s; pass --force to overwrite it (a backup will be kept)", path)
		}
		backup = uniqueBackupPath(path)
		if err := os.Rename(path, backup); err != nil {
			return "", fmt.Errorf("back up existing catalog %s: %w", path, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read user catalog %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return backup, fmt.Errorf("create catalog directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return backup, fmt.Errorf("write catalog %s: %w", path, err)
	}
	return backup, nil
}

func uniqueBackupPath(path string) string {
	stamp := time.Now().Format("20060102-150405")
	candidate := path + "." + stamp + ".bak"
	for i := 1; ; i++ {
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
		candidate = fmt.Sprintf("%s.%s-%d.bak", path, stamp, i)
	}
}

type catalogReport struct {
	Name          string `json:"name"`
	SchemaVersion int    `json:"schema_version"`
	Mode          string `json:"mode"`
	Origin        string `json:"origin"`
	Path          string `json:"path,omitempty"`
	UsesBuiltin   bool   `json:"uses_builtin"`
	ToolCount     int    `json:"tool_count"`
	CategoryCount int    `json:"category_count"`
	ProfileCount  int    `json:"profile_count"`
}

func reportFor(cat *catalog.Catalog) catalogReport {
	return catalogReport{
		Name:          cat.Meta.EffectiveName(),
		SchemaVersion: cat.Meta.SchemaVersion,
		Mode:          string(cat.Meta.Mode),
		Origin:        string(cat.Meta.Source),
		Path:          cat.Meta.Path,
		UsesBuiltin:   cat.Meta.UsesBuiltin,
		ToolCount:     len(cat.Tools),
		CategoryCount: len(cat.Categories),
		ProfileCount:  len(cat.Profiles),
	}
}

func writeCatalogReport(w io.Writer, report catalogReport) error {
	lines := []struct{ label, value string }{
		{"Name", report.Name},
		{"Schema version", fmt.Sprint(report.SchemaVersion)},
		{"Mode", report.Mode},
		{"Origin", report.Origin},
	}
	if report.Path != "" {
		lines = append(lines, struct{ label, value string }{"Path", report.Path})
	}
	builtin := "no"
	if report.UsesBuiltin {
		builtin = "yes"
	}
	lines = append(lines,
		struct{ label, value string }{"Uses built-in content", builtin},
		struct{ label, value string }{"Tools", fmt.Sprint(report.ToolCount)},
		struct{ label, value string }{"Categories", fmt.Sprint(report.CategoryCount)},
		struct{ label, value string }{"Profiles", fmt.Sprint(report.ProfileCount)},
	)
	for _, line := range lines {
		if _, err := fmt.Fprintf(w, "%s: %s\n", line.label, line.value); err != nil {
			return err
		}
	}
	return nil
}

func newCatalogShowCmd(opts *options) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show the effective catalog's origin, mode, and counts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cat, err := opts.loadRealCatalog()
			if err != nil {
				return fmt.Errorf("%w (hint: run `outfitter catalog validate` to check the file, or pass --ignore-user-catalog to use the built-in catalog)", err)
			}
			report := reportFor(cat)
			if jsonOut {
				return writeJSON(cmd.OutOrStdout(), report)
			}
			return writeCatalogReport(cmd.OutOrStdout(), report)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit machine-readable JSON")
	return cmd
}

type catalogValidateReport struct {
	Valid bool   `json:"valid"`
	Error string `json:"error,omitempty"`
	catalogReport
}

func newCatalogValidateCmd(opts *options) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "validate [path]",
		Short: "Validate a catalog file without inspecting any tools",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 1 {
				return &ExitError{Code: 2, Err: fmt.Errorf("catalog validate accepts at most one path")}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, target, err := opts.loadValidateTarget(args)
			if err != nil {
				if jsonOut {
					report := catalogValidateReport{Valid: false, Error: err.Error()}
					report.Path = target
					if writeErr := writeJSON(cmd.OutOrStdout(), report); writeErr != nil {
						return writeErr
					}
				}
				var exitErr *ExitError
				if errors.As(err, &exitErr) {
					return err
				}
				return &ExitError{Code: 1, Err: fmt.Errorf("catalog is invalid: %w", err)}
			}
			report := reportFor(cat)
			if report.Path == "" {
				report.Path = target
			}
			if jsonOut {
				return writeJSON(cmd.OutOrStdout(), catalogValidateReport{Valid: true, catalogReport: report})
			}
			out := cmd.OutOrStdout()
			if _, err := fmt.Fprintln(out, "Catalog is valid."); err != nil {
				return err
			}
			return writeCatalogReport(out, report)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit machine-readable JSON")
	return cmd
}

// loadValidateTarget resolves the validate target: positional path, then
// --catalog, then the default user file when present, else the built-in
// catalog. It returns the catalog and the file path it came from ("" for
// the built-in catalog).
func (opts *options) loadValidateTarget(args []string) (*catalog.Catalog, string, error) {
	if len(args) == 1 {
		cat, err := catalog.LoadWithOptions(catalog.LoadOptions{Path: args[0]})
		return cat, args[0], err
	}
	if opts.catalog != "" {
		cat, err := catalog.LoadWithOptions(catalog.LoadOptions{Path: opts.catalog})
		return cat, opts.catalog, err
	}
	if opts.ignoreUserCatalog {
		cat, err := catalog.LoadDefault()
		return cat, "", err
	}
	path := opts.defaultCatalogPath()
	if path != "" {
		if _, err := os.Stat(path); err == nil {
			cat, loadErr := catalog.LoadWithOptions(catalog.LoadOptions{Path: path})
			return cat, path, loadErr
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, path, fmt.Errorf("read user catalog %s: %w", path, err)
		}
	}
	cat, err := catalog.LoadDefault()
	return cat, "", err
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
