package frameworkplugin

import (
	"encoding/json"
	"fmt"
	"io"
	"mesos-cli/internal/mesos"
	"mesos-cli/internal/table"
	"slices"
)

type FrameworkClient interface {
	Frameworks() ([]mesos.Framework, error)
}
type FrameworkPlugin struct{ client FrameworkClient }

func New(client FrameworkClient) *FrameworkPlugin { return &FrameworkPlugin{client: client} }
func (*FrameworkPlugin) Name() string             { return "framework" }
func (*FrameworkPlugin) Description() string      { return "Interacts with the Mesos Frameworks" }
func (p *FrameworkPlugin) Run(args []string, _ io.Reader, stdout, _ io.Writer) (int, error) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(stdout, help)
		return 0, nil
	}
	if args[0] == "--version" {
		fmt.Fprintln(stdout, "v0.1.0")
		return 0, nil
	}
	switch args[0] {
	case "list":
		return p.list(slices.Contains(args[1:], "--all") || slices.Contains(args[1:], "-a"), stdout)
	case "inspect":
		if len(args) < 2 {
			return 1, fmt.Errorf("missing <framework_id>")
		}
		return p.inspect(args[1], stdout)
	default:
		fmt.Fprint(stdout, help)
		return 0, nil
	}
}
func (p *FrameworkPlugin) list(all bool, stdout io.Writer) (int, error) {
	items, err := p.client.Frameworks()
	if err != nil {
		return 1, err
	}
	tbl, _ := table.New([]string{"ID", "Active", "Hostname", "Name"})
	for _, item := range items {
		if !all && !item.Active {
			continue
		}
		active := "False"
		if item.Active {
			active = "True"
		}
		if err := tbl.AddRow([]string{item.ID, active, item.Hostname, item.Name}); err != nil {
			return 1, err
		}
	}
	fmt.Fprintln(stdout, tbl.String())
	return 0, nil
}
func (p *FrameworkPlugin) inspect(id string, stdout io.Writer) (int, error) {
	items, err := p.client.Frameworks()
	if err != nil {
		return 1, err
	}
	for _, item := range items {
		if item.ID != id {
			continue
		}
		raw := make(map[string]any, len(item.Raw))
		for key, value := range item.Raw {
			raw[key] = value
		}
		delete(raw, "tasks")
		delete(raw, "unreachable_tasks")
		delete(raw, "completed_tasks")
		data, err := json.MarshalIndent(raw, "", "    ")
		if err != nil {
			return 1, err
		}
		fmt.Fprintln(stdout, string(data))
		break
	}
	return 0, nil
}

const help = `Interacts with the Mesos Frameworks

Usage:
  mesos framework (-h | --help)
  mesos framework --version
  mesos framework <command> (-h | --help)
  mesos framework [options] <command> [<args>...]

Options:
  -h --help  Show this screen.
  --version  Show version info.

Commands:
  inspect  Return low-level information on the framework.
  list     List the Mesos frameworks.
`
