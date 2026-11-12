// Command killy is the single binary for the agent, the Killy server and the demo summoner.
package main

import (
	"fmt"
	"io"
	"os"
)

const usage = `usage: killy <command> [arguments]

commands:
  agent    watch this server for zombie processes
  server   run the Killy server
  summon   keep creating demo parents that leave zombies
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "agent":
		return runAgent(args[1:], stdout, stderr)
	case "server":
		return runServer(args[1:], stderr)
	case "summon":
		fmt.Fprintf(stderr, "killy %s: not implemented yet\n", args[0])
		return 1
	case "-h", "-help", "--help", "help":
		fmt.Fprint(stderr, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "killy: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}
