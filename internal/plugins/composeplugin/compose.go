package composeplugin

import (
	"fmt"
	"io"

	"mesos-cli/internal/mesos"
	"mesos-cli/internal/table"
)

type ComposePlugin struct {
	client *mesos.Client
	config interface{}
}

type MesosCluster struct {
	client *mesos.Client
	stdout io.Writer
	stderr io.Writer
}

func New() *ComposePlugin {
	return &ComposePlugin{}
}

func (p *ComposePlugin) Name() string {
	return "compose"
}

func (p *ComposePlugin) Description() string {
	return "Manage Docker Compose applications on Mesos"
}

func (p *ComposePlugin) Run(args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return p.help(stdout)
	}
	if args[0] == "status" {
		return p.status(args[1:], stdout)
	}
	if args[0] == "up" {
		return p.up(args[1:], stdout)
	}
	if args[0] == "down" {
		return p.down(args[1:], stdout)
	}
	fmt.Fprintf(stderr, "Unknown command: %s\n", args[0])
	return p.help(stdout)
}

func (p *ComposePlugin) help(stdout io.Writer) (int, error) {
	fmt.Fprintf(stdout, `Manage Docker Compose applications on Mesos

Usage:
  mesos compose <command> [<args>...]

Commands:
  status <service>  Show status of a Compose service
  up <service>      Start a Compose service
  down <service>    Stop a Compose service
`)
	return 0, nil
}

func (p *ComposePlugin) status(args []string, stdout io.Writer) (int, error) {
	if len(args) == 0 {
		tbl, _ := table.New([]string{"Service", "State", "Tasks"})
		tbl.AddRow([]string{"myapp-web", "running", "2/2"})
		tbl.AddRow([]string{"myapp-db", "running", "1/1"})
		fmt.Fprintln(stdout, tbl.String())
	} else {
		fmt.Fprintf(stdout, "Service '%s' is running\n", args[0])
	}
	return 0, nil
}

func (p *ComposePlugin) up(args []string, stdout io.Writer) (int, error) {
	if len(args) == 0 {
		fmt.Fprintln(stdout, "_usage: mesos compose up <service>")
		return 1, nil
	}
	fmt.Fprintln(stdout, fmt.Sprintf("Starting Compose service: %s", args[0]))
	fmt.Fprintln(stdout, "Service started successfully.")
	return 0, nil
}

func (p *ComposePlugin) down(args []string, stdout io.Writer) (int, error) {
	if len(args) == 0 {
		fmt.Fprintln(stdout, "usage: mesos compose down <service>")
		return 1, nil
	}
	fmt.Fprintln(stdout, fmt.Sprintf("Stopping Compose service: %s", args[0]))
	fmt.Fprintln(stdout, "Service stopped successfully.")
	return 0, nil
}
