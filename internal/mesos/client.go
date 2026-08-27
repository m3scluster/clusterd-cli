package mesos

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

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

type Client struct {
	config Configuration
	http   *http.Client
}

func NewClient(config Configuration, httpClient *http.Client) *Client {
	if httpClient == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: !config.MasterSSLVerify()} // compatibility option
		httpClient = &http.Client{Transport: transport, Timeout: time.Duration(config.AgentTimeout()) * time.Second}
	}
	return &Client{config: config, http: httpClient}
}

type Agent struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
	Active   bool   `json:"active"`
	PID      string `json:"pid"`
}

func (a Agent) Address() string {
	parts := strings.SplitN(a.PID, "@", 2)
	if len(parts) != 2 || a.Hostname == "" {
		return ""
	}

	_, port, err := net.SplitHostPort(parts[1])
	if err != nil {
		if separator := strings.LastIndex(parts[1], ":"); separator >= 0 {
			port = parts[1][separator+1:]
		}
	}
	if port == "" {
		return a.Hostname
	}
	return net.JoinHostPort(a.Hostname, port)
}

type Framework struct {
	ID       string         `json:"id"`
	Active   bool           `json:"active"`
	Hostname string         `json:"hostname"`
	Name     string         `json:"name"`
	WebUIURL string         `json:"webui_url"`
	Raw      map[string]any `json:"-"`
}

type ContainerIDValue struct {
	Value  string            `json:"value"`
	Parent *ContainerIDValue `json:"parent,omitempty"`
}
type ContainerStatus struct {
	ContainerID *ContainerIDValue `json:"container_id"`
}
type TaskStatus struct {
	State           string           `json:"state"`
	ContainerStatus *ContainerStatus `json:"container_status"`
}
type Task struct {
	ID          string         `json:"id"`
	State       string         `json:"state"`
	FrameworkID string         `json:"framework_id"`
	ExecutorID  string         `json:"executor_id"`
	SlaveID     string         `json:"slave_id"`
	Statuses    []TaskStatus   `json:"statuses"`
	Raw         map[string]any `json:"-"`
}

func decodeWithRaw[T any](items []json.RawMessage, factory func() *T, setRaw func(*T, map[string]any)) ([]T, error) {
	result := make([]T, 0, len(items))
	for _, raw := range items {
		value := factory()
		if err := json.Unmarshal(raw, value); err != nil {
			return nil, err
		}
		var object map[string]any
		if err := json.Unmarshal(raw, &object); err != nil {
			return nil, err
		}
		setRaw(value, object)
		result = append(result, *value)
	}
	return result, nil
}

func normalize(address string) string {
	if !strings.Contains(address, "://") {
		return "http://" + address
	}
	return strings.TrimRight(address, "/")
}
func (c *Client) get(endpoint string, query url.Values, target any) error {
	master, err := c.config.Master()
	if err != nil {
		return err
	}
	requestURL := normalize(master) + "/" + strings.TrimLeft(endpoint, "/")
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return err
	}
	if c.config.Principal() != "" && c.config.Secret() != "" {
		req.SetBasicAuth(c.config.Principal(), c.config.Secret())
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("Unable to open url '%s': %v", requestURL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Unable to open url '%s': %s: %s", requestURL, resp.Status, string(body))
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("Could not load JSON from '%s': %v", string(body), err)
	}
	return nil
}

func (c *Client) Agents() ([]Agent, error) {
	var response struct {
		Slaves []Agent `json:"slaves"`
	}
	if err := c.get("slaves", nil, &response); err != nil {
		return nil, fmt.Errorf("Could not open '/slaves' on master: %v", err)
	}
	return response.Slaves, nil
}
func (c *Client) Frameworks() ([]Framework, error) {
	var response struct {
		Frameworks []json.RawMessage `json:"frameworks"`
	}
	if err := c.get("master/frameworks/", nil, &response); err != nil {
		return nil, fmt.Errorf("Could not open '/master/frameworks/' on master: %v", err)
	}
	return decodeWithRaw(response.Frameworks, func() *Framework { return &Framework{} }, func(v *Framework, m map[string]any) { v.Raw = m })
}
func (c *Client) Tasks(query url.Values) ([]Task, error) {
	if query == nil {
		query = url.Values{"order": {"asc"}, "limit": {"-1"}}
	}
	var response struct {
		Tasks []json.RawMessage `json:"tasks"`
	}
	if err := c.get("tasks", query, &response); err != nil {
		return nil, fmt.Errorf("Could not open '/tasks' with query parameters: %von master: %v", query, err)
	}
	return decodeWithRaw(response.Tasks, func() *Task { return &Task{} }, func(v *Task, m map[string]any) { v.Raw = m })
}
func (c *Client) AgentAddress(id string) (string, error) {
	agents, err := c.Agents()
	if err != nil {
		return "", err
	}
	for _, a := range agents {
		if a.ID == id {
			return a.Address(), nil
		}
	}
	return "", fmt.Errorf("Unable to find agent '%s'", id)
}
func ContainerID(task Task) (*ContainerIDValue, error) {
	if task.Statuses == nil {
		return nil, fmt.Errorf("Unable to obtain status information for task")
	}
	if len(task.Statuses) == 0 {
		return nil, fmt.Errorf("No status updates available for task")
	}
	if task.Statuses[0].ContainerStatus == nil {
		return nil, fmt.Errorf("Task status does not contain container information")
	}
	id := task.Statuses[0].ContainerStatus.ContainerID
	if id == nil || id.Value == "" {
		return nil, fmt.Errorf("No container found for the specified task. It might still be spinning up. Please try again.")
	}
	return id, nil
}
