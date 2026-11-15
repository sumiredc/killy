package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sumiredc/killy/internal/agent"
	"github.com/sumiredc/killy/internal/agent/kill"
	"github.com/sumiredc/killy/internal/proc"
	"github.com/sumiredc/killy/internal/version"
)

const agentUsage = `usage: killy agent <command> [flags]

commands:
  pair     register this server with the Killy server and show a pairing code
  run      report zombie processes to the Killy server (the long-running service)
`

func runAgent(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, agentUsage)
		return 2
	}
	switch args[0] {
	case "pair":
		return runAgentPair(args[1:], stdout, stderr)
	case "run":
		return runAgentRun(args[1:], stderr)
	default:
		fmt.Fprintf(stderr, "killy agent: unknown command %q\n\n%s", args[0], agentUsage)
		return 2
	}
}

func runAgentPair(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("killy agent pair", flag.ContinueOnError)
	fs.SetOutput(stderr)
	server := fs.String("server", "", "URL of the Killy server (required the first time)")
	config := fs.String("config", agent.DefaultConfigPath, "agent configuration file")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	hostname, err := os.Hostname()
	if err != nil {
		fmt.Fprintf(stderr, "killy agent pair: hostname: %v\n", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	opts := agent.PairOptions{
		ConfigPath: *config,
		Server:     *server,
		Hostname:   hostname,
		Client:     &http.Client{Timeout: 30 * time.Second},
	}
	if err := agent.Pair(ctx, opts, stdout); err != nil {
		fmt.Fprintf(stderr, "killy agent pair: %v\n", err)
		return 1
	}
	return 0
}

func runAgentRun(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("killy agent run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	config := fs.String("config", agent.DefaultConfigPath, "agent configuration file")
	outcomes := fs.String("outcomes", "/var/lib/killy/outcomes.jsonl", "file that keeps the outcome of every kill ID")
	demo := fs.Bool("demo", false, "public demo: report and kill only parents started by killy summon")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	log := slog.New(slog.NewTextHandler(stderr, nil))
	store, err := kill.OpenFileStore(*outcomes)
	if err != nil {
		log.Error("open kill outcomes", "err", err)
		return 1
	}
	defer store.Close()
	scanner, killer := newAgentParts(*demo, os.DirFS("/proc"), store, log)
	if *demo {
		log.Info("demo mode: only parents in the cgroup of killy-summon.service are reported and killed", "cgroup", proc.SummonCgroup)
	}
	hostname, err := os.Hostname()
	if err != nil {
		log.Error("hostname", "err", err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	opts := agent.RunOptions{
		ConfigPath:     *config,
		Client:         &http.Client{Timeout: 10 * time.Second},
		Log:            log,
		Hostname:       hostname,
		Version:        version.Version,
		Scanner:        scanner,
		PidfdAvailable: func() bool { return kill.Available(kill.NewPidfdOps(), os.Getpid()) },
		Ticks:          every(ctx, agent.ScanInterval),
		Killer:         killer,
		// No overall timeout: the command stream stays open for hours.
		StreamClient: &http.Client{},
		Rand:         rand.Float64,
		Sleep:        agent.SleepContext,
	}
	if err := agent.Run(ctx, opts); err != nil {
		fmt.Fprintf(stderr, "killy agent run: %v\n", err)
		return 1
	}
	return 0
}

// newAgentParts builds the scanner and the killer. With demo, both are limited to killy summon's
// cgroup: the scanner reports nothing else and the killer kills nothing else. They are set together
// here so that what the public demo shows and what it can kill never differ.
func newAgentParts(demo bool, root fs.FS, store kill.OutcomeStore, log *slog.Logger) (*proc.Scanner, *kill.Killer) {
	scanner := &proc.Scanner{Root: root}
	killer := &kill.Killer{Proc: scanner, Pidfd: kill.NewPidfdOps(), Store: store, Clock: kill.SystemClock{}, SelfPID: os.Getpid, Log: log, Demo: demo}
	if demo {
		scanner.OnlyCgroup = proc.SummonCgroup
	}
	return scanner, killer
}

// every sends the current time at once and then every d until ctx is done.
func every(ctx context.Context, d time.Duration) <-chan time.Time {
	ch := make(chan time.Time)
	go func() {
		t := time.NewTicker(d)
		defer t.Stop()
		now := time.Now()
		for {
			select {
			case ch <- now:
			case <-ctx.Done():
				return
			}
			select {
			case now = <-t.C:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
}
