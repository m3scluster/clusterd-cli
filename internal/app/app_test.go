package app

import (
	"bytes"
	"mesos-cli/internal/config"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func configFor(t *testing.T, master string, plugins []string) *config.Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	var body strings.Builder
	if len(plugins) > 0 {
		body.WriteString("plugins = [\n")
		for _, p := range plugins {
			body.WriteString("  \"")
			body.WriteString(p)
			body.WriteString("\",\n")
		}
		body.WriteString("]\n")
	}
	body.WriteString("[master]\naddress = \"")
	body.WriteString(master)
	body.WriteString("\"\n")
	if err := os.WriteFile(path, []byte(body.String()), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
func TestTopHelpListsBuiltinsInPythonOrderFormatting(t *testing.T) {
	cfg := configFor(t, "master.example.test:5050", nil)
	application, err := New(cfg, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := application.Run([]string{"--help"}, bytes.NewReader(nil), &out, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	for _, line := range []string{"agent      Interacts with the Mesos agents", "config     Interacts with the Mesos CLI configuration file", "framework  Interacts with the Mesos Frameworks", "task       Interacts with the tasks running in a Mesos cluster"} {
		if !strings.Contains(out.String(), line) {
			t.Fatalf("missing %q in %q", line, out.String())
		}
	}
}
func TestAgentCommandReachesMaster(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"slaves":[]}`))
	}))
	defer server.Close()
	cfg := configFor(t, server.URL, nil)
	application, err := New(cfg, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := application.Run([]string{"agent", "list"}, bytes.NewReader(nil), &out, &bytes.Buffer{})
	if code != 0 || out.String() != "The cluster does not have any agents.\n" {
		t.Fatalf("code=%d out=%q", code, out.String())
	}
}
func TestExternalPluginFromConfigRunsUnchangedArguments(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "run")
	os.WriteFile(script, []byte("#!/bin/sh\nprintf 'external:%s' \"$*\"\n"), 0700)
	os.WriteFile(filepath.Join(dir, "plugin.toml"), []byte("name='synthetic'\ndescription='Synthetic plugin'\nexecutable='run'\n"), 0600)
	cfg := configFor(t, "master.example.test:5050", []string{dir})
	application, err := New(cfg, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := application.Run([]string{"synthetic", "one", "two"}, bytes.NewReader(nil), &out, &bytes.Buffer{})
	if code != 0 || out.String() != "external:one two" {
		t.Fatalf("code=%d out=%q", code, out.String())
	}
}
