package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The support page is for people who use the store release, which connects to production. These
// tests pin what that page must say and must not say, and the files that deploy-site.yml
// publishes for it (see "Introduction site" in docs/ARCHITECTURE.md).

const (
	formURLToken = "__CONTACT_FORM_URL__"
	downloadBase = "https://killy.sumire-sakamoto.co.jp/download/"
)

// What deploy-site.yml puts in /download/. The page spells the binary as killy-linux-$arch.
var downloadFiles = map[string]bool{
	"killy-linux-amd64":   true,
	"killy-linux-arm64":   true,
	"SHA256SUMS":          true,
	"killy-agent.service": true,
}

func readSite(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("../../site/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSupportPageHasTheInstallSteps(t *testing.T) {
	page := readSite(t, "support/index.html")
	for _, want := range []string{
		"Linux 5.3",
		"amd64",
		"arm64",
		"root",
		"sha256sum -c",
		"/usr/local/bin",
		"sudo killy agent pair --server https://killy.sumire.pink",
		"Add server",
		"サーバーを追加",
		"systemctl enable --now killy-agent.service",
		"Check",
		"確認する",
		"SIGKILL",
		"PID 1",
		`lang="en"`,
		`<script src="../lang.js">`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("support page lacks %q", want)
		}
	}
	for _, id := range []string{"agent", "safety", "contact"} {
		if !strings.Contains(page, `id="`+id+`"`) {
			t.Errorf("support page has no section %q", id)
		}
	}
}

func TestSupportPageIsForProductionOnly(t *testing.T) {
	page := strings.ToLower(readSite(t, "support/index.html"))
	for _, banned := range []string{"stg-", "staging", "demo", "デモ", "github", "testflight", "summon"} {
		if strings.Contains(page, banned) {
			t.Errorf("support page mentions %q", banned)
		}
	}
}

func TestSiteDoesNotPointAtGitHub(t *testing.T) {
	// The repository goes private after the hackathon. The privacy policy text still names the
	// issues page (docs/PRIVACY.md); everything else on the site must not.
	for _, name := range []string{"index.html", "support/index.html", "download/VERSION"} {
		if strings.Contains(strings.ToLower(readSite(t, name)), "github") {
			t.Errorf("%s points at GitHub", name)
		}
	}
	privacy := readSite(t, "privacy/index.html")
	footer := privacy[strings.Index(privacy, "<footer>"):]
	if strings.Contains(strings.ToLower(footer), "github") {
		t.Error("the privacy page's footer points at GitHub")
	}
	if !strings.Contains(footer, `href="../support/"`) {
		t.Error("the privacy page's footer does not link to the support page")
	}
}

func TestTopPageLinksToSupport(t *testing.T) {
	top := readSite(t, "index.html")
	if strings.Count(top, `href="support/`) < 2 {
		t.Error("the top page must link to the support page from the agent section and the footer")
	}
}

func TestDownloadLinksAreFilesThatAreDeployed(t *testing.T) {
	for _, name := range []string{"index.html", "support/index.html"} {
		page := readSite(t, name)
		found := regexp.MustCompile(regexp.QuoteMeta(downloadBase)+`[^\s"'<$]*`).FindAllString(page, -1)
		if name == "support/index.html" && len(found) < 3 {
			t.Errorf("the support page lists %d download URLs; it needs the binary, SHA256SUMS and the unit", len(found))
		}
		for _, u := range found {
			if !downloadFiles[strings.TrimPrefix(u, downloadBase)] && !strings.HasSuffix(u, "killy-linux-") {
				t.Errorf("%s: %s is not a file deploy-site.yml puts in /download/", name, u)
			}
		}
	}
}

func TestPinnedVersionIsOneReleaseTag(t *testing.T) {
	v := readSite(t, "download/VERSION")
	if !regexp.MustCompile(`^v\d+\.\d+\.\d+\n$`).MatchString(v) {
		t.Errorf("site/download/VERSION must be one tag like v1.2.3 and a newline, got %q", v)
	}
}

func TestContactFormURLIsInOnePlace(t *testing.T) {
	page := readSite(t, "support/index.html")
	if n := strings.Count(page, formURLToken); n != 2 {
		t.Errorf("the page has %d %s marks, want 2 (the frame and the new-tab link)", n, formURLToken)
	}
	if regexp.MustCompile(`docs\.google\.com`).MatchString(page) {
		t.Error("the form URL must be only in site/support/form-url")
	}
	// Empty until the form exists; deploy-site.yml publishes nothing while it is empty.
	url := strings.TrimSpace(readSite(t, "support/form-url"))
	if url != "" && !regexp.MustCompile(`^https://docs\.google\.com/forms/d/e/[A-Za-z0-9_-]+/viewform\?embedded=true$`).MatchString(url) {
		t.Errorf("site/support/form-url must be empty or a Google Forms embed URL (…/viewform?embedded=true), got %q", url)
	}
}
