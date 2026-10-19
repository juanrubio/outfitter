package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunAcceptsValidLinksAndAnchors(t *testing.T) {
	root := t.TempDir()
	write(t, root, "docs/README.md", "# Index\n\n[Guide](guide.md#usage) and [ref][g].\n\n[g]: guide.md\n")
	write(t, root, "docs/guide.md", "# Guide\n\n## Usage\n")
	if err := run(root); err != nil {
		t.Fatal(err)
	}
}

func TestRunRejectsBrokenInlineLink(t *testing.T) {
	root := t.TempDir()
	write(t, root, "docs/README.md", "[Missing](missing.md)\n")
	err := run(root)
	if err == nil || !strings.Contains(err.Error(), "missing.md") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunRejectsMissingAnchor(t *testing.T) {
	root := t.TempDir()
	write(t, root, "docs/README.md", "[Guide](guide.md#absent)\n")
	write(t, root, "docs/guide.md", "# Guide\n")
	err := run(root)
	if err == nil || !strings.Contains(err.Error(), "absent") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunRejectsUndefinedReference(t *testing.T) {
	root := t.TempDir()
	write(t, root, "docs/README.md", "[text][absent]\n")
	err := run(root)
	if err == nil || !strings.Contains(err.Error(), "undefined reference") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunRejectsRootSpecification(t *testing.T) {
	root := t.TempDir()
	write(t, root, "SPEC.md", "# Spec\n")
	if err := run(root); err == nil {
		t.Fatal("expected root specification rejection")
	}
}

func TestRunAcceptsTitledAndMultilineLinks(t *testing.T) {
	root := t.TempDir()
	write(t, root, "docs/README.md", "[Guide](guide.md 'title')\n\n[Other](\nguide.md\n)\n")
	write(t, root, "docs/guide.md", "# Guide\n")
	if err := run(root); err != nil {
		t.Fatal(err)
	}
}

func TestRunRejectsBrokenTitledLink(t *testing.T) {
	root := t.TempDir()
	write(t, root, "docs/README.md", "[Missing](missing.md 'title')\n")
	err := run(root)
	if err == nil || !strings.Contains(err.Error(), "missing.md") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunChecksNestedImageAndLinkDestinations(t *testing.T) {
	root := t.TempDir()
	write(t, root, "docs/README.md", "[![Preview](preview.png)](guide.md)\n")
	write(t, root, "docs/preview.png", "image")
	write(t, root, "docs/guide.md", "# Guide\n")
	if err := run(root); err != nil {
		t.Fatal(err)
	}
}

func TestRunRejectsBrokenOuterLinkAroundImage(t *testing.T) {
	root := t.TempDir()
	write(t, root, "docs/README.md", "[![Preview](preview.png)](missing.md)\n")
	write(t, root, "docs/preview.png", "image")
	err := run(root)
	if err == nil || !strings.Contains(err.Error(), "missing.md") {
		t.Fatalf("err = %v", err)
	}
}
