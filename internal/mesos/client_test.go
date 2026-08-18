package mesos

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

type testConfig struct{ master string }

func (c testConfig) Master() (string, error) { return c.master, nil }
func (testConfig) Principal() string         { return "synthetic-user" }
func (testConfig) Secret() string            { return "synthetic-secret" }
func (testConfig) MasterSSLVerify() bool     { return true }
func (testConfig) AgentSSL() bool            { return false }
func (testConfig) AgentSSLVerify() bool      { return true }
func (testConfig) AgentTimeout() int         { return 7 }
func (testConfig) AgentAuthentication() (string, string, bool) {
	return "agent-user", "agent-secret", true
}

func TestAgentsUsesSlavesEndpointAndBasicAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/slaves" {
			t.Errorf("path=%s", r.URL.Path)
		}
		user, secret, ok := r.BasicAuth()
		if !ok || user != "synthetic-user" || secret != "synthetic-secret" {
			t.Error("basic auth mismatch")
		}
		json.NewEncoder(w).Encode(map[string]any{"slaves": []map[string]any{{"id": "agent-1", "hostname": "node.example.test", "active": true, "pid": "slave(1)@node.example.test:5051"}}})
	}))
	defer server.Close()
	client := NewClient(testConfig{master: server.URL}, server.Client())
	agents, err := client.Agents()
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 || agents[0].ID != "agent-1" || agents[0].Address() != "node.example.test:5051" {
		t.Fatalf("agents=%+v", agents)
	}
}

func TestTasksPreservesQueryParameters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tasks" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if got := r.URL.Query().Get("task_id"); got != "synthetic-task-1" {
			t.Errorf("task_id=%q", got)
		}
		json.NewEncoder(w).Encode(map[string]any{"tasks": []map[string]any{{"id": "synthetic-task-1", "state": "TASK_RUNNING"}}})
	}))
	defer server.Close()
	client := NewClient(testConfig{master: server.URL}, server.Client())
	tasks, err := client.Tasks(url.Values{"task_id": {"synthetic-task-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].ID != "synthetic-task-1" {
		t.Fatalf("tasks=%+v", tasks)
	}
}

func TestFrameworksExposeWebUIURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"frameworks": []map[string]any{{
			"id":        "synthetic-framework-id",
			"name":      "synthetic-framework",
			"active":    true,
			"webui_url": "http://framework.example.test:10000",
		}}})
	}))
	defer server.Close()
	client := NewClient(testConfig{master: server.URL}, server.Client())
	frameworks, err := client.Frameworks()
	if err != nil {
		t.Fatal(err)
	}
	if len(frameworks) != 1 || frameworks[0].WebUIURL != "http://framework.example.test:10000" {
		t.Fatalf("frameworks=%+v", frameworks)
	}
}

func TestContainerIDReportsMissingStatus(t *testing.T) {
	_, err := ContainerID(Task{ID: "synthetic-task-1"})
	if err == nil || err.Error() != "Unable to obtain status information for task" {
		t.Fatalf("err=%v", err)
	}
}
