package main

import (
	"io"
	"log/slog"
	"testing"
	"testing/fstest"

	"github.com/sumiredc/killy/internal/agent/kill"
	"github.com/sumiredc/killy/internal/proc"
)

// parts builds the agent's scanner and killer as killy agent run does with args.
func parts(t *testing.T, args ...string) (*proc.Scanner, *kill.Killer) {
	t.Helper()
	flags, err := parseAgentRunFlags(args, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	return newAgentParts(flags, fstest.MapFS{}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestDemoLimitsBothTheReportAndTheKillsToSummonsCgroup(t *testing.T) {
	scanner, killer := parts(t, "--demo")
	if scanner.OnlyCgroup != proc.SummonCgroup {
		t.Errorf("scanner.OnlyCgroup = %q, want %q", scanner.OnlyCgroup, proc.SummonCgroup)
	}
	if !killer.Demo {
		t.Error("killer.Demo = false, want true")
	}
	if killer.Proc != scanner {
		t.Error("the killer reads /proc through another scanner")
	}
}

func TestWithoutDemoNeitherIsLimited(t *testing.T) {
	scanner, killer := parts(t)
	if scanner.OnlyCgroup != "" || killer.Demo {
		t.Errorf("OnlyCgroup %q, Demo %v; want neither set", scanner.OnlyCgroup, killer.Demo)
	}
}
