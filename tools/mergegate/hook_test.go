package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type hookRun struct {
	code   int
	stderr string
	ran    string
}

// runHook runs hook.sh from the repository root, as Kiro does, with PATH set to bin first.
func runHook(t *testing.T, bin, stdin string) hookRun {
	t.Helper()
	cmd := exec.Command("sh", "tools/mergegate/hook.sh")
	cmd.Dir = "../.."
	cmd.Env = append(os.Environ(), "PATH="+bin+":/usr/bin:/bin")
	cmd.Stdin = strings.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	b, _ := os.ReadFile(filepath.Join(bin, "ran"))
	r := hookRun{stderr: stderr.String(), ran: string(b)}
	if ee, ok := err.(*exec.ExitError); ok {
		r.code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return r
}

// fakeGo is a `go` whose `build -o <path>` exits buildExit, or writes a gate that exits gateExit.
func fakeGo(t *testing.T, buildExit, gateExit string) string {
	t.Helper()
	bin := t.TempDir()
	script := "#!/bin/sh\necho go >> " + filepath.Join(bin, "ran") + "\n" +
		"[ " + buildExit + " = 0 ] || exit " + buildExit + "\n" +
		"printf '#!/bin/sh\\ncat >/dev/null\\nexit " + gateExit + "\\n' > \"$3\"\nchmod +x \"$3\"\n"
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestTheHookPassesACommandWithoutMergeWithoutStartingGo(t *testing.T) {
	r := runHook(t, fakeGo(t, "0", "2"), string(event(t, "gh pr view 59")))
	if r.code != 0 || r.ran != "" {
		t.Fatalf("exit %d, ran %q; want 0 and nothing run", r.code, r.ran)
	}
}

func TestTheHookAllowsWhatTheGateAllows(t *testing.T) {
	if r := runHook(t, fakeGo(t, "0", "0"), string(event(t, "gh pr merge 62"))); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
}

func TestTheHookBlocksWhatTheGateBlocks(t *testing.T) {
	if r := runHook(t, fakeGo(t, "0", "2"), string(event(t, "gh pr merge 62"))); r.code != 2 {
		t.Fatalf("exit %d", r.code)
	}
}

func TestTheHookBlocksWithAReasonWhenTheGateCannotBeBuilt(t *testing.T) {
	r := runHook(t, fakeGo(t, "1", "0"), string(event(t, "gh pr merge 62")))
	if r.code != 2 || !strings.Contains(r.stderr, "could not be built") || !strings.Contains(r.stderr, "Do not merge another way") {
		t.Fatalf("exit %d stderr %q", r.code, r.stderr)
	}
}

func TestTheHookBlocksWithAReasonWhenGoIsMissing(t *testing.T) {
	r := runHook(t, t.TempDir(), string(event(t, "gh pr merge 62")))
	if r.code != 2 || !strings.Contains(r.stderr, "could not be built") {
		t.Fatalf("exit %d stderr %q", r.code, r.stderr)
	}
}

func TestTheHookTurnsAnyOtherExitOfTheGateIntoABlock(t *testing.T) {
	for _, gateExit := range []string{"1", "3", "127"} {
		r := runHook(t, fakeGo(t, "0", gateExit), string(event(t, "gh pr merge 62")))
		if r.code != 2 || !strings.Contains(r.stderr, "stopped with exit "+gateExit) {
			t.Fatalf("gate exit %s: hook exit %d stderr %q", gateExit, r.code, r.stderr)
		}
	}
}

// End to end: the real gate, built by the real go, against a fake gh that answers with a
// recorded rollup.
func TestTheHookEndToEnd(t *testing.T) {
	goPath, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		fixture, cmd string
		code         int
		stderr       string
	}{
		{"rollup-pr62-success", "gh pr merge 62 --match-head-commit " + head62, 0, ""},
		{"rollup-pr61-failure", "gh pr merge 61 --match-head-commit " + head61, 2, "go: FAILURE"},
	} {
		bin := t.TempDir()
		fixture, _ := filepath.Abs("testdata/" + c.fixture + ".json")
		gh := "#!/bin/sh\ncat " + fixture + "\n"
		if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(gh), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(goPath, filepath.Join(bin, "go")); err != nil {
			t.Fatal(err)
		}
		r := runHook(t, bin, string(event(t, c.cmd)))
		if r.code != c.code || !strings.Contains(r.stderr, c.stderr) {
			t.Fatalf("%s: exit %d stderr %q", c.fixture, r.code, r.stderr)
		}
	}
}

func TestTheHookIsCalledByTheKiroHookDefinition(t *testing.T) {
	b, err := os.ReadFile("../../.kiro/hooks/merge-gate.json")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{`"PreToolUse"`, `"^execute_bash$"`, `sh tools/mergegate/hook.sh || exit 2`, `"timeout": 30`} {
		if !strings.Contains(s, want) {
			t.Fatalf(".kiro/hooks/merge-gate.json lacks %s", want)
		}
	}
}
