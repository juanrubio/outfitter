package catalog_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestImportBoundaries(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test source")
	}
	catalogDir := filepath.Dir(filename)
	domainDir := filepath.Join(catalogDir, "..", "domain")

	forbidden := []string{
		"github.com/juanrubio/outfitter/internal/tui",
		"github.com/juanrubio/outfitter/internal/cli",
		"github.com/juanrubio/outfitter/internal/adapter",
		"github.com/juanrubio/outfitter/internal/engine",
	}

	for _, dir := range []string{catalogDir, domainDir} {
		if err := checkImports(dir, forbidden); err != nil {
			t.Error(err)
		}
	}

	if err := checkDomainHasNoInternalImports(domainDir); err != nil {
		t.Error(err)
	}
}

func checkImports(dir string, forbidden []string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			for _, prefix := range forbidden {
				if imported == prefix || strings.HasPrefix(imported, prefix+"/") {
					return &importBoundaryError{file: path, imported: imported, forbidden: prefix}
				}
			}
		}
	}
	return nil
}

func checkDomainHasNoInternalImports(domainDir string) error {
	entries, err := os.ReadDir(domainDir)
	if err != nil {
		return err
	}

	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		path := filepath.Join(domainDir, entry.Name())
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if imported == "github.com/juanrubio/outfitter/internal/domain" {
				continue
			}
			if strings.HasPrefix(imported, "github.com/juanrubio/outfitter/internal/") {
				return &importBoundaryError{file: path, imported: imported, forbidden: "any other internal package"}
			}
		}
	}
	return nil
}

type importBoundaryError struct {
	file      string
	imported  string
	forbidden string
}

func (e *importBoundaryError) Error() string {
	return e.file + " imports " + e.imported + " (forbidden: " + e.forbidden + ")"
}
