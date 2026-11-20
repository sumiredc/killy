package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommittedPageMatchesTheDocuments(t *testing.T) {
	want, err := build("../..")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../" + pageOut)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("%s is not what docs/PRIVACY.md and docs/PRIVACY.ja.md produce; run `go run ./tools/privacypage` from the repository root and commit the result", pageOut)
	}
}

func TestPageHoldsBothLanguagesInFull(t *testing.T) {
	page, err := build("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<h1>Killy — Privacy Policy</h1>", "<h1>Killy — プライバシーポリシー</h1>", `<div class="en" lang="en">`, `<div class="ja" lang="ja">`} {
		if !strings.Contains(string(page), want) {
			t.Errorf("the page has no %q", want)
		}
	}
}

func TestRenderSubset(t *testing.T) {
	src := "# Title & more\n\nOne **bold `x<y`** and [a link](https://example.com/a?b=1&c=2)\nover two lines, see <https://example.com/>.\n\n- first\n- second [l](https://example.com/l)\n\n| A | B |\n|---|:-:|\n| 1 | `c` |\n| 2 | d |\n"
	got, shape, err := render(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<h1>Title &amp; more</h1>",
		"<strong>bold <code>x&lt;y</code></strong>",
		`<a href="https://example.com/a?b=1&amp;c=2">a link</a>`,
		"over two lines, see",
		`<a href="https://example.com/">https://example.com/</a>`,
		"<li>first</li>",
		"<thead><tr><th>A</th><th>B</th></tr></thead>",
		"<td><code>c</code></td>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
	wantShape := []string{
		"h1",
		"p https://example.com/a?b=1&c=2 https://example.com/",
		"ul 2 https://example.com/l",
		"table 2x2",
	}
	if strings.Join(shape, "\n") != strings.Join(wantShape, "\n") {
		t.Errorf("shape = %q, want %q", shape, wantShape)
	}
}

func TestRenderRefusesWhatItDoesNotUnderstand(t *testing.T) {
	cases := map[string]string{
		"a block quote":          "> quoted\n",
		"a code fence":           "```\ncode\n```\n",
		"a numbered list":        "1. one\n",
		"a star list":            "* one\n",
		"a third-level heading":  "### h\n",
		"an indented block":      "    code\n",
		"raw HTML":               "<div>x</div>\n",
		"raw HTML inside a line": "text <b>x</b>\n",
		"an image":               "![alt](https://example.com/a.png)\n",
		"a relative link":        "[x](PRIVACY.md)\n",
		"an http link":           "[x](http://example.com/)\n",
		"italics":                "some *emphasis* here\n",
		"an unclosed code span":  "some `code\n",
		"a link in a link":       "[a [b](https://example.com/b)](https://example.com/a)\n",
		"a table without rows":   "| A |\n|---|\n",
		"a ragged table":         "| A | B |\n|---|---|\n| 1 |\n",
		"a table with no header": "| 1 | 2 |\n| 3 | 4 |\n| 5 | 6 |\n",
		"a multi-line list item": "- one\n  two\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if out, _, err := render(src); err == nil {
				t.Fatalf("render accepted %q and wrote %q", src, out)
			}
		})
	}
}

// docs writes a pair of documents under a temporary root and returns it.
func docs(t *testing.T, en, ja string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, text := range map[string]string{enDoc: en, jaDoc: ja} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const (
	enBase = "# T\n\nLast updated: 2026-11-19. See <https://example.com/a>.\n\n## H\n\n- one\n- two\n"
	jaBase = "# 題\n\n最終更新: 2026-11-19。<https://example.com/a> を参照。\n\n## 見出し\n\n- 一\n- 二\n"
)

func TestBuildAcceptsATranslationOfTheSameShape(t *testing.T) {
	if _, err := build(docs(t, enBase, jaBase)); err != nil {
		t.Fatal(err)
	}
}

func TestBuildRejectsATranslationThatDiffers(t *testing.T) {
	cases := map[string]string{
		"a list item is missing":     strings.Replace(jaBase, "- 二\n", "", 1),
		"a paragraph is added":       jaBase + "\n追加\n",
		"a link target differs":      strings.Replace(jaBase, "example.com/a", "example.com/b", 1),
		"a heading becomes a para":   strings.Replace(jaBase, "## 見出し", "見出し", 1),
		"the date was not updated":   strings.Replace(jaBase, "2026-11-19", "2026-10-01", 1),
		"the date is missing":        strings.Replace(jaBase, "2026-11-19", "いつか", 1),
		"the translation is missing": "",
	}
	for name, ja := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := build(docs(t, enBase, ja)); err == nil {
				t.Fatal("build accepted the translation")
			}
		})
	}
}

func TestBuildRejectsAnEnglishChangeTheTranslationDoesNotFollow(t *testing.T) {
	en := strings.Replace(enBase, "2026-11-19", "2026-12-01", 1) + "\nNew paragraph.\n"
	if _, err := build(docs(t, en, jaBase)); err == nil {
		t.Fatal("build accepted an English document that changed alone")
	}
}
