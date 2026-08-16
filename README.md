# Mesos CLI in Go

A Go-based CLI tool for managing and monitoring Apache Mesos clusters.

## Installation

The CLI is pre-built and ready to use at `/data/mesos-cli`. No dependencies are required.

## Usage

```bash
./mesos-cli [command] [args...]
```

### Commands

- **`state`**: Show cluster state summary
  Displays cluster ID, name, number of slaves, and tasks.

- **`slaves`**: List all slave nodes
  Shows detailed information about each slave including CPU, memory, and disk resources.

- **`tasks`**: List all tasks
  Displays the total number of running tasks across all frameworks.

- **`containers`**: List container status
  Shows status of all containers including their state and resource usage.

- **`help`** (or `-h`, `--help`): Show help
  Displays this help message.

## Examples

```bash
# Show cluster state
./mesos-cli state

# List all slaves
./mesos-cli slaves

# List all tasks
./mesos-cli tasks

# List container status
./mesos-cli containers
```

## Accessing the Mesos UI

The Mesos web interface is available at: http://devtest.lab.internal:5050

Authentication:
- Username: mesos
- Password: test

## Remote Access

You can SSH to the servers where the containers run:
- **devtest.lab.internal** - Primary Mesos server
- **andreas-ki.lab.internal** - Additional Kubernetes server

Use the SSH key `~/.ssh/hermes-agent` as user `andreas`.

## Development

The source code is located in `/data/mesos-cli-go/` and uses Go modules.

To rebuild:
```bash
cd /data/mesos-cli-go
go build -o mesos-cli
```

## Dependencies

- Go 1.21+
- Apache Mesos 1.x API

## Configuration

The tool connects to Mesos at `http://devtest.lab.internal:5050` using basic authentication.

You can modify the connection parameters in the `main.go` file:

```go
var (
	mesosURL    = "http://devtest.lab.internal:5050"
	mesosUser   = "mesos"
	mesosPass   = "test"
)
```

## Supported Mesos API Endpoints

- `/api/v1/state` - Get cluster state
- `/api/v1/states` - Get detailed state including tasks and containers

## Features

- 📊 Cluster state monitoring
- 🖥️  Slave node inventory
- 🔧 Container status tracking
- 📋 Task management
- ✅ Simple, dependency-free Go implementation
- 🚀 Fast execution (no Python dependencies)

## Notes

- The tool is optimized for Mesos clusters with Docker containerizers
- Resource usage is reported in the format the Mesos API returns
- Container details include CPU, memory, and disk usage
