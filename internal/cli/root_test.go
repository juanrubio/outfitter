package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersion(t *testing.T) {
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "outfitter version dev") {
		t.Fatalf("version output = %q", out.String())
	}
}

func TestHelp(t *testing.T) {
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	help := out.String()
	for _, want := range []string{"outfitter", "--demo", "--catalog", "--version"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help output missing %q:\n%s", want, help)
		}
	}
}

func TestFlagsRegistered(t *testing.T) {
	cmd := NewRootCmd()
	for _, name := range []string{"demo", "catalog", "version"} {
		if cmd.Flag(name) == nil {
			t.Fatalf("--%s flag is missing", name)
		}
	}
	for _, name := range []string{"demo", "catalog"} {
		if cmd.PersistentFlags().Lookup(name) == nil {
			t.Fatalf("--%s must be a persistent flag", name)
		}
	}
}

func TestExecuteWithoutTerminalIsClean(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("clean execution returned %v", err)
	}
}
