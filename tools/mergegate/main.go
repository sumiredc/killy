// Command mergegate is a Kiro PreToolUse hook for the shell tool (#60). It blocks `gh pr merge`
// unless every check on the PR's head has succeeded and the command pins that head with
// --match-head-commit. It reads the hook event on stdin, exits 0 to allow and 2 to block, and
// writes the reason for a block to stderr. Anything it cannot judge is blocked.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// ghTimeout is how long gh may take. The hook's own timeout is longer, so the gate decides
// (and blocks) rather than Kiro, whose behaviour on a hook timeout is not documented.
const ghTimeout = 20 * time.Second

const usage = "use exactly one `gh pr merge <number> --match-head-commit <sha>` per command"

const textHint = "If `pr merge` is only text (a commit message, a PR body), pass the text with --body-file or -F."

const advice = "Do not merge another way (gh api, git push); wait for CI to finish, or ask the user."

var (
	separator = regexp.MustCompile(`&&|\|\||[;|\n&()]`)
	number    = regexp.MustCompile(`^[0-9]+$`)
	sha       = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// Flags of gh pr merge that do not change which PR or repository is merged.
var harmless = map[string]bool{"--merge": true, "-m": true, "--squash": true, "-s": true, "--rebase": true, "-r": true, "--delete-branch": true, "-d": true}

type decision struct {
	allow  bool
	reason string
}

func allow() decision { return decision{allow: true} }

func block(format string, a ...any) decision {
	return decision{reason: "Merge blocked (#60): " + fmt.Sprintf(format, a...) + "\n" + advice}
}

// decide judges one hook event. view runs `gh pr view <pr> --json headRefOid,statusCheckRollup`.
func decide(stdin []byte, view func(pr string) ([]byte, error)) decision {
	// Looked for in the whole event first, so a renamed field cannot let a merge through.
	if !bytes.Contains(stdin, []byte("merge")) {
		return allow()
	}
	var ev struct {
		ToolInput struct {
			Command json.RawMessage `json:"command"`
		} `json:"tool_input"`
	}
	var command string
	if json.Unmarshal(stdin, &ev) != nil || json.Unmarshal(ev.ToolInput.Command, &command) != nil {
		return block("cannot read the shell command from the hook event")
	}
	merges := mergeSegments(command)
	if len(merges) == 0 {
		return allow()
	}
	if len(merges) > 1 {
		return block("more than one merge in one command; %s", usage)
	}
	pr, pinned, d := parse(merges[0])
	if d != nil {
		return *d
	}
	out, err := view(pr)
	if err != nil {
		return block("could not ask gh about PR %s: %v", pr, err)
	}
	var st struct {
		HeadRefOid        string `json:"headRefOid"`
		StatusCheckRollup []struct {
			Type       string `json:"__typename"`
			Name       string `json:"name"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			Context    string `json:"context"`
			State      string `json:"state"`
		} `json:"statusCheckRollup"`
	}
	if err := json.Unmarshal(out, &st); err != nil || !sha.MatchString(st.HeadRefOid) {
		return block("could not read gh's answer about PR %s", pr)
	}
	if pinned == "" {
		return block("add --match-head-commit %s (the head whose checks are judged); %s", st.HeadRefOid, usage)
	}
	if pinned != st.HeadRefOid {
		return block("--match-head-commit %s is not the head of PR %s, which is %s", pinned, pr, st.HeadRefOid)
	}
	if len(st.StatusCheckRollup) == 0 {
		return block("PR %s has no checks on %s yet; CI has not started", pr, st.HeadRefOid[:7])
	}
	var pending []string
	for _, c := range st.StatusCheckRollup {
		switch c.Type {
		case "CheckRun":
			if c.Status != "COMPLETED" {
				pending = append(pending, c.Name+": "+c.Status)
			} else if c.Conclusion != "SUCCESS" {
				pending = append(pending, c.Name+": "+c.Conclusion)
			}
		case "StatusContext":
			if c.State != "SUCCESS" {
				pending = append(pending, c.Context+": "+c.State)
			}
		default:
			pending = append(pending, c.Name+c.Context+": unknown kind "+c.Type)
		}
	}
	if len(pending) > 0 {
		return block("not every check on %s succeeded: %s", st.HeadRefOid[:7], strings.Join(pending, ", "))
	}
	return allow()
}

// mergeSegments returns each simple command that has a word `pr` followed later by a word
// `merge`, whatever comes between (`gh pr -R x merge` is a merge too). Line continuations are
// joined first so a merge split over lines is still one command.
func mergeSegments(command string) []string {
	command = strings.ReplaceAll(command, "\\\n", " ")
	var out []string
	for _, seg := range separator.Split(command, -1) {
		pr := false
		for _, w := range strings.Fields(seg) {
			if w == "pr" {
				pr = true
			} else if pr && w == "merge" {
				out = append(out, seg)
				break
			}
		}
	}
	return out
}

// parse reads `gh pr merge <number> [harmless flags] --match-head-commit <sha>` and nothing else.
// Quotes, substitutions and variables are not understood, so a token holding one is rejected.
func parse(seg string) (pr, pinned string, d *decision) {
	fail := func(format string, a ...any) (string, string, *decision) {
		b := block(format, a...)
		return "", "", &b
	}
	f := strings.Fields(seg)
	if len(f) < 3 || f[0] != "gh" || f[1] != "pr" || f[2] != "merge" {
		return fail("%s. %s", usage, textHint)
	}
	for i := 3; i < len(f); i++ {
		a := f[i]
		switch {
		case a == "-R" || a == "--repo" || strings.HasPrefix(a, "--repo=") || strings.HasPrefix(a, "-R"):
			return fail("--repo is not accepted, since the gate judges this repository only; %s", usage)
		case a == "--match-head-commit":
			if i+1 >= len(f) {
				return fail("--match-head-commit needs a 40-character SHA")
			}
			i++
			pinned = f[i]
		case strings.HasPrefix(a, "--match-head-commit="):
			pinned = strings.TrimPrefix(a, "--match-head-commit=")
		case harmless[a]:
		case strings.HasPrefix(a, "-"):
			return fail("flag %s is not accepted; %s", a, usage)
		case pr == "" && number.MatchString(a):
			pr = a
		default:
			return fail("%s", usage)
		}
	}
	if pr == "" {
		return fail("%s", usage)
	}
	if pinned != "" && !sha.MatchString(pinned) {
		return fail("--match-head-commit needs the full 40-character lower-case SHA, not %s", pinned)
	}
	return pr, pinned, nil
}

// ghView asks gh about the PR, giving up after timeout.
func ghView(pr string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", pr, "--json", "headRefOid,statusCheckRollup")
	cmd.Stderr = &stderr
	// A child of gh that keeps the pipes open must not hold the gate past the timeout.
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("gh did not answer within %v", timeout)
	}
	if err != nil {
		return nil, fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// run is main without the exit, so tests can check what it writes and returns.
func run(stdin io.Reader, stderr io.Writer, view func(pr string) ([]byte, error)) int {
	b, err := io.ReadAll(io.LimitReader(stdin, 1<<20))
	if err != nil {
		fmt.Fprintln(stderr, "Merge blocked (#60): cannot read the hook event\n"+advice)
		return 2
	}
	d := decide(b, view)
	if d.allow {
		return 0
	}
	fmt.Fprintln(stderr, d.reason)
	return 2
}

func main() {
	os.Exit(run(os.Stdin, os.Stderr, func(pr string) ([]byte, error) { return ghView(pr, ghTimeout) }))
}
