package agentplugin

import (
	"fmt"
	"io"

	"mesos-cli/internal/mesos"
	"mesos-cli/internal/table"
)

type AgentClient interface{ Agents() ([]mesos.Agent, error) }
type AgentPlugin struct{ client AgentClient }

func New(client AgentClient) *AgentPlugin { return &AgentPlugin{client: client} }
func (*AgentPlugin) Name() string         { return "agent" }
func (*AgentPlugin) Description() string  { return "Interacts with the Mesos agents" }
func (p *AgentPlugin) Run(args []string, _ io.Reader, stdout, _ io.Writer) (int, error) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(stdout, help)
		return 0, nil
	}
	if args[0] == "--version" {
		fmt.Fprintln(stdout, "Mesos CLI Agent Plugin")
		return 0, nil
	}
	if args[0] != "list" {
		fmt.Fprint(stdout, help)
		return 0, nil
	}
	agents, err := p.client.Agents()
	if err != nil {
		return 1, fmt.Errorf("Unable to get agents: %v", err)
	}
	if len(agents) == 0 {
		fmt.Fprintln(stdout, "The cluster does not have any agents.")
		return 0, nil
	}
	tbl, _ := table.New([]string{"Agent ID", "Hostname", "Active"})
	for _, agent := range agents {
		active := "False"
		if agent.Active {
			active = "True"
		}
		if err := tbl.AddRow([]string{agent.ID, agent.Hostname, active}); err != nil {
			return 1, fmt.Errorf("Unable to build table of agents: %v", err)
		}
	}
	fmt.Fprintln(stdout, tbl.String())
	return 0, nil
}

const help = `Interacts with the Mesos agents

Usage:
  mesos agent (-h | --help)
  mesos agent --version
  mesos agent <command> (-h | --help)
  mesos agent [options] <command> [<args>...]

Options:
  -h --help  Show this screen.
  --version  Show version info.

Commands:
  list  List the Mesos agents.
`
