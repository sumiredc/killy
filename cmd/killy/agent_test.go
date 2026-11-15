package main

import (
	"io"
	"log/slog"
	"testing"
	"testing/fstest"

	"github.com/sumiredc/killy/internal/proc"
)

func TestDemoLimitsBothTheReportAndTheKillsToSummonsCgroup(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	scanner, killer := newAgentParts(true, fstest.MapFS{}, nil, log)
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
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	scanner, killer := newAgentParts(false, fstest.MapFS{}, nil, log)
	if scanner.OnlyCgroup != "" || killer.Demo {
		t.Errorf("OnlyCgroup %q, Demo %v; want neither set", scanner.OnlyCgroup, killer.Demo)
	}
}
