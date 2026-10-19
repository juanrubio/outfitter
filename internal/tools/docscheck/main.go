// Command docscheck verifies that repository Markdown links resolve and that
// specifications live only under docs/.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var (
	inlineLink = regexp.MustCompile(`\]\(\s*(?:<([^>\n]*)>|([^)\s]+))(?:\s+(?:"[^"]*"|'[^']*'|\([^)]*\)))?\s*\)`)
	refLink    = regexp.MustCompile(`!?\[[^\]]*\]\[([^\]]+)\]`)
	refDef     = regexp.MustCompile(`^\s*\[([^\]]+)\]:\s*(?:<([^>]+)>|(\S+))`)
	heading    = regexp.MustCompile(`^#{1,6}\s+(.*)$`)
)

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	if err := run(root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("docs-check: ok")
}

func run(root string) error {
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	for _, name := range []string{"SPEC.md", "UX.md", "IMPLEMENTATION_PLAN.md"} {
		if _, err := os.Stat(filepath.Join(abs, name)); err == nil {
			return fmt.Errorf("root document %s must live under docs/", name)
		}
	}
	var files []string
	err = filepath.WalkDir(abs, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "screens":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".md") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	anchors := map[string]map[string]bool{}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		anchors[path] = headingAnchors(string(data))
	}
	var failures []string
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := stripFences(string(data))
		defs := map[string]bool{}
		for _, line := range strings.Split(text, "\n") {
			if match := refDef.FindStringSubmatch(line); match != nil {
				defs[strings.ToLower(match[1])] = true
				target := match[2]
				if target == "" {
					target = match[3]
				}
				failures = append(failures, checkTarget(abs, path, target, anchors)...)
			}
		}
		scan := stripInlineCode(text)
		for _, match := range inlineLink.FindAllStringSubmatch(scan, -1) {
			target := match[1]
			if target == "" {
				target = match[2]
			}
			failures = append(failures, checkTarget(abs, path, target, anchors)...)
		}
		for _, match := range refLink.FindAllStringSubmatch(scan, -1) {
			label := strings.ToLower(strings.TrimSpace(match[1]))
			if !defs[label] {
				failures = append(failures, fmt.Sprintf("%s: undefined reference link [%s]", rel(abs, path), match[1]))
			}
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("docs-check failed:\n%s", strings.Join(failures, "\n"))
	}
	return nil
}

func checkTarget(abs, path, target string, anchors map[string]map[string]bool) []string {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	if strings.HasPrefix(target, "#") {
		return checkAnchor(abs, path, path, strings.TrimPrefix(target, "#"), anchors)
	}
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") ||
		strings.HasPrefix(target, "mailto:") {
		return nil
	}
	pathPart, fragment, _ := strings.Cut(target, "#")
	if pathPart == "" {
		return checkAnchor(abs, path, path, fragment, anchors)
	}
	resolved := filepath.Join(filepath.Dir(path), filepath.FromSlash(pathPart))
	if _, err := os.Stat(resolved); err != nil {
		return []string{fmt.Sprintf("%s: broken link %q", rel(abs, path), target)}
	}
	if fragment == "" || !strings.EqualFold(filepath.Ext(resolved), ".md") {
		return nil
	}
	return checkAnchor(abs, path, resolved, fragment, anchors)
}

func checkAnchor(abs, source, target, fragment string, anchors map[string]map[string]bool) []string {
	if fragment == "" {
		return nil
	}
	if found := anchors[target]; found != nil && found[strings.ToLower(fragment)] {
		return nil
	}
	return []string{fmt.Sprintf("%s: missing anchor %q in %s", rel(abs, source), fragment, rel(abs, target))}
}

func rel(abs, path string) string {
	value, err := filepath.Rel(abs, path)
	if err != nil {
		return path
	}
	return value
}

func stripFences(data string) string {
	lines := strings.Split(data, "\n")
	inFence := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			lines[i] = ""
			continue
		}
		if inFence {
			lines[i] = ""
		}
	}
	return strings.Join(lines, "\n")
}

func stripInlineCode(data string) string {
	var b strings.Builder
	inCode := false
	for _, r := range data {
		if r == '`' {
			inCode = !inCode
			b.WriteRune(r)
			continue
		}
		if inCode {
			if r == '\n' {
				b.WriteRune('\n')
			} else {
				b.WriteRune(' ')
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func headingAnchors(data string) map[string]bool {
	result := map[string]bool{}
	for _, line := range strings.Split(stripFences(data), "\n") {
		match := heading.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		result[slug(match[1])] = true
	}
	return result
}

func slug(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	var b strings.Builder
	for _, r := range text {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '-', r == '_':
			b.WriteRune(r)
		case unicode.IsSpace(r):
			b.WriteRune('-')
		}
	}
	return b.String()
}
