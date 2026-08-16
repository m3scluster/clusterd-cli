package configplugin

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"mesos-cli/internal/config"
	"mesos-cli/internal/plugin"
	"mesos-cli/internal/table"
)

type ConfigPlugin struct {
	config   *config.Config
	registry *plugin.Registry
}

func New(cfg *config.Config, registry *plugin.Registry) *ConfigPlugin {
	return &ConfigPlugin{config: cfg, registry: registry}
}
func (*ConfigPlugin) Name() string        { return "config" }
func (*ConfigPlugin) Description() string { return "Interacts with the Mesos CLI configuration file" }
func (p *ConfigPlugin) Run(args []string, _ io.Reader, stdout, _ io.Writer) (int, error) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(stdout, help)
		return 0, nil
	}
	switch args[0] {
	case "plugins":
		return p.plugins(stdout)
	case "show":
		return p.show(stdout)
	case "--version":
		fmt.Fprintln(stdout, "Mesos CLI Config Plugin")
		return 0, nil
	default:
		fmt.Fprint(stdout, help)
		return 0, nil
	}
}
func (p *ConfigPlugin) show(stdout io.Writer) (int, error) {
	body, err := os.ReadFile(p.config.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return 1, fmt.Errorf("Unable to show the config file, '%s' does not exist", p.config.Path)
		}
		return 1, fmt.Errorf("Unable to read config file '%s': %v", p.config.Path, err)
	}
	fmt.Fprintln(stdout, strings.TrimRightFunc(string(body), unicode.IsSpace))
	return 0, nil
}
func (p *ConfigPlugin) plugins(stdout io.Writer) (int, error) {
	tbl, _ := table.New([]string{"NAME", "DESCRIPTION"})
	for _, entry := range p.registry.All() {
		if err := tbl.AddRow([]string{entry.Name(), entry.Description()}); err != nil {
			return 1, err
		}
	}
	fmt.Fprintln(stdout, tbl.String())
	return 0, nil
}

const help = `Interacts with the Mesos CLI configuration file

Usage:
  mesos config (-h | --help)
  mesos config --version
  mesos config <command> (-h | --help)
  mesos config [options] <command> [<args>...]

Options:
  -h --help  Show this screen.
  --version  Show version info.

Commands:
  plugins  Print the plugins that can be used.
  show     Show the contents of the configuration file.
`
