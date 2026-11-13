package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTheRecordedKiroEventIsRead(t *testing.T) {
	stdin := rollup(t, "event-merge") // tool_input as Kiro sent it, with description, cwd and the rest
	mustBlock(t, decide(stdin, (&fakeGH{out: rollup(t, "rollup-pr62-success")}).view), "add --match-head-commit "+head62)
}

func TestAPROfOnlyTheGoJobIsAllowedWhenItSucceeded(t *testing.T) {
	// #53 ran before the app job existed.
	gh := &fakeGH{out: rollup(t, "rollup-pr53-go-only")}
	headOf53 := func() string {
		var s struct{ HeadRefOid string }
		_ = json.Unmarshal(rollup(t, "rollup-pr53-go-only"), &s)
		return s.HeadRefOid
	}()
	if d := decide(event(t, "gh pr merge 53 --match-head-commit "+headOf53), gh.view); !d.allow {
		t.Fatalf("blocked: %s", d.reason)
	}
}

func TestSpellingsOfAMergeThatDoNotStartWithGHPRMergeBlock(t *testing.T) {
	for _, cmd := range []string{
		"gh pr -R other/repo merge 62 --match-head-commit " + head62,
		"gh pr --repo=other/repo merge 62 --match-head-commit " + head62,
		"env X=1 gh pr merge 62 --match-head-commit " + head62,
		"/opt/homebrew/bin/gh pr merge 62 --match-head-commit " + head62,
		"sh -c 'gh pr merge 62 --match-head-commit " + head62 + "'",
	} {
		gh := &fakeGH{out: rollup(t, "rollup-pr62-success")}
		mustBlock(t, decide(event(t, cmd), gh.view), "gh pr merge <number> --match-head-commit <sha>")
		if len(gh.calls) != 0 {
			t.Fatalf("%q: gh was called", cmd)
		}
	}
}

func TestAMergeSplitByATabOrALineContinuationIsStillJudged(t *testing.T) {
	for _, cmd := range []string{
		"gh pr\tmerge 61 --match-head-commit " + head61,
		"gh pr \\\nmerge 61 --match-head-commit " + head61,
	} {
		mustBlock(t, decide(event(t, cmd), (&fakeGH{out: rollup(t, "rollup-pr61-failure")}).view), "go: FAILURE")
	}
}

func TestAutoAndAdminAreNotAccepted(t *testing.T) {
	for _, flag := range []string{"--auto", "--admin"} {
		mustBlock(t, decide(event(t, "gh pr merge 62 "+flag+" --match-head-commit "+head62), (&fakeGH{out: rollup(t, "rollup-pr62-success")}).view), "flag "+flag)
	}
}

func TestPRMergeInsideAQuotedMessageIsNotTakenForAMerge(t *testing.T) {
	gh := &fakeGH{}
	for _, cmd := range []string{`git commit -m "note on pr merge"`, `git merge main`} {
		if d := decide(event(t, cmd), gh.view); !d.allow {
			t.Fatalf("%q blocked: %s", cmd, d.reason)
		}
	}
}

func TestABlockedTextMentionTellsHowToPassText(t *testing.T) {
	mustBlock(t, decide(event(t, `git commit -m "see gh pr merge 62 later"`), (&fakeGH{}).view), "--body-file")
}

func fakeOnPath(t *testing.T, script string) {
	t.Helper()
	bin := t.TempDir()
	if script != "" {
		if err := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
}

func TestGHMissingFromPATHIsAnError(t *testing.T) {
	fakeOnPath(t, "")
	if _, err := ghView("62", time.Second); err == nil {
		t.Fatal("no error without gh")
	}
}

func TestGHThatDoesNotAnswerInTimeIsAnError(t *testing.T) {
	fakeOnPath(t, "exec /bin/sleep 5")
	start := time.Now()
	_, err := ghView("62", 200*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "did not answer") {
		t.Fatalf("err = %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("took %v", time.Since(start))
	}
}

func TestGHWhoseChildKeepsThePipeOpenDoesNotHoldTheGate(t *testing.T) {
	fakeOnPath(t, "/bin/sleep 5 & echo '{}'")
	start := time.Now()
	_, _ = ghView("62", 20*time.Second)
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("took %v", d)
	}
}

func TestGHThatFailsIsAnError(t *testing.T) {
	fakeOnPath(t, "echo 'HTTP 502' >&2; exit 1")
	if _, err := ghView("62", time.Second); err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("err = %v", err)
	}
}

func TestGHIsAskedForTheHeadAndTheChecks(t *testing.T) {
	fakeOnPath(t, `echo "$@"`)
	out, err := ghView("62", time.Second)
	if err != nil || strings.TrimSpace(string(out)) != "pr view 62 --json headRefOid,statusCheckRollup" {
		t.Fatalf("out %q err %v", out, err)
	}
}

func TestRunExitsTwoAndWritesTheReasonOnABlock(t *testing.T) {
	var stderr bytes.Buffer
	view := func(string) ([]byte, error) { return nil, errors.New("down") }
	if code := run(bytes.NewReader(event(t, "gh pr merge 62 --match-head-commit "+head62)), &stderr, view); code != 2 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stderr.String(), "Merge blocked (#60)") || !strings.Contains(stderr.String(), "Do not merge another way") {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestRunExitsZeroAndWritesNothingWhenAllowed(t *testing.T) {
	var stderr bytes.Buffer
	if code := run(strings.NewReader(string(event(t, "ls"))), &stderr, nil); code != 0 || stderr.Len() != 0 {
		t.Fatalf("exit %d stderr %q", code, stderr.String())
	}
}
