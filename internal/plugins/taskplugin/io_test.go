package taskplugin

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mesos-cli/internal/mesos"
	"mesos-cli/internal/recordio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExecLaunchesSessionStreamsOutputAndReturnsExitCode(t *testing.T) {
	var launch map[string]any
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var message map[string]any
		if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
			t.Fatal(err)
		}
		switch message["type"] {
		case "LAUNCH_NESTED_CONTAINER_SESSION":
			launch = message
			w.Header().Set("Content-Type", "application/recordio")
			payload := map[string]any{"type": "DATA", "data": map[string]any{"type": "STDOUT", "data": base64.StdEncoding.EncodeToString([]byte("synthetic-output"))}}
			data, _ := json.Marshal(payload)
			w.Write(recordio.Encode(data))
		case "WAIT_CONTAINER":
			json.NewEncoder(w).Encode(map[string]any{"wait_container": map[string]any{"exit_status": 7 << 8}})
		default:
			t.Errorf("type=%v", message["type"])
		}
	}))
	defer server.Close()
	address := strings.TrimPrefix(server.URL, "http://")
	p := New(fakeClient{tasks: []mesos.Task{runningTask()}, agentAddress: address}, fakeConfig{}, server.Client())
	var out bytes.Buffer
	code, err := p.Run([]string{"exec", "synthetic-task-1", "/bin/sh", "-c", "printf synthetic-output"}, bytes.NewReader(nil), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if code != 7 || out.String() != "synthetic-output" || calls != 2 {
		t.Fatalf("code=%d out=%q calls=%d", code, out.String(), calls)
	}
	session := launch["launch_nested_container_session"].(map[string]any)
	command := session["command"].(map[string]any)
	if fmt.Sprint(command["arguments"]) != "[/bin/sh -c printf synthetic-output]" || command["shell"] != false {
		t.Fatalf("command=%v", command)
	}
}
func TestAttachNoStdinUsesAttachOutputCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var message map[string]any
		json.NewDecoder(r.Body).Decode(&message)
		if message["type"] != "ATTACH_CONTAINER_OUTPUT" {
			t.Errorf("message=%v", message)
		}
	}))
	defer server.Close()
	address := strings.TrimPrefix(server.URL, "http://")
	p := New(fakeClient{tasks: []mesos.Task{runningTask()}, agentAddress: address}, fakeConfig{}, server.Client())
	code, err := p.Run([]string{"attach", "--no-stdin", "synthetic-task-1"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
}
