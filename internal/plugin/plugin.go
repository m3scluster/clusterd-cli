package plugin

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"
)

type Plugin interface {
	Name() string
	Description() string
	Run(args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error)
}

type Registry struct{ plugins map[string]Plugin }

func NewRegistry() *Registry { return &Registry{plugins: map[string]Plugin{}} }
func (r *Registry) Register(p Plugin) error {
	if _, ok := r.plugins[p.Name()]; ok {
		return fmt.Errorf("plugin '%s' is already registered", p.Name())
	}
	r.plugins[p.Name()] = p
	return nil
}
func (r *Registry) Get(name string) (Plugin, bool) { p, ok := r.plugins[name]; return p, ok }
func (r *Registry) All() []Plugin {
	names := make([]string, 0, len(r.plugins))
	for name := range r.plugins {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]Plugin, 0, len(names))
	for _, name := range names {
		result = append(result, r.plugins[name])
	}
	return result
}

type externalManifest struct {
	Name        string `toml:"name"`
	Description string `toml:"description"`
	Executable  string `toml:"executable"`
}
type externalPlugin struct {
	manifest   externalManifest
	executable string
	directory  string
}

func (p *externalPlugin) Name() string        { return p.manifest.Name }
func (p *externalPlugin) Description() string { return p.manifest.Description }
func (p *externalPlugin) Run(args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	cmd := exec.Command(p.executable, args...)
	cmd.Dir = p.directory
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = os.Environ()
	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	if exit, ok := err.(*exec.ExitError); ok {
		return exit.ExitCode(), nil
	}
	return 1, err
}
func LoadExternal(directory string) (Plugin, error) {
	var manifest externalManifest
	if _, err := toml.DecodeFile(filepath.Join(directory, "plugin.toml"), &manifest); err != nil {
		return nil, fmt.Errorf("unable to load plugin manifest '%s': %v", directory, err)
	}
	if manifest.Name == "" || manifest.Description == "" || manifest.Executable == "" {
		return nil, fmt.Errorf("plugin manifest '%s' must define name, description, and executable", directory)
	}
	executable := manifest.Executable
	if !filepath.IsAbs(executable) {
		executable = filepath.Join(directory, executable)
	}
	info, err := os.Stat(executable)
	if err != nil {
		return nil, fmt.Errorf("plugin executable not found: %s", executable)
	}
	if info.Mode()&0111 == 0 {
		return nil, fmt.Errorf("plugin executable is not executable: %s", executable)
	}
	return &externalPlugin{manifest: manifest, executable: executable, directory: directory}, nil
}
