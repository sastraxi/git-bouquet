// Command git-bouquet maintains a long-lived integration branch by merging
// a curated list of feature branches into a fresh checkout of an upstream
// base, optionally running tests, and recording the result as a single
// linear commit on the integration branch.
//
// Invokable as `git bouquet` once on PATH.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/sastraxi/git-bouquet/internal/workflow"
)

const usage = `git-bouquet — keep an integration branch up to date with many feature branches

usage:
  git bouquet <command> [flags]

commands:
  start [target] [--pull] [--sync] [--dry-run]
                                        begin a rebuild for target branch
  continue                              resume after resolving a conflict
  abort                                 cancel in-progress rebuild, clean up
  status                                show progress of in-progress rebuild
  list [target]                         print the leaves that would be merged

config:
  .bouquet.yaml at repo root. See README for schema.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "start":
		fs := flag.NewFlagSet("start", flag.ExitOnError)
		var opts workflow.StartOpts
		fs.BoolVar(&opts.Pull, "pull", false, "fast-forward base and each leaf from upstream first")
		fs.BoolVar(&opts.Sync, "sync", false, "run `git town sync -s` on each leaf first")
		fs.BoolVar(&opts.DryRun, "dry-run", false, "do everything except the final commit")
		_ = fs.Parse(args)
		target := fs.Arg(0)
		err = workflow.Start(target, opts)
	case "continue":
		err = workflow.Continue()
	case "abort":
		err = workflow.Abort()
	case "status":
		err = workflow.Status()
	case "list":
		fs := flag.NewFlagSet("list", flag.ExitOnError)
		_ = fs.Parse(args)
		target := fs.Arg(0)
		err = workflow.List(target)
	case "-h", "--help", "help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n%s", cmd, usage)
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		if workflow.IsConflict(err) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}
