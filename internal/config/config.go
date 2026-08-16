package config

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/go-zookeeper/zk"
)

type Config struct {
	Path string
	data fileConfig
}

type fileConfig struct {
	Plugins []string     `toml:"plugins"`
	Master  masterConfig `toml:"master"`
	Agent   agentConfig  `toml:"agent"`
}

type masterConfig struct {
	Address   string           `toml:"address"`
	Principal string           `toml:"principal"`
	Secret    string           `toml:"secret"`
	SSLVerify *bool            `toml:"ssl_verify"`
	Zookeeper *zookeeperConfig `toml:"zookeeper"`
}

type zookeeperConfig struct {
	Addresses []string `toml:"addresses"`
	Path      string   `toml:"path"`
}

type agentConfig struct {
	SSL       *bool  `toml:"ssl"`
	SSLVerify *bool  `toml:"ssl_verify"`
	Timeout   *int   `toml:"timeout"`
	Principal string `toml:"principal"`
	Secret    string `toml:"secret"`
}

func Load(explicitPath string) (*Config, error) {
	path := explicitPath
	if envPath := os.Getenv("MESOS_CLI_CONFIG"); envPath != "" {
		path = envPath
	}
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(home, ".mesos", "config.toml")
	}
	var data fileConfig
	if _, err := toml.DecodeFile(path, &data); err != nil {
		return nil, fmt.Errorf("Error loading config file as TOML: %v", err)
	}
	return &Config{Path: path, data: data}, nil
}

func SanitizeAddress(address string) (string, error) {
	if !strings.Contains(address, "://") {
		address = "http://" + address
	}
	parsed, err := url.Parse(address)
	if err != nil {
		return "", fmt.Errorf("Unable to parse address: %v", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("Invalid scheme '%s' in address", parsed.Scheme)
	}
	if parsed.Hostname() == "" {
		return "", fmt.Errorf("Missing hostname in address")
	}
	if ip := net.ParseIP(parsed.Hostname()); ip != nil {
		if ip.To4() == nil {
			return "", fmt.Errorf("IPv6 addresses are unsupported")
		}
		if parsed.Port() == "" {
			return "", fmt.Errorf("Addresses formatted as IP must contain a port")
		}
	}
	return strings.TrimRight(address, "/"), nil
}

func (c *Config) Master() (string, error) {
	master := c.data.Master
	if master.Address != "" && master.Zookeeper != nil {
		return "", fmt.Errorf("The 'master' field should only contain  an 'address' field or a 'zookeeper' dictionary but not both")
	}
	if master.Address != "" {
		address, err := SanitizeAddress(master.Address)
		if err != nil {
			return "", fmt.Errorf("The 'master' address %s is formatted incorrectly: %v", master.Address, err)
		}
		return address, nil
	}
	if master.Zookeeper != nil {
		return resolveZookeeper(master.Zookeeper)
	}
	if master == (masterConfig{}) {
		return "127.0.0.1:5050", nil
	}
	return "", fmt.Errorf("The 'master' field must either contain an 'address' field or a 'zookeeper' dictionary")
}

func resolveZookeeper(field *zookeeperConfig) (string, error) {
	if len(field.Addresses) == 0 {
		return "", fmt.Errorf("The 'zookeeper' field must contain an 'addresses' list")
	}
	if !strings.HasPrefix(field.Path, "/") {
		return "", fmt.Errorf("The 'zookeeper' field 'path' must start with a '/'")
	}
	if field.Path == "/" {
		return "", fmt.Errorf("The 'zookeeper' field 'path' should be nested ('/' is not supported)")
	}
	conn, _, err := zk.Connect(field.Addresses, 5*time.Second)
	if err != nil {
		return "", fmt.Errorf("Could not resolve the leading master: %v", err)
	}
	defer conn.Close()
	children, _, err := conn.Children(field.Path)
	if err != nil {
		return "", fmt.Errorf("Unable to get children of %s: %v", field.Path, err)
	}
	sort.Strings(children)
	for _, child := range children {
		if !strings.HasPrefix(child, "json.info") {
			continue
		}
		value, _, err := conn.Get(strings.TrimRight(field.Path, "/") + "/" + child)
		if err != nil {
			return "", fmt.Errorf("Unable to get the value of '%s': %v", child, err)
		}
		var info struct {
			Address struct {
				IP   string `json:"ip"`
				Port int    `json:"port"`
			} `json:"address"`
		}
		if err := json.Unmarshal(value, &info); err != nil {
			return "", fmt.Errorf("Could not load JSON from '%s': %v", value, err)
		}
		if info.Address.IP != "" && info.Address.Port != 0 {
			return fmt.Sprintf("%s:%d", info.Address.IP, info.Address.Port), nil
		}
	}
	return "", fmt.Errorf("Unable to resolve the leading master using ZooKeeper")
}

func (c *Config) Principal() string { return c.data.Master.Principal }
func (c *Config) Secret() string    { return c.data.Master.Secret }
func (c *Config) MasterSSLVerify() bool {
	return c.data.Master.SSLVerify != nil && *c.data.Master.SSLVerify
}
func (c *Config) AgentSSL() bool { return c.data.Agent.SSL != nil && *c.data.Agent.SSL }
func (c *Config) AgentSSLVerify() bool {
	return c.data.Agent.SSLVerify != nil && *c.data.Agent.SSLVerify
}
func (c *Config) AgentTimeout() int {
	if c.data.Agent.Timeout != nil {
		return *c.data.Agent.Timeout
	}
	return 5
}
func (c *Config) AgentAuthentication() (string, string, bool) {
	return c.data.Agent.Principal, c.data.Agent.Secret, c.data.Agent.Principal != "" && c.data.Agent.Secret != ""
}
func (c *Config) PluginPaths() ([]string, error) {
	for _, path := range c.data.Plugins {
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("Plugin path not found: %s", path)
		}
	}
	return append([]string(nil), c.data.Plugins...), nil
}
