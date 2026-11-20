// Command privacypage writes site/privacy/index.html, the privacy policy page Google Play links
// to, from docs/PRIVACY.md (English, the source of truth) and docs/PRIVACY.ja.md (its translation).
// Run it from the repository root: go run ./tools/privacypage. The test in this package fails when
// the committed page differs from what this would write.
package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

//go:embed page.html.tmpl
var pageTemplate string

const (
	enDoc   = "docs/PRIVACY.md"
	jaDoc   = "docs/PRIVACY.ja.md"
	pageOut = "site/privacy/index.html"
)

func main() {
	page, err := build(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, "privacypage:", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(pageOut), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "privacypage:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(pageOut, page, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "privacypage:", err)
		os.Exit(1)
	}
}

// build returns the page for the documents under root (the repository root).
func build(root string) ([]byte, error) {
	en, enShape, err := renderFile(filepath.Join(root, enDoc))
	if err != nil {
		return nil, err
	}
	ja, jaShape, err := renderFile(filepath.Join(root, jaDoc))
	if err != nil {
		return nil, err
	}
	if err := sameStructure(enShape, jaShape); err != nil {
		return nil, fmt.Errorf("%s and %s differ in structure: %w", enDoc, jaDoc, err)
	}
	enDate, err := lastUpdated(filepath.Join(root, enDoc))
	if err != nil {
		return nil, err
	}
	jaDate, err := lastUpdated(filepath.Join(root, jaDoc))
	if err != nil {
		return nil, err
	}
	if enDate != jaDate {
		return nil, fmt.Errorf("%s is dated %s but its source %s is dated %s: translate the change, then update the date", jaDoc, jaDate, enDoc, enDate)
	}

	t, err := template.New("page").Parse(pageTemplate)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	// text/template, not html/template: the documents are already HTML built by render, which
	// escapes the text it was given, and html/template would drop the page's generated-by comment.
	err = t.Execute(&buf, map[string]string{"EN": en, "JA": ja})
	return buf.Bytes(), err
}

func renderFile(path string) (string, []string, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	h, shape, err := render(string(src))
	if err != nil {
		return "", nil, fmt.Errorf("%s: %w", path, err)
	}
	return h, shape, nil
}

func sameStructure(a, b []string) error {
	for i := 0; i < len(a) || i < len(b); i++ {
		switch {
		case i >= len(a):
			return fmt.Errorf("block %d: the English document ends, the translation continues with %q", i+1, b[i])
		case i >= len(b):
			return fmt.Errorf("block %d: the translation ends, the English document continues with %q", i+1, a[i])
		case a[i] != b[i]:
			return fmt.Errorf("block %d: English has %q, translation has %q", i+1, a[i], b[i])
		}
	}
	return nil
}
