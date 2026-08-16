package plugin

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadExternalPluginManifestAndRun(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "synthetic-plugin")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'ARGS:%s' \"$*\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	manifest := `name = "synthetic"
description = "Synthetic test plugin"
executable = "synthetic-plugin"
`
	if err := os.WriteFile(filepath.Join(dir, "plugin.toml"), []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := LoadExternal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "synthetic" || p.Description() != "Synthetic test plugin" {
		t.Fatalf("metadata mismatch")
	}
	var out bytes.Buffer
	code, err := p.Run([]string{"one", "two"}, bytes.NewBuffer(nil), &out, &bytes.Buffer{})
	if err != nil || code != 0 || out.String() != "ARGS:one two" {
		t.Fatalf("code=%d err=%v out=%q", code, err, out.String())
	}
}

func TestRegistryRejectsDuplicateNames(t *testing.T) {
	r := NewRegistry()
	p := stubPlugin{name: "duplicate"}
	if err := r.Register(p); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(p); err == nil {
		t.Fatal("expected duplicate error")
	}
}

type stubPlugin struct{ name string }

func (p stubPlugin) Name() string                                             { return p.name }
func (stubPlugin) Description() string                                        { return "stub" }
func (stubPlugin) Run([]string, io.Reader, io.Writer, io.Writer) (int, error) { return 0, nil }
