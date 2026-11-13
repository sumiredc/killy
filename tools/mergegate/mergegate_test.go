package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// Recorded with `gh pr view <n> --json headRefOid,statusCheckRollup` (#60): PR 62 after CI
// succeeded, PR 61 after it failed, PR 53 (the go job only), and PR 63 while its CI was queued
// and then running.
func rollup(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

const (
	head62 = "0c74441b728bb86d29c4f7713c9999c44c64086f"
	head61 = "1444db8bde741376f735c0254b74e39e52e04ec1"
	head63 = "b1d261f94abf2ac7366de3a824319e8d706c1878"
)

// A fake gh: returns out for any call and records the arguments.
type fakeGH struct {
	out   []byte
	err   error
	calls [][]string
}

func (f *fakeGH) view(pr string) ([]byte, error) {
	f.calls = append(f.calls, []string{"pr", "view", pr})
	return f.out, f.err
}

// event builds the stdin of a PreToolUse event for the shell tool, as Kiro sends it.
func event(t *testing.T, command string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"hook_event_name": "PreToolUse",
		"tool_name":       "execute_bash",
		"tool_input":      map[string]any{"command": command},
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mustBlock(t *testing.T, d decision, want string) {
	t.Helper()
	if d.allow {
		t.Fatalf("allowed; want a block mentioning %q", want)
	}
	if !strings.Contains(d.reason, want) {
		t.Fatalf("reason %q does not mention %q", d.reason, want)
	}
}

func TestAMergeIsAllowedOnlyWhenEveryCheckOnTheHeadSucceededAndTheSHAMatches(t *testing.T) {
	gh := &fakeGH{out: rollup(t, "rollup-pr62-success")}
	d := decide(event(t, "gh pr merge 62 --merge --match-head-commit "+head62), gh.view)
	if !d.allow {
		t.Fatalf("blocked: %s", d.reason)
	}
	if len(gh.calls) != 1 || gh.calls[0][2] != "62" {
		t.Fatalf("gh calls = %v, want one view of PR 62", gh.calls)
	}
}

func TestAMergeInsideAChainOfCommandsIsChecked(t *testing.T) {
	gh := &fakeGH{out: rollup(t, "rollup-pr62-success")}
	d := decide(event(t, "cd /x && gh pr merge 62 --match-head-commit "+head62+" && git pull"), gh.view)
	if !d.allow {
		t.Fatalf("blocked: %s", d.reason)
	}
}

func TestChecksStillRunningBlock(t *testing.T) {
	queued := decide(event(t, "gh pr merge 63 --match-head-commit "+head63), (&fakeGH{out: rollup(t, "rollup-pr63-queued")}).view)
	mustBlock(t, queued, "go: QUEUED")
	mustBlock(t, queued, "app: QUEUED")
	running := decide(event(t, "gh pr merge 63 --match-head-commit "+head63), (&fakeGH{out: rollup(t, "rollup-pr63-in-progress")}).view)
	mustBlock(t, running, "go: IN_PROGRESS")
	mustBlock(t, running, "app: IN_PROGRESS")
}

func TestFailedChecksBlock(t *testing.T) {
	gh := &fakeGH{out: rollup(t, "rollup-pr61-failure")}
	mustBlock(t, decide(event(t, "gh pr merge 61 --match-head-commit "+head61), gh.view), "go: FAILURE")
}

func TestEveryConclusionButSuccessBlocks(t *testing.T) {
	for _, c := range []string{"CANCELLED", "SKIPPED", "NEUTRAL", "TIMED_OUT", "ACTION_REQUIRED", "STALE"} {
		out := `{"headRefOid":"` + head62 + `","statusCheckRollup":[{"__typename":"CheckRun","name":"go","status":"COMPLETED","conclusion":"` + c + `"}]}`
		gh := &fakeGH{out: []byte(out)}
		mustBlock(t, decide(event(t, "gh pr merge 62 --match-head-commit "+head62), gh.view), "go: "+c)
	}
}

func TestAStatusContextCountsOnlyWhenItsStateIsSuccess(t *testing.T) {
	for state, allow := range map[string]bool{"SUCCESS": true, "PENDING": false, "EXPECTED": false, "FAILURE": false, "ERROR": false} {
		out := `{"headRefOid":"` + head62 + `","statusCheckRollup":[{"__typename":"StatusContext","context":"ext","state":"` + state + `"}]}`
		gh := &fakeGH{out: []byte(out)}
		d := decide(event(t, "gh pr merge 62 --match-head-commit "+head62), gh.view)
		if d.allow != allow {
			t.Fatalf("state %s: allow = %v, want %v (%s)", state, d.allow, allow, d.reason)
		}
	}
}

func TestACheckOfAnUnknownKindBlocks(t *testing.T) {
	out := `{"headRefOid":"` + head62 + `","statusCheckRollup":[{"__typename":"SomethingNew","name":"x"}]}`
	mustBlock(t, decide(event(t, "gh pr merge 62 --match-head-commit "+head62), (&fakeGH{out: []byte(out)}).view), "x")
}

func TestNoChecksAtAllBlocks(t *testing.T) {
	out := `{"headRefOid":"` + head62 + `","statusCheckRollup":[]}`
	mustBlock(t, decide(event(t, "gh pr merge 62 --match-head-commit "+head62), (&fakeGH{out: []byte(out)}).view), "no checks")
}

func TestTheMatchHeadCommitFlagIsRequiredAndMustBeTheHead(t *testing.T) {
	ok := rollup(t, "rollup-pr62-success")
	cases := map[string]string{
		"gh pr merge 62 --merge":                                        head62,
		"gh pr merge 62 --match-head-commit 0c74441":                    "40",
		"gh pr merge 62 --match-head-commit " + head61:                  head62,
		"gh pr merge 62 --match-head-commit=" + strings.ToUpper(head62): "40",
	}
	for cmd, want := range cases {
		mustBlock(t, decide(event(t, cmd), (&fakeGH{out: ok}).view), want)
	}
	mustBlock(t, decide(event(t, "gh pr merge 62 --merge"), (&fakeGH{out: ok}).view), "add --match-head-commit "+head62)
	if d := decide(event(t, "gh pr merge 62 --match-head-commit="+head62), (&fakeGH{out: ok}).view); !d.allow {
		t.Fatalf("--match-head-commit=<sha> blocked: %s", d.reason)
	}
}

func TestOnlyAPRNumberIsAccepted(t *testing.T) {
	for _, cmd := range []string{
		"gh pr merge --match-head-commit " + head62,
		"gh pr merge https://github.com/sumiredc/killy/pull/62 --match-head-commit " + head62,
		"gh pr merge ci/14-evidence --match-head-commit " + head62,
		"gh pr merge 62 61 --match-head-commit " + head62,
	} {
		gh := &fakeGH{out: rollup(t, "rollup-pr62-success")}
		mustBlock(t, decide(event(t, cmd), gh.view), "gh pr merge <number> --match-head-commit <sha>")
		if len(gh.calls) != 0 {
			t.Fatalf("%q: gh was called", cmd)
		}
	}
}

func TestAnotherRepositoryIsNotAccepted(t *testing.T) {
	for _, cmd := range []string{
		"gh pr merge 62 -R other/repo --match-head-commit " + head62,
		"gh pr merge 62 --repo other/repo --match-head-commit " + head62,
		"gh pr merge 62 --repo=other/repo --match-head-commit " + head62,
	} {
		mustBlock(t, decide(event(t, cmd), (&fakeGH{out: rollup(t, "rollup-pr62-success")}).view), "--repo")
	}
}

func TestTwoMergesInOneCommandBlock(t *testing.T) {
	cmd := "gh pr merge 62 --match-head-commit " + head62 + " && gh pr merge 61 --match-head-commit " + head61
	mustBlock(t, decide(event(t, cmd), (&fakeGH{out: rollup(t, "rollup-pr62-success")}).view), "one")
}

func TestQuotesInTheMergeCommandBlock(t *testing.T) {
	cmd := `gh pr merge 62 --subject "x y" --match-head-commit ` + head62
	mustBlock(t, decide(event(t, cmd), (&fakeGH{out: rollup(t, "rollup-pr62-success")}).view), "gh pr merge <number> --match-head-commit <sha>")
}

func TestAFailingGHBlocks(t *testing.T) {
	gh := &fakeGH{err: os.ErrNotExist}
	mustBlock(t, decide(event(t, "gh pr merge 62 --match-head-commit "+head62), gh.view), "gh")
}

func TestAnAnswerThatIsNotJSONBlocks(t *testing.T) {
	gh := &fakeGH{out: []byte("<html>")}
	mustBlock(t, decide(event(t, "gh pr merge 62 --match-head-commit "+head62), gh.view), "gh")
}

func TestAMergeThatCannotBeReadFromTheEventBlocks(t *testing.T) {
	for name, stdin := range map[string]string{
		"no command field": `{"tool_name":"execute_bash","tool_input":{"cmd":"gh pr merge 62"}}`,
		"not JSON":         `gh pr merge 62`,
		"command not text": `{"tool_input":{"command":["gh pr merge 62"]}}`,
		"no tool_input":    `{"note":"gh pr merge 62"}`,
	} {
		gh := &fakeGH{out: rollup(t, "rollup-pr62-success")}
		mustBlock(t, decide([]byte(stdin), gh.view), "cannot read")
		if len(gh.calls) != 0 {
			t.Fatalf("%s: gh was called", name)
		}
	}
}

func TestAnEventWhoseFieldsChangedStillPassesCommandsWithoutPRMerge(t *testing.T) {
	// Otherwise a renamed field in a Kiro update would block every shell command.
	if d := decide([]byte(`{"tool_input":{"cmd":"ls -la"}}`), (&fakeGH{}).view); !d.allow {
		t.Fatalf("blocked: %s", d.reason)
	}
}

func TestQuotesAndVariablesInTheMergeCommandBlock(t *testing.T) {
	for _, cmd := range []string{
		`gh pr merge "62" --match-head-commit ` + head62,
		`gh pr merge 62 --match-head-commit $SHA`,
		`gh pr merge 62 --match-head-commit "` + head62 + `"`,
	} {
		gh := &fakeGH{out: rollup(t, "rollup-pr62-success")}
		if d := decide(event(t, cmd), gh.view); d.allow {
			t.Fatalf("%q allowed", cmd)
		}
	}
}

func TestACommandWithoutPRMergeIsAllowedWithoutCallingGH(t *testing.T) {
	gh := &fakeGH{}
	for _, cmd := range []string{"gh pr view 59", "gh pr checks 62", "git merge main", "ls"} {
		if d := decide(event(t, cmd), gh.view); !d.allow {
			t.Fatalf("%q blocked: %s", cmd, d.reason)
		}
	}
	if len(gh.calls) != 0 {
		t.Fatalf("gh was called: %v", gh.calls)
	}
}

func TestEveryBlockSaysNotToMergeAnotherWay(t *testing.T) {
	d := decide(event(t, "gh pr merge 62"), (&fakeGH{out: rollup(t, "rollup-pr62-success")}).view)
	mustBlock(t, d, "Do not merge another way")
}

func TestGHGivesUpAfter20Seconds(t *testing.T) {
	if ghTimeout != 20*time.Second {
		t.Fatalf("ghTimeout = %v, want 20s", ghTimeout)
	}
}
