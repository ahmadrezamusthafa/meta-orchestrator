package packages

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"
)

// CLIUsage describes `daemon packages …`.
const CLIUsage = `usage: daemon packages <command> [id] [version]

  list                      every package with its status, version and latest release
  versions <id>             versions <id> can be installed at (newest first)
  install <id> [version]    install the latest release, or version
  update <id>               install the latest release
  rollback <id> [version]   return to version, or to the previous kept version / snapshot
  uninstall <id>            remove the package (managed copies only for CLIs)
  setup                     install every required package that is missing, dependencies first`

// RunCLI runs a packages subcommand with m, printing progress to out, and returns the exit code.
// Cancelling ctx (Ctrl-C) stops running jobs.
func RunCLI(ctx context.Context, m *Manager, args []string, out io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(out, CLIUsage)
		return 2
	}
	cmd, rest := args[0], args[1:]
	arg := func(i int) string {
		if i < len(rest) {
			return rest[i]
		}
		return ""
	}
	needID := func() bool {
		if arg(0) == "" {
			fmt.Fprintf(out, "packages %s: missing package id\n\n%s\n", cmd, CLIUsage)
			return false
		}
		return true
	}
	switch cmd {
	case "list":
		m.RefreshLatest(ctx)
		printList(out, m.List())
		return 0
	case "versions":
		if !needID() {
			return 2
		}
		vs, latest, err := m.Versions(ctx, arg(0), false)
		if err != nil {
			fmt.Fprintf(out, "error: %v\n", err)
			return 1
		}
		fmt.Fprintf(out, "latest: %s\n", orDash(latest))
		for _, v := range vs {
			fmt.Fprintln(out, v)
		}
		return 0
	case "install", "update", "rollback", "uninstall":
		if !needID() {
			return 2
		}
		var job *Job
		var err error
		switch cmd {
		case "install":
			job, err = m.Install(arg(0), arg(1))
		case "update":
			job, err = m.Update(arg(0))
		case "rollback":
			job, err = m.Rollback(arg(0), arg(1))
		case "uninstall":
			job, err = m.Uninstall(arg(0))
		}
		if err != nil {
			printErr(out, err)
			return 1
		}
		return waitJobs(ctx, m, out, []string{job.ID})
	case "setup":
		res, err := m.Setup()
		if err != nil {
			printErr(out, err)
			return 1
		}
		for _, s := range res.Skipped {
			fmt.Fprintf(out, "skip %s: %s\n", s.ID, s.Reason)
		}
		if len(res.Jobs) == 0 {
			fmt.Fprintln(out, "Nothing to install.")
			for _, s := range res.Skipped {
				if !strings.HasPrefix(s.Reason, "already installed") {
					return 1
				}
			}
			return 0
		}
		ids := make([]string, len(res.Jobs))
		for i, j := range res.Jobs {
			ids[i] = j.ID
		}
		return waitJobs(ctx, m, out, ids)
	}
	fmt.Fprintf(out, "unknown command %q\n\n%s\n", cmd, CLIUsage)
	return 2
}

func printErr(out io.Writer, err error) {
	var me *ManualError
	if errors.As(err, &me) {
		fmt.Fprintf(out, "error: %s\n", me.Reason)
		for _, c := range me.Manual.Commands {
			fmt.Fprintf(out, "  $ %s\n", c)
		}
		if me.Manual.Note != "" {
			fmt.Fprintf(out, "  %s\n", me.Manual.Note)
		}
		return
	}
	fmt.Fprintf(out, "error: %v\n", err)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func printList(out io.Writer, l Listing) {
	fmt.Fprintf(out, "home: %s\nbin:  %s (on PATH: %v)\n\n", l.Home, l.BinDir, l.OnPath)
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tKIND\tSTATUS\tVERSION\tLATEST\tMANAGED\tREQUIRED\tLOCATION")
	for _, p := range l.Packages {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%v\t%v\t%s\n", p.ID, p.Kind, p.Status, orDash(p.Version), orDash(p.Latest),
			p.Managed, p.Required, orDash(p.Location))
	}
	_ = tw.Flush()
}

// waitJobs prints each job's new log lines until all have finished; any failure exits 1.
func waitJobs(ctx context.Context, m *Manager, out io.Writer, ids []string) int {
	printed := map[string]int{}
	done := map[string]bool{}
	code := 0
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		for _, id := range ids {
			if done[id] {
				continue
			}
			j := m.Job(id)
			if j == nil {
				done[id] = true
				continue
			}
			if printed[id] > len(j.Log) {
				printed[id] = 0 // the log was trimmed
			}
			for _, l := range j.Log[printed[id]:] {
				fmt.Fprintf(out, "[%s] %s\n", j.Package, l)
			}
			printed[id] = len(j.Log)
			if j.State == JobSucceeded || j.State == JobFailed {
				done[id] = true
				fmt.Fprintf(out, "[%s] %s: %s\n", j.Package, j.State, j.Message)
				if j.State == JobFailed {
					code = 1
				}
			}
		}
		if len(done) == len(ids) {
			return code
		}
		select {
		case <-ctx.Done():
			m.Close()
			fmt.Fprintln(out, "cancelled")
			return 130
		case <-tick.C:
		}
	}
}
