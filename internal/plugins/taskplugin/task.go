package taskplugin

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"mesos-cli/internal/mesos"
	"mesos-cli/internal/table"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"
)

type TaskClient interface {
	Tasks(url.Values) ([]mesos.Task, error)
	AgentAddress(string) (string, error)
}
type Configuration interface {
	AgentSSL() bool
	AgentSSLVerify() bool
	AgentTimeout() int
	AgentAuthentication() (string, string, bool)
}
type TaskPlugin struct {
	client TaskClient
	config Configuration
	http   *http.Client
}

func New(client TaskClient, config Configuration, httpClient *http.Client) *TaskPlugin {
	if httpClient == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: !config.AgentSSLVerify()}
		httpClient = &http.Client{Transport: transport, Timeout: time.Duration(config.AgentTimeout()) * time.Second}
	}
	return &TaskPlugin{client: client, config: config, http: httpClient}
}
func (*TaskPlugin) Name() string        { return "task" }
func (*TaskPlugin) Description() string { return "Interacts with the tasks running in a Mesos cluster" }
func (*TaskPlugin) Subcommands() []string {
	return []string{"attach", "exec", "inspect", "kill", "list"}
}
func (p *TaskPlugin) Run(args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(stdout, help)
		return 0, nil
	}
	if args[0] == "--version" {
		fmt.Fprintln(stdout, "Mesos CLI Task Plugin")
		return 0, nil
	}
	switch args[0] {
	case "list":
		return p.list(slices.Contains(args[1:], "--all") || slices.Contains(args[1:], "-a"), stdout)
	case "inspect":
		if len(args) < 2 {
			return 1, fmt.Errorf("missing <task_id>")
		}
		return p.inspect(args[1], stdout)
	case "update-memory":
		if len(args) < 3 {
			return 1, fmt.Errorf("missing <task-id> or <memory-mib>")
		}
		return p.updateMemory(args[1], args[2])
	case "attach":
		return p.attach(args[1:], stdin, stdout, stderr)
	case "exec":
		return p.exec(args[1:], stdin, stdout, stderr)
	default:
		fmt.Fprint(stdout, help)
		return 0, nil
	}
}
func (p *TaskPlugin) list(all bool, stdout io.Writer) (int, error) {
	tasks, err := p.client.Tasks(nil)
	if err != nil {
		return 1, fmt.Errorf("Unable to get tasks: %v", err)
	}
	if len(tasks) == 0 {
		fmt.Fprintln(stdout, "There are no tasks running in the cluster.")
		return 0, nil
	}
	tbl, _ := table.New([]string{"ID", "State", "Framework ID", "Executor ID"})
	for _, task := range tasks {
		state := "UNKNOWN"
		if len(task.Statuses) > 0 {
			state = task.Statuses[len(task.Statuses)-1].State
		}
		if !all && state != "TASK_RUNNING" {
			continue
		}
		if err := tbl.AddRow([]string{task.ID, state, task.FrameworkID, task.ExecutorID}); err != nil {
			return 1, fmt.Errorf("Unable to build table of tasks: %v", err)
		}
	}
	fmt.Fprintln(stdout, tbl.String())
	return 0, nil
}
func (p *TaskPlugin) inspect(id string, stdout io.Writer) (int, error) {
	tasks, err := p.client.Tasks(nil)
	if err != nil {
		return 1, err
	}
	for _, task := range tasks {
		if task.ID == id {
			data, err := json.MarshalIndent(task.Raw, "", "    ")
			if err != nil {
				return 1, err
			}
			fmt.Fprintln(stdout, string(data))
			break
		}
	}
	return 0, nil
}
func (p *TaskPlugin) updateMemory(taskID, memoryText string) (int, error) {
	memory, err := strconv.Atoi(memoryText)
	if err != nil || memory <= 0 {
		return 1, fmt.Errorf("Memory limit must be a positive integer in MiB")
	}
	tasks, err := p.client.Tasks(url.Values{"task_id": {taskID}})
	if err != nil {
		return 1, fmt.Errorf("Unable to get task with ID %s: %v", taskID, err)
	}
	matches := []mesos.Task{}
	for _, task := range tasks {
		if task.ID == taskID && task.State == "TASK_RUNNING" {
			matches = append(matches, task)
		}
	}
	if len(matches) == 0 {
		return 1, fmt.Errorf("Unable to find running task '%s'", taskID)
	}
	if len(matches) > 1 {
		return 1, fmt.Errorf("More than one running task matching id '%s'", taskID)
	}
	task := matches[0]
	containerID, err := mesos.ContainerID(task)
	if err != nil {
		return 1, fmt.Errorf("Could not get container ID of task '%s': %v", taskID, err)
	}
	agent, err := p.client.AgentAddress(task.SlaveID)
	if err != nil {
		return 1, fmt.Errorf("Could not resolve the agent for task '%s': %v", taskID, err)
	}
	scheme := "http://"
	if p.config.AgentSSL() {
		scheme = "https://"
	}
	endpoint := scheme + agent + "/api/v1"
	message := map[string]any{"type": "UPDATE_CONTAINER_MEMORY_LIMIT", "update_container_memory_limit": map[string]any{"container_id": containerID, "memory_limit": map[string]any{"value": memory}}}
	data, _ := json.Marshal(message)
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return 1, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if user, secret, ok := p.config.AgentAuthentication(); ok {
		req.SetBasicAuth(user, secret)
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return 1, fmt.Errorf("Unable to update memory limit for task '%s' on agent '%s': %v", taskID, endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return 1, fmt.Errorf("Unable to update memory limit for task '%s' on agent '%s': %s: %s", taskID, endpoint, resp.Status, string(body))
	}
	return 0, nil
}

const help = `Interacts with the tasks running in a Mesos cluster

Usage:
  mesos task (-h | --help)
  mesos task --version
  mesos task <command> (-h | --help)
  mesos task [options] <command> [<args>...]

Options:
  -h --help  Show this screen.
  --version  Show version info.

Commands:
  attach         Attach the CLI to the stdio of a running task
  exec           Execute commands in a task's container
  inspect        Return low-level information on the task
  list           List all running tasks in a Mesos cluster
  update-memory  Change the memory limit of a running task
`
