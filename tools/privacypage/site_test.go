package main

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The site is published as plain files, so a link to a file that is not in site/ is a 404 for
// everyone. This checks every relative href and src in the site's HTML (and the scripts and
// stylesheet it loads) against the files in site/.

var (
	attr    = regexp.MustCompile(`\b(?:href|src)="([^"]*)"`)
	comment = regexp.MustCompile(`(?s)<!--.*?-->`)
)

func TestSiteLinksPointAtFilesInTheSite(t *testing.T) {
	const root = "../../site"
	var pages []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".html") {
			pages = append(pages, p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) < 2 {
		t.Fatalf("expected the top page and the privacy page, found %v", pages)
	}
	for _, page := range pages {
		src, err := os.ReadFile(page)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range attr.FindAllStringSubmatch(comment.ReplaceAllString(string(src), ""), -1) {
			target := m[1]
			if strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "#") {
				continue
			}
			if strings.Contains(target, ":") || strings.HasPrefix(target, "/") {
				t.Errorf("%s: %q is neither a relative link nor an https:// URL", page, target)
				continue
			}
			rel := strings.SplitN(strings.SplitN(target, "#", 2)[0], "?", 2)[0]
			file := path.Join(filepath.ToSlash(filepath.Dir(page)), rel)
			if rel == "" || strings.HasSuffix(rel, "/") {
				file = path.Join(file, "index.html")
			}
			if st, err := os.Stat(filepath.FromSlash(file)); err != nil || st.IsDir() {
				t.Errorf("%s: %q does not point at a file in the site (looked for %s)", page, target, file)
			}
		}
	}
}
