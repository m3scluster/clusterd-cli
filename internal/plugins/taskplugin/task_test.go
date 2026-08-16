package taskplugin

import (
	"bytes"
	"encoding/json"
	"mesos-cli/internal/mesos"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type fakeClient struct {
	tasks        []mesos.Task
	agentAddress string
	err          error
}

func (f fakeClient) Tasks(url.Values) ([]mesos.Task, error) { return f.tasks, f.err }
func (f fakeClient) AgentAddress(string) (string, error)    { return f.agentAddress, f.err }

type fakeConfig struct{}

func (fakeConfig) AgentSSL() bool       { return false }
func (fakeConfig) AgentSSLVerify() bool { return true }
func (fakeConfig) AgentTimeout() int    { return 7 }
func (fakeConfig) AgentAuthentication() (string, string, bool) {
	return "agent-user", "agent-secret", true
}

func runningTask() mesos.Task {
	return mesos.Task{ID: "synthetic-task-1", State: "TASK_RUNNING", FrameworkID: "framework-1", ExecutorID: "executor-1", SlaveID: "agent-1", Statuses: []mesos.TaskStatus{{State: "TASK_RUNNING", ContainerStatus: &mesos.ContainerStatus{ContainerID: &mesos.ContainerIDValue{Value: "container-1"}}}}, Raw: map[string]any{"id": "synthetic-task-1", "state": "TASK_RUNNING"}}
}
func TestListUsesLatestStatusAndFiltersFinished(t *testing.T) {
	finished := runningTask()
	finished.ID = "finished-1"
	finished.Statuses = []mesos.TaskStatus{{State: "TASK_FINISHED"}}
	p := New(fakeClient{tasks: []mesos.Task{runningTask(), finished}}, fakeConfig{}, http.DefaultClient)
	var out bytes.Buffer
	_, err := p.Run([]string{"list"}, bytes.NewReader(nil), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "synthetic-task-1") || strings.Contains(out.String(), "finished-1") {
		t.Fatalf("out=%q", out.String())
	}
	out.Reset()
	p.Run([]string{"list", "--all"}, bytes.NewReader(nil), &out, &bytes.Buffer{})
	if !strings.Contains(out.String(), "finished-1") {
		t.Fatalf("all out=%q", out.String())
	}
}
func TestInspectPrintsFourSpaceJSON(t *testing.T) {
	p := New(fakeClient{tasks: []mesos.Task{runningTask()}}, fakeConfig{}, http.DefaultClient)
	var out bytes.Buffer
	_, err := p.Run([]string{"inspect", "synthetic-task-1"}, bytes.NewReader(nil), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "{\n    \"id\": \"synthetic-task-1\",\n    \"state\": \"TASK_RUNNING\"\n}\n" {
		t.Fatalf("out=%q", out.String())
	}
}
func TestUpdateMemoryPostsOperatorCallToOwningAgent(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1" {
			t.Errorf("path=%s", r.URL.Path)
		}
		user, secret, ok := r.BasicAuth()
		if !ok || user != "agent-user" || secret != "agent-secret" {
			t.Error("auth mismatch")
		}
		json.NewDecoder(r.Body).Decode(&payload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	address := strings.TrimPrefix(server.URL, "http://")
	p := New(fakeClient{tasks: []mesos.Task{runningTask()}, agentAddress: address}, fakeConfig{}, server.Client())
	code, err := p.Run([]string{"update-memory", "synthetic-task-1", "256"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	update := payload["update_container_memory_limit"].(map[string]any)
	if payload["type"] != "UPDATE_CONTAINER_MEMORY_LIMIT" || update["memory_limit"].(map[string]any)["value"] != float64(256) {
		t.Fatalf("payload=%v", payload)
	}
}
func TestUpdateMemoryRejectsInvalidValue(t *testing.T) {
	p := New(fakeClient{}, fakeConfig{}, http.DefaultClient)
	_, err := p.Run([]string{"update-memory", "synthetic-task-1", "0"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "positive integer") {
		t.Fatalf("err=%v", err)
	}
}
