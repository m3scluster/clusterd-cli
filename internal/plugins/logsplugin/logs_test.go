package logsplugin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"mesos-cli/internal/mesos"
)

type fakeClient struct {
	tasks        []mesos.Task
	agentAddress string
	queries      *[]url.Values
	agentIDs     *[]string
}

func (f fakeClient) Tasks(query url.Values) ([]mesos.Task, error) {
	if f.queries != nil {
		*f.queries = append(*f.queries, query)
	}
	return f.tasks, nil
}
func (f fakeClient) AgentAddress(id string) (string, error) {
	if f.agentIDs != nil {
		*f.agentIDs = append(*f.agentIDs, id)
	}
	return f.agentAddress, nil
}

type fakeConfig struct{ master string }

func (f fakeConfig) Master() (string, error) { return f.master, nil }
func (fakeConfig) Principal() string         { return "master-user" }
func (fakeConfig) Secret() string            { return "master-secret" }
func (fakeConfig) MasterSSLVerify() bool     { return true }
func (fakeConfig) AgentSSL() bool            { return false }
func (fakeConfig) AgentSSLVerify() bool      { return true }
func (fakeConfig) AgentTimeout() int         { return 10 }
func (fakeConfig) AgentAuthentication() (string, string, bool) {
	return "agent-user", "agent-secret", true
}

func TestMasterLogsReadsAuthenticatedOperatorAPI(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1" {
			t.Errorf("request=%s %s", r.Method, r.URL.Path)
		}
		user, secret, ok := r.BasicAuth()
		if !ok || user != "master-user" || secret != "master-secret" {
			t.Errorf("auth=%q %q %t", user, secret, ok)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"READ_LOG","read_log":{"size":"16","data":"c3ludGhldGljLW1hc3Rlcg=="}}`))
	}))
	defer server.Close()

	plugin := New(fakeClient{}, fakeConfig{master: server.URL}, server.Client())
	var stdout, stderr bytes.Buffer
	code, err := plugin.Run([]string{"master"}, bytes.NewReader(nil), &stdout, &stderr)
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if stdout.String() != "synthetic-master" || stderr.String() != "" {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	readLog, ok := payload["read_log"].(map[string]any)
	if payload["type"] != "READ_LOG" || !ok || readLog["offset"] != float64(0) {
		t.Fatalf("payload=%v", payload)
	}
}

func TestAgentLogsReadsAuthenticatedOperatorAPI(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1" {
			t.Errorf("request=%s %s", r.Method, r.URL.Path)
		}
		user, secret, ok := r.BasicAuth()
		if !ok || user != "agent-user" || secret != "agent-secret" {
			t.Errorf("auth=%q %q %t", user, secret, ok)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"READ_LOG","read_log":{"stdout":{"size":"15","data":"c3ludGhldGljLWFnZW50"},"stderr":{"size":"0","data":""}}}`))
	}))
	defer server.Close()

	address := strings.TrimPrefix(server.URL, "http://")
	plugin := New(fakeClient{agentAddress: address}, fakeConfig{}, server.Client())
	var stdout, stderr bytes.Buffer
	code, err := plugin.Run([]string{"agent", "synthetic-agent-1"}, bytes.NewReader(nil), &stdout, &stderr)
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if stdout.String() != "synthetic-agent" || stderr.String() != "" {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	readLog, ok := payload["read_log"].(map[string]any)
	if payload["type"] != "READ_LOG" || !ok || readLog["source"] != "AGENT" || readLog["stdout_offset"] != float64(0) || readLog["stderr_offset"] != float64(0) {
		t.Fatalf("payload=%v", payload)
	}
}

func TestTaskLogsPreservesLegacySyntaxAndResolvesOwningAgent(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"type":"READ_LOG","read_log":{"stdout":{"size":"16","data":"c3ludGhldGljLXN0ZG91dA=="},"stderr":{"size":"16","data":"c3ludGhldGljLXN0ZGVycg=="}}}`))
	}))
	defer server.Close()

	queries := []url.Values{}
	agentIDs := []string{}
	address := strings.TrimPrefix(server.URL, "http://")
	client := fakeClient{
		tasks: []mesos.Task{{
			ID: "synthetic-task-1", State: "TASK_RUNNING", SlaveID: "synthetic-agent-1",
			Statuses: []mesos.TaskStatus{{
				State:           "TASK_RUNNING",
				ContainerStatus: &mesos.ContainerStatus{ContainerID: &mesos.ContainerIDValue{Value: "synthetic-container-1"}},
			}},
		}},
		agentAddress: address,
		queries:      &queries,
		agentIDs:     &agentIDs,
	}
	plugin := New(client, fakeConfig{}, server.Client())
	var stdout, stderr bytes.Buffer
	code, err := plugin.Run([]string{"synthetic-task-1"}, bytes.NewReader(nil), &stdout, &stderr)
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if stdout.String() != "synthetic-stdout" || stderr.String() != "synthetic-stderr" {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if len(queries) != 1 || queries[0].Get("task_id") != "synthetic-task-1" || len(agentIDs) != 1 || agentIDs[0] != "synthetic-agent-1" {
		t.Fatalf("queries=%v agentIDs=%v", queries, agentIDs)
	}
	readLog := payload["read_log"].(map[string]any)
	container := readLog["container_id"].(map[string]any)
	if readLog["source"] != "CONTAINER" || container["value"] != "synthetic-container-1" {
		t.Fatalf("payload=%v", payload)
	}
}

func TestLogsHelpDocumentsAllTargets(t *testing.T) {
	plugin := New(fakeClient{}, fakeConfig{}, http.DefaultClient)
	var stdout bytes.Buffer
	code, err := plugin.Run([]string{"--help"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	for _, expected := range []string{
		"clusterd-cli logs [-f | --follow] [-n <number>] <mesos-task-id>",
		"clusterd-cli logs [-f | --follow] [-n <number>] task <mesos-task-id>",
		"clusterd-cli logs [-f | --follow] [-n <number>] agent <mesos-agent-id>",
		"clusterd-cli logs [-f | --follow] [-n <number>] master",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("help missing %q:\n%s", expected, stdout.String())
		}
	}
}

func TestMasterLogsFollowAdvancesOffset(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload struct {
			ReadLog struct {
				Offset uint64 `json:"offset"`
			} `json:"read_log"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if calls == 1 {
			if payload.ReadLog.Offset != 0 {
				t.Fatalf("first offset=%d", payload.ReadLog.Offset)
			}
			_, _ = w.Write([]byte(`{"type":"READ_LOG","read_log":{"size":"1","data":"QQ=="}}`))
			return
		}
		if payload.ReadLog.Offset != 1 {
			t.Fatalf("second offset=%d", payload.ReadLog.Offset)
		}
		http.Error(w, "synthetic stop", http.StatusInternalServerError)
	}))
	defer server.Close()

	original := pollInterval
	pollInterval = time.Millisecond
	defer func() { pollInterval = original }()

	plugin := New(fakeClient{}, fakeConfig{master: server.URL}, server.Client())
	var stdout bytes.Buffer
	code, err := plugin.Run([]string{"-f", "master"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
	if err == nil || code != 1 || calls != 2 || stdout.String() != "A" {
		t.Fatalf("code=%d err=%v calls=%d stdout=%q", code, err, calls, stdout.String())
	}
}

func TestAgentLogsFollowAdvancesBothOffsets(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload struct {
			ReadLog struct {
				StdoutOffset uint64 `json:"stdout_offset"`
				StderrOffset uint64 `json:"stderr_offset"`
			} `json:"read_log"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if calls == 1 {
			_, _ = w.Write([]byte(`{"type":"READ_LOG","read_log":{"stdout":{"size":"1","data":"QQ=="},"stderr":{"size":"1","data":"Qg=="}}}`))
			return
		}
		if payload.ReadLog.StdoutOffset != 1 || payload.ReadLog.StderrOffset != 1 {
			t.Fatalf("offsets=%d/%d", payload.ReadLog.StdoutOffset, payload.ReadLog.StderrOffset)
		}
		http.Error(w, "synthetic stop", http.StatusInternalServerError)
	}))
	defer server.Close()

	original := pollInterval
	pollInterval = time.Millisecond
	defer func() { pollInterval = original }()

	address := strings.TrimPrefix(server.URL, "http://")
	plugin := New(fakeClient{agentAddress: address}, fakeConfig{}, server.Client())
	var stdout, stderr bytes.Buffer
	code, err := plugin.Run([]string{"agent", "synthetic-agent-1", "--follow"}, bytes.NewReader(nil), &stdout, &stderr)
	if err == nil || code != 1 || calls != 2 || stdout.String() != "A" || stderr.String() != "B" {
		t.Fatalf("code=%d err=%v calls=%d stdout=%q stderr=%q", code, err, calls, stdout.String(), stderr.String())
	}
}

func TestParseArgsSupportsFollowAndLineCountInEitherOrder(t *testing.T) {
	for _, args := range [][]string{{"-f", "-n", "20", "master"}, {"-n", "20", "-f", "master"}} {
		follow, lines, positional, err := parseArgs(args)
		if err != nil || !follow || lines != 20 || len(positional) != 1 || positional[0] != "master" {
			t.Fatalf("args=%v follow=%v lines=%d positional=%v err=%v", args, follow, lines, positional, err)
		}
	}
}

func TestParseArgsRejectsInvalidLineCount(t *testing.T) {
	for _, args := range [][]string{{"-n"}, {"-n", "nope"}, {"-n", "-1"}} {
		if _, _, _, err := parseArgs(args); err == nil {
			t.Fatalf("args=%v unexpectedly succeeded", args)
		}
	}
}

func TestLineOutputPrintsLastLines(t *testing.T) {
	var out bytes.Buffer
	output := newLineOutput(2, &out)
	_, _ = output.Write([]byte("one\ntwo\nthree\n"))
	if err := output.Flush(); err != nil {
		t.Fatal(err)
	}
	if out.String() != "two\nthree\n" {
		t.Fatalf("output=%q", out.String())
	}
}
