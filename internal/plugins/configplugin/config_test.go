package configplugin

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"mesos-cli/internal/config"
	"mesos-cli/internal/plugin"
)

type stubPlugin struct{ name, description string }

func (s stubPlugin) Name() string                                             { return s.name }
func (s stubPlugin) Description() string                                      { return s.description }
func (stubPlugin) Run([]string, io.Reader, io.Writer, io.Writer) (int, error) { return 0, nil }

func TestShowPrintsConfigurationWithoutExtraBlankLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	body := "plugins = []\n[master]\naddress = \"master.example.test:5050\"\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	p := New(cfg, plugin.NewRegistry())
	var out bytes.Buffer
	code, err := p.Run([]string{"show"}, bytes.NewReader(nil), &out, &bytes.Buffer{})
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if got, want := out.String(), body; got != want {
		t.Fatalf("want %q got %q", want, got)
	}
}

func TestPluginsPrintsSortedTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("plugins=[]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.Load(path)
	registry := plugin.NewRegistry()
	registry.Register(stubPlugin{"zeta", "Last plugin"})
	registry.Register(stubPlugin{"alpha", "First plugin"})
	p := New(cfg, registry)
	var out bytes.Buffer
	code, err := p.Run([]string{"plugins"}, bytes.NewReader(nil), &out, &bytes.Buffer{})
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	want := "NAME   DESCRIPTION   \nalpha  First plugin  \nzeta   Last plugin   \n"
	if out.String() != want {
		t.Fatalf("want %q got %q", want, out.String())
	}
}
