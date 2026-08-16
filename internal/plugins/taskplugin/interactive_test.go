package taskplugin

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"mesos-cli/internal/mesos"
	"mesos-cli/internal/recordio"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestInteractiveExecStreamsStdinAsProcessIO(t *testing.T) {
	var mu sync.Mutex
	var inputRecords []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") == "application/recordio" {
			body, _ := io.ReadAll(r.Body)
			decoder := recordio.NewDecoder()
			records, err := decoder.Decode(body)
			if err != nil {
				t.Error(err)
			}
			mu.Lock()
			defer mu.Unlock()
			for _, record := range records {
				var message map[string]any
				json.Unmarshal(record, &message)
				inputRecords = append(inputRecords, message)
			}
			return
		}
		var message map[string]any
		json.NewDecoder(r.Body).Decode(&message)
		switch message["type"] {
		case "LAUNCH_NESTED_CONTAINER_SESSION":
			w.Header().Set("Content-Type", "application/recordio")
		case "WAIT_CONTAINER":
			json.NewEncoder(w).Encode(map[string]any{"wait_container": map[string]any{"exit_status": 0}})
		}
	}))
	defer server.Close()
	address := strings.TrimPrefix(server.URL, "http://")
	p := New(fakeClient{tasks: []mesos.Task{runningTask()}, agentAddress: address}, fakeConfig{}, server.Client())
	code, err := p.Run([]string{"exec", "-i", "synthetic-task-1", "/bin/cat"}, bytes.NewBufferString("synthetic-input"), &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, message := range inputRecords {
		attach, ok := message["attach_container_input"].(map[string]any)
		if !ok || attach["type"] != "PROCESS_IO" {
			continue
		}
		process := attach["process_io"].(map[string]any)
		data := process["data"].(map[string]any)
		decoded, _ := base64.StdEncoding.DecodeString(data["data"].(string))
		if string(decoded) == "synthetic-input" {
			found = true
		}
	}
	if !found {
		raw, _ := json.Marshal(inputRecords)
		t.Fatalf("stdin record missing: %s", raw)
	}
}
