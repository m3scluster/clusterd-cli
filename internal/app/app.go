package app

import (
	"fmt"
	"io"
	"mesos-cli/internal/config"
	"mesos-cli/internal/mesos"
	"mesos-cli/internal/plugin"
	"mesos-cli/internal/plugins/agentplugin"
	"mesos-cli/internal/plugins/configplugin"
	"mesos-cli/internal/plugins/frameworkplugin"
	"mesos-cli/internal/plugins/taskplugin"
	"net/http"
	"sort"
	"strings"
)

type App struct{ registry *plugin.Registry }

func New(cfg *config.Config, httpClient *http.Client) (*App, error) {
	registry := plugin.NewRegistry()
	client := mesos.NewClient(cfg, httpClient)
	builtins := []plugin.Plugin{agentplugin.New(client), configplugin.New(cfg, registry), frameworkplugin.New(client), taskplugin.New(client, cfg, httpClient)}
	for _, entry := range builtins {
		if err := registry.Register(entry); err != nil {
			return nil, err
		}
	}
	paths, err := cfg.PluginPaths()
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		entry, err := plugin.LoadExternal(path)
		if err != nil {
			return nil, err
		}
		if err := registry.Register(entry); err != nil {
			return nil, err
		}
	}
	return &App{registry: registry}, nil
}
func (a *App) Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(stdout, a.help())
		return 0
	}
	if args[0] == "--version" {
		fmt.Fprintln(stdout, "Mesos Development CLI")
		return 0
	}
	if args[0] == "__autocomplete__" {
		a.autocomplete(args[1:], stdout)
		return 0
	}
	if args[0] == "help" {
		if len(args) > 1 {
			if entry, ok := a.registry.Get(args[1]); ok {
				code, err := entry.Run([]string{"--help"}, stdin, stdout, stderr)
				return finish(code, err, stderr)
			}
		}
		fmt.Fprint(stdout, a.help())
		return 0
	}
	entry, ok := a.registry.Get(args[0])
	if !ok {
		fmt.Fprint(stdout, a.help())
		return 0
	}
	code, err := entry.Run(args[1:], stdin, stdout, stderr)
	return finish(code, err, stderr)
}
func finish(code int, err error, stderr io.Writer) int {
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v.\n", err)
		return 1
	}
	return code
}
func (a *App) help() string {
	commands := a.registry.All()
	width := 0
	for _, entry := range commands {
		if len(entry.Name()) > width {
			width = len(entry.Name())
		}
	}
	var rows strings.Builder
	for _, entry := range commands {
		fmt.Fprintf(&rows, "  %s%s%s\n", entry.Name(), strings.Repeat(" ", width-len(entry.Name())+2), entry.Description())
	}
	return fmt.Sprintf(`Mesos CLI

Usage:
  mesos (-h | --help)
  mesos --version
  mesos <command> [<args>...]

Options:
  -h --help  Show this screen.
  --version  Show version info.

Commands:
%sSee 'mesos help <command>' for more information on a specific command.
`, rows.String())
}
func(a *App) autocomplete(args []string, stdout io.Writer) {
	if len(args) > 1 {
		if entry, ok := a.registry.Get(args[1]); ok {
			var buf bytes.Buffer
			entry.Run([]string{"--help"}, nil, &buf, &bytes.Buffer{})
			words := []string{"-h", "--help"}
			for _, line := range strings.Split(buf.String(), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "Usage:") || strings.HasPrefix(line, "Options:") || strings.HasPrefix(line, "Commands:") || strings.HasPrefix(line, "Interacts with") {
					continue
				}
				fields := strings.Fields(line)
				if len(fields) > 0 && !strings.HasPrefix(fields[0], "-") {
					words = append(words, fields[0])
				}
			}
			sort.Strings(words)
			current := ""
			if len(args) > 0 {
				current = args[0]
			}
			matches := []string{}
			for _, word := range words {
				if strings.HasPrefix(word, current) {
					matches = append(matches, word)
				}
			}
			fmt.Fprintln(stdout, "default")
			fmt.Fprintln(stdout, strings.Join(matches, " "))
			return
		}
	}
	words := []string{"help"}
	for _, entry := range a.registry.All() {
		words = append(words, entry.Name())
	}
	sort.Strings(words)
	current := ""
	if len(args) > 0 {
		current = args[0]
	}
	matches := []string{}
	for _, word := range words {
		if strings.HasPrefix(word, current) {
			matches = append(matches, word)
		}
	}
	fmt.Fprintln(stdout, "default")
	fmt.Fprintln(stdout, strings.Join(matches, " "))
}
