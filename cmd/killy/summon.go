package main

import (
	"context"
	"flag"
	"io"
	"log/slog"
	"math/rand/v2"
	"os"
	"os/signal"
	"syscall"

	"github.com/sumiredc/killy/internal/summon"
)

// runSummon keeps up to summon.MaxParents demo parents alive: one at start, then one more every
// summon.Interval while fewer are alive. Stopping it leaves the parents to systemd, which stops
// the whole cgroup with the service.
func runSummon(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("killy summon", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	log := slog.New(slog.NewTextHandler(stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	s := &summon.Summoner{Start: summon.ExecStart, Pick: rand.IntN, Log: log}
	log.Info("summoning demo parents", "max", summon.MaxParents, "interval", summon.Interval)
	s.Run(ctx, every(ctx, summon.Interval))
	return 0
}
