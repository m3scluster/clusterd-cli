package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadUsesMESOSCLIConfig(t *testing.T) {
	path := writeConfig(t, `[master]
address = "master.example.test:5050"
principal = "synthetic-user"
secret = "synthetic-secret"
ssl_verify = true
[agent]
ssl = true
ssl_verify = true
timeout = 7
principal = "agent-user"
secret = "agent-secret"
plugins = []
`)
	t.Setenv("MESOS_CLI_CONFIG", path)
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Path != path {
		t.Fatalf("path: %q", cfg.Path)
	}
	master, err := cfg.Master()
	if err != nil || master != "http://master.example.test:5050" {
		t.Fatalf("master=%q err=%v", master, err)
	}
	if cfg.Principal() != "synthetic-user" || cfg.Secret() != "synthetic-secret" {
		t.Fatal("master credentials mismatch")
	}
	if !cfg.MasterSSLVerify() || !cfg.AgentSSL() || !cfg.AgentSSLVerify() || cfg.AgentTimeout() != 7 {
		t.Fatal("agent/master options mismatch")
	}
	user, secret, ok := cfg.AgentAuthentication()
	if !ok || user != "agent-user" || secret != "agent-secret" {
		t.Fatal("agent auth mismatch")
	}
}

func TestMasterRejectsAddressAndZookeeperTogether(t *testing.T) {
	path := writeConfig(t, `[master]
address = "master.example.test:5050"
[master.zookeeper]
addresses = ["zk.example.test:2181"]
path = "/mesos"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.Master(); err == nil || err.Error() != "The 'master' field should only contain  an 'address' field or a 'zookeeper' dictionary but not both" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPluginsRejectMissingPath(t *testing.T) {
	path := writeConfig(t, `plugins = ["/synthetic/path/that/does/not/exist"]
[master]
address = "master.example.test:5050"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.PluginPaths(); err == nil {
		t.Fatal("expected missing plugin error")
	}
}
