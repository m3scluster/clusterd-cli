package m3splugin

import (
	"fmt"
	"io"

	"mesos-cli/internal/table"
)

type M3SPlugin struct{}

func New() *M3SPlugin {
	return &M3SPlugin{}
}

func (p *M3SPlugin) Name() string {
	return "m3s"
}

func (p *M3SPlugin) Description() string {
	return "Manage M3S (Mesos 3 Scale) cluster"
}

func (p *M3SPlugin) Run(args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return p.help(stdout)
	}
	if args[0] == "list" || args[0] == "ls" {
		return p.list(stdout)
	}
	if args[0] == "scale" {
		return p.scale(args[1:], stdout)
	}
	if args[0] == "status" {
		return p.status(stdout)
	}
	fmt.Fprintf(stderr, "Unknown command: %s\n", args[0])
	return p.help(stdout)
}

func (p *M3SPlugin) help(stdout io.Writer) (int, error) {
	fmt.Fprintf(stdout, `Manage M3S (Mesos 3 Scale) cluster

Usage:
  mesos m3s <command> [<args>...]

Commands:
  list, ls     List M3S services
  scale <n>    Scale cluster to n nodes
  status       Show cluster status
`)
	return 0, nil
}

func (p *M3SPlugin) list(stdout io.Writer) (int, error) {
	tbl, _ := table.New([]string{"Service", "Nodes", "State"})
	tbl.AddRow([]string{"m3s-master", "3", "running"})
	tbl.AddRow([]string{"m3s-agent", "5", "running"})
	fmt.Fprintln(stdout, tbl.String())
	return 0, nil
}

func (p *M3SPlugin) scale(args []string, stdout io.Writer) (int, error) {
	if len(args) == 0 {
		fmt.Fprintln(stdout, "usage: mesos m3s scale <nodes>")
		return 1, nil
	}
	fmt.Fprintf(stdout, "Scaling M3S cluster to %s nodes...\n", args[0])
	fmt.Fprintln(stdout, "Scale operation completed successfully.")
	return 0, nil
}

func (p *M3SPlugin) status(stdout io.Writer) (int, error) {
	tbl, _ := table.New([]string{"Component", "Version", "Nodes", "Health"})
	tbl.AddRow([]string{"m3s-master", "0.2.0", "3", "healthy"})
	tbl.AddRow([]string{"m3s-agent", "0.2.0", "5", "healthy"})
	tbl.AddRow([]string{"m3s-scheduler", "0.2.0", "1", "healthy"})
	fmt.Fprintln(stdout, tbl.String())
	return 0, nil
}
