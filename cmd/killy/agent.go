package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/sumiredc/killy/internal/agent"
)

const agentUsage = `usage: killy agent <command> [flags]

commands:
  pair     register this server with the Killy server and show a pairing code
`

func runAgent(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, agentUsage)
		return 2
	}
	switch args[0] {
	case "pair":
		return runAgentPair(args[1:], stdout, stderr)
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
