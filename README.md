# Go Mesos CLI

Go Mesos CLI is a command-line client for Apache Mesos, implemented in Go and broadly compatible with the command model of the Python `mesos-cli`. It includes built-in commands for agents, frameworks, tasks, Mesos Compose, and M3S, plus support for external executable plugins.

Apache Mesos is a project of the Apache Software Foundation.

## Features

- Built-in `agent`, `framework`, `task`, `config`, `compose`, and `m3s` command groups
- Tabular output for agent, framework, task, Compose task, and M3S framework lists
- Direct master connections or leader discovery through ZooKeeper
- Master, agent, and per-framework Basic Authentication
- Configurable TLS certificate verification
- Shell completion support through `__autocomplete__`
- External executable plugins loaded from TOML manifests

## Requirements

- Go 1.26.5 or newer in the Go 1.26 release series
- GNU Make

## Build

```bash
make all
```

The resulting binary is written to `./mesos-cli`.

To display the available command groups:

```bash
./mesos-cli --help
```

## Configuration

Set `MESOS_CLI_CONFIG` to select a configuration file. If the variable is not set, the CLI reads `~/.mesos/config.toml`.

The following example uses synthetic hosts and credentials:

```toml
[master]
address = "https://master.example.test:5050"
principal = "synthetic-master-user"
secret = "synthetic-master-secret"
ssl_verify = true

[agent]
ssl = true
ssl_verify = true
timeout = 10
principal = "synthetic-agent-user"
secret = "synthetic-agent-secret"

[compose.synthetic-compose]
principal = "synthetic-compose-user"
secret = "synthetic-compose-secret"
ssl_verify = true

[m3s.synthetic-m3s]
principal = "synthetic-m3s-user"
secret = "synthetic-m3s-secret"
ssl_verify = true
```

Framework credential section names must match the framework name supplied to the command.

### ZooKeeper leader discovery

ZooKeeper discovery can be used instead of `master.address`. Do not configure both methods at the same time.

```toml
[master.zookeeper]
addresses = ["zk-1.example.test:2181", "zk-2.example.test:2181"]
path = "/mesos"
```

## Core commands

```bash
./mesos-cli agent list
./mesos-cli framework list --all
./mesos-cli framework inspect synthetic-framework-id
./mesos-cli task list --all
./mesos-cli task inspect synthetic-task-id
./mesos-cli config show
./mesos-cli config plugins
```

Use `./mesos-cli help <command>` for command-specific help.

## Mesos Compose

Compose is built into the binary; no external plugin manifest is required.

```bash
./mesos-cli compose version synthetic-compose
./mesos-cli compose info synthetic-compose
./mesos-cli compose list synthetic-compose
./mesos-cli compose launch synthetic-compose synthetic-project compose.yaml
./mesos-cli compose update synthetic-compose synthetic-project compose.yaml
./mesos-cli compose kill synthetic-compose synthetic-task-id
./mesos-cli compose kill synthetic-compose prefix:synthetic-project:synthetic-service
./mesos-cli compose restart synthetic-compose prefix:synthetic-project:synthetic-service
./mesos-cli compose framework synthetic-compose reregister
./mesos-cli compose framework synthetic-compose suppress
```

`launch` and `update` send the selected Compose file to the framework API. `kill` accepts either a task ID or a service name. Service names use the `<prefix>:<project>:<service>` form.

## M3S

M3S is also built into the binary.

```bash
./mesos-cli m3s list
./mesos-cli m3s list --all
./mesos-cli m3s kubeconfig synthetic-m3s
./mesos-cli m3s version synthetic-m3s
./mesos-cli m3s status synthetic-m3s --m3s
./mesos-cli m3s status synthetic-m3s --m3s --kubernetes
./mesos-cli m3s scale synthetic-m3s 3 --agent
./mesos-cli m3s scale synthetic-m3s 3 --etcd
./mesos-cli m3s cluster synthetic-m3s stop
./mesos-cli m3s cluster synthetic-m3s start
./mesos-cli m3s cluster synthetic-m3s restart
```

`status` accepts either status flag or both. `scale` requires exactly one target flag: `--agent` or `--etcd`.

## External plugins

External plugins are configured as directory paths in the top-level `plugins` array:

```toml
plugins = [
  "/opt/mesos-cli/plugins/synthetic-plugin",
]
```

Each directory must contain a `plugin.toml` manifest:

```toml
name = "synthetic-plugin"
description = "Synthetic external plugin"
executable = "synthetic-plugin"
```

The executable path may be absolute or relative to the plugin directory. Arguments, standard input, standard output, standard error, and the current environment are forwarded to the executable.

## Demo

[Watch the terminal demo](demo/mesos-cli-demo.mp4).

The reproducible Bash demo is available as [`demo/demo.sh`](demo/demo.sh). It runs the real `agent`, `framework`, and `task` commands against a local synthetic Mesos master. It does not contact an external Mesos cluster and does not include the Compose or M3S extensions.

Run it locally with:

```bash
bash demo/demo.sh
```

Regenerate the MP4 with [VHS](https://github.com/charmbracelet/vhs) and FFmpeg available:

```bash
bash demo/render-demo.sh
```

## Development checks

```bash
make test
make fmt
make lint
```

Run all checks and rebuild before submitting changes:

```bash
make test && make fmt && make lint && make all
```
