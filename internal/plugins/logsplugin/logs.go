package logsplugin

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mesos-cli/internal/mesos"
)

type Client interface {
	Tasks(url.Values) ([]mesos.Task, error)
	AgentAddress(string) (string, error)
}

type Configuration interface {
	Master() (string, error)
	Principal() string
	Secret() string
	MasterSSLVerify() bool
	AgentSSL() bool
	AgentSSLVerify() bool
	AgentTimeout() int
	AgentAuthentication() (string, string, bool)
}

type Plugin struct {
	client     Client
	config     Configuration
	masterHTTP *http.Client
	agentHTTP  *http.Client
}

var pollInterval = 500 * time.Millisecond

func New(client Client, config Configuration, httpClient *http.Client) *Plugin {
	if httpClient != nil {
		return &Plugin{client: client, config: config, masterHTTP: httpClient, agentHTTP: httpClient}
	}
	return &Plugin{
		client:     client,
		config:     config,
		masterHTTP: newHTTPClient(config.MasterSSLVerify(), config.AgentTimeout()),
		agentHTTP:  newHTTPClient(config.AgentSSLVerify(), config.AgentTimeout()),
	}
}

func newHTTPClient(verify bool, timeout int) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: !verify}
	return &http.Client{Transport: transport, Timeout: time.Duration(timeout) * time.Second}
}

func (*Plugin) Name() string          { return "logs" }
func (*Plugin) Description() string   { return "Reads task, agent, and master logs" }
func (*Plugin) Subcommands() []string { return []string{"agent", "master", "task"} }

func (p *Plugin) Run(args []string, _ io.Reader, stdout, stderr io.Writer) (int, error) {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprint(stdout, help)
		return 0, nil
	}
	follow, positional, err := parseArgs(args)
	if err != nil {
		return 1, err
	}
	if len(positional) == 1 && positional[0] == "master" {
		return p.masterLogs(follow, stdout)
	}
	if len(positional) == 2 && positional[0] == "agent" {
		return p.agentLogs(positional[1], follow, stdout, stderr)
	}
	if len(positional) == 2 && positional[0] == "task" {
		return p.taskLogs(positional[1], follow, stdout, stderr)
	}
	if len(positional) == 1 {
		return p.taskLogs(positional[0], follow, stdout, stderr)
	}
	return 1, fmt.Errorf("invalid logs target; see 'clusterd-cli help logs'")
}

func parseArgs(args []string) (bool, []string, error) {
	follow := false
	positional := make([]string, 0, len(args))
	for _, arg := range args {
		switch arg {
		case "-f", "--follow":
			follow = true
		default:
			if strings.HasPrefix(arg, "-") {
				return false, nil, fmt.Errorf("unknown option: %s", arg)
			}
			positional = append(positional, arg)
		}
	}
	return follow, positional, nil
}

type logSize uint64

func (s *logSize) UnmarshalJSON(data []byte) error {
	var text string
	if len(data) > 0 && data[0] == '"' {
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
	} else {
		text = string(data)
	}
	value, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		return err
	}
	*s = logSize(value)
	return nil
}

func normalize(address string) string {
	if !strings.Contains(address, "://") {
		return "http://" + strings.TrimRight(address, "/")
	}
	return strings.TrimRight(address, "/")
}

type agentLogResponse struct {
	Type    string `json:"type"`
	ReadLog struct {
		Stdout *struct {
			Size logSize `json:"size"`
			Data []byte  `json:"data"`
		} `json:"stdout"`
		Stderr *struct {
			Size logSize `json:"size"`
			Data []byte  `json:"data"`
		} `json:"stderr"`
	} `json:"read_log"`
}

func (p *Plugin) agentLogs(agentID string, follow bool, stdout, stderr io.Writer) (int, error) {
	address, err := p.client.AgentAddress(agentID)
	if err != nil {
		return 1, fmt.Errorf("Unable to resolve agent '%s': %v", agentID, err)
	}
	scheme := "http://"
	if p.config.AgentSSL() {
		scheme = "https://"
	}
	endpoint := scheme + address + "/api/v1"
	user, secret, authenticated := p.config.AgentAuthentication()
	if !authenticated {
		user, secret = "", ""
	}

	var stdoutOffset, stderrOffset uint64
	for {
		payload := map[string]any{
			"type": "READ_LOG",
			"read_log": map[string]any{
				"source":        "AGENT",
				"stdout_offset": stdoutOffset,
				"stderr_offset": stderrOffset,
			},
		}
		var result agentLogResponse
		if err := p.post(p.agentHTTP, endpoint, user, secret, payload, &result); err != nil {
			return 1, fmt.Errorf("Unable to read logs for agent '%s': %v", agentID, err)
		}
		if result.Type != "READ_LOG" {
			return 1, fmt.Errorf("Unexpected response type while reading logs for agent '%s': %s", agentID, result.Type)
		}

		stdoutSize, stderrSize := stdoutOffset, stderrOffset
		if result.ReadLog.Stdout != nil {
			stdoutSize = uint64(result.ReadLog.Stdout.Size)
			if stdoutSize < stdoutOffset {
				stdoutOffset = 0
			} else if len(result.ReadLog.Stdout.Data) > 0 {
				if _, err := stdout.Write(result.ReadLog.Stdout.Data); err != nil {
					return 1, err
				}
				stdoutOffset += uint64(len(result.ReadLog.Stdout.Data))
			}
		}
		if result.ReadLog.Stderr != nil {
			stderrSize = uint64(result.ReadLog.Stderr.Size)
			if stderrSize < stderrOffset {
				stderrOffset = 0
			} else if len(result.ReadLog.Stderr.Data) > 0 {
				if _, err := stderr.Write(result.ReadLog.Stderr.Data); err != nil {
					return 1, err
				}
				stderrOffset += uint64(len(result.ReadLog.Stderr.Data))
			}
		}
		if stdoutOffset < stdoutSize || stderrOffset < stderrSize {
			continue
		}
		if !follow {
			return 0, nil
		}
		time.Sleep(pollInterval)
	}
}

func (p *Plugin) taskLogs(taskID string, follow bool, stdout, stderr io.Writer) (int, error) {
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
	address, err := p.client.AgentAddress(task.SlaveID)
	if err != nil {
		return 1, fmt.Errorf("Could not resolve the agent for task '%s': %v", taskID, err)
	}
	scheme := "http://"
	if p.config.AgentSSL() {
		scheme = "https://"
	}
	endpoint := scheme + address + "/api/v1"
	user, secret, authenticated := p.config.AgentAuthentication()
	if !authenticated {
		user, secret = "", ""
	}

	var stdoutOffset, stderrOffset uint64
	for {
		payload := map[string]any{
			"type": "READ_LOG",
			"read_log": map[string]any{
				"source":        "CONTAINER",
				"container_id":  containerID,
				"stdout_offset": stdoutOffset,
				"stderr_offset": stderrOffset,
			},
		}
		var result agentLogResponse
		if err := p.post(p.agentHTTP, endpoint, user, secret, payload, &result); err != nil {
			return 1, fmt.Errorf("Unable to read logs for task '%s': %v", taskID, err)
		}
		if result.Type != "READ_LOG" {
			return 1, fmt.Errorf("Unexpected response type while reading logs for task '%s': %s", taskID, result.Type)
		}

		stdoutSize, stderrSize := stdoutOffset, stderrOffset
		if result.ReadLog.Stdout != nil {
			stdoutSize = uint64(result.ReadLog.Stdout.Size)
			if stdoutSize < stdoutOffset {
				stdoutOffset = 0
			} else if len(result.ReadLog.Stdout.Data) > 0 {
				if _, err := stdout.Write(result.ReadLog.Stdout.Data); err != nil {
					return 1, err
				}
				stdoutOffset += uint64(len(result.ReadLog.Stdout.Data))
			}
		}
		if result.ReadLog.Stderr != nil {
			stderrSize = uint64(result.ReadLog.Stderr.Size)
			if stderrSize < stderrOffset {
				stderrOffset = 0
			} else if len(result.ReadLog.Stderr.Data) > 0 {
				if _, err := stderr.Write(result.ReadLog.Stderr.Data); err != nil {
					return 1, err
				}
				stderrOffset += uint64(len(result.ReadLog.Stderr.Data))
			}
		}
		if stdoutOffset < stdoutSize || stderrOffset < stderrSize {
			continue
		}
		if !follow {
			return 0, nil
		}
		time.Sleep(pollInterval)
	}
}

func (p *Plugin) masterLogs(follow bool, stdout io.Writer) (int, error) {
	master, err := p.config.Master()
	if err != nil {
		return 1, err
	}
	var offset uint64
	for {
		payload := map[string]any{
			"type":     "READ_LOG",
			"read_log": map[string]any{"offset": offset},
		}
		var result struct {
			Type    string `json:"type"`
			ReadLog struct {
				Size logSize `json:"size"`
				Data []byte  `json:"data"`
			} `json:"read_log"`
		}
		if err := p.post(p.masterHTTP, normalize(master)+"/api/v1", p.config.Principal(), p.config.Secret(), payload, &result); err != nil {
			return 1, fmt.Errorf("Unable to read master logs: %v", err)
		}
		if result.Type != "READ_LOG" {
			return 1, fmt.Errorf("Unexpected response type while reading master logs: %s", result.Type)
		}
		size := uint64(result.ReadLog.Size)
		if size < offset {
			offset = 0
		} else if len(result.ReadLog.Data) > 0 {
			if _, err := stdout.Write(result.ReadLog.Data); err != nil {
				return 1, err
			}
			offset += uint64(len(result.ReadLog.Data))
		}
		if offset < size {
			continue
		}
		if !follow {
			return 0, nil
		}
		time.Sleep(pollInterval)
	}
}

func (p *Plugin) post(client *http.Client, endpoint, user, secret string, payload any, result any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if user != "" && secret != "" {
		req.SetBasicAuth(user, secret)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s: %s", resp.Status, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(result)
}

const help = `Reads task, agent, and master logs

Usage:
  clusterd-cli logs [-f | --follow] <mesos-task-id>
  clusterd-cli logs [-f | --follow] task <mesos-task-id>
  clusterd-cli logs [-f | --follow] agent <mesos-agent-id>
  clusterd-cli logs [-f | --follow] master

Options:
  -f --follow  Continue printing new log output.
  -h --help    Show this screen.
`
