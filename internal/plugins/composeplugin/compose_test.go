package composeplugin

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mesos-cli/internal/config"
	"mesos-cli/internal/mesos"
)

type fakeFrameworkClient struct {
	frameworks []mesos.Framework
	err        error
}

func (f fakeFrameworkClient) Frameworks() ([]mesos.Framework, error) { return f.frameworks, f.err }

type fakeCredentials struct {
	entries map[string]config.FrameworkCredentials
}

func (f fakeCredentials) FrameworkCredentials(_, name string) (config.FrameworkCredentials, error) {
	return f.entries[name], nil
}

func newComposeForServer(server *httptest.Server) *ComposePlugin {
	return New(
		fakeFrameworkClient{frameworks: []mesos.Framework{{
			ID:       "synthetic-compose-id",
			Name:     "synthetic-compose",
			Active:   true,
			WebUIURL: server.URL,
		}}},
		fakeCredentials{entries: map[string]config.FrameworkCredentials{
			"synthetic-compose": {Principal: "synthetic-user", Secret: "synthetic-secret"},
		}},
		server.Client(),
	)
}

func runCompose(t *testing.T, plugin *ComposePlugin, args ...string) (int, string, error) {
	t.Helper()
	var stdout bytes.Buffer
	code, err := plugin.Run(args, bytes.NewReader(nil), &stdout, io.Discard)
	return code, stdout.String(), err
}

func TestVersionResolvesFrameworkAndUsesFrameworkAuthentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/compose/versions" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		user, secret, ok := r.BasicAuth()
		if !ok || user != "synthetic-user" || secret != "synthetic-secret" {
			t.Fatalf("unexpected auth: %q %q %t", user, secret, ok)
		}
		io.WriteString(w, `{"version":"synthetic-1.0"}`)
	}))
	defer server.Close()

	code, output, err := runCompose(t, newComposeForServer(server), "version", "synthetic-compose")
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if output != "{\"version\":\"synthetic-1.0\"}\n" {
		t.Fatalf("output=%q", output)
	}
}

func TestListRendersFrameworkTasks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/compose/v0/tasks" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		io.WriteString(w, `[{"TaskID":"synthetic-task-id","task_name":"synthetic:project:service","State":"TASK_RUNNING","MesosAgent":{"hostname":"agent.example.test"}}]`)
	}))
	defer server.Close()

	code, output, err := runCompose(t, newComposeForServer(server), "list", "synthetic-compose")
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	for _, expected := range []string{"synthetic-task-id", "synthetic:project:service", "TASK_RUNNING", "agent.example.test"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("missing %q in %q", expected, output)
		}
	}
}

func TestInfoPrintsResolvedFramework(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("info must not call framework API")
	}))
	defer server.Close()
	code, output, err := runCompose(t, newComposeForServer(server), "info", "synthetic-compose")
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if !strings.Contains(output, server.URL) || !strings.Contains(output, "synthetic-compose-id") {
		t.Fatalf("output=%q", output)
	}
}

func TestLaunchSendsComposeFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPut || r.URL.Path != "/api/compose/v0/synthetic-project" || string(body) != "services:\n  synthetic: {}\n" {
			t.Fatalf("unexpected request: %s %s %q", r.Method, r.URL.Path, body)
		}
		io.WriteString(w, `{"result":"launched"}`)
	}))
	defer server.Close()
	composeFile := filepath.Join(t.TempDir(), "compose.yaml")
	if err := os.WriteFile(composeFile, []byte("services:\n  synthetic: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	code, output, err := runCompose(t, newComposeForServer(server), "launch", "synthetic-compose", "synthetic-project", composeFile)
	if err != nil || code != 0 || output != "{\n  \"result\": \"launched\"\n}\n" {
		t.Fatalf("code=%d output=%q err=%v", code, output, err)
	}
}

func TestUpdateSendsComposeFileWithUpdateMethod(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != "UPDATE" || r.URL.Path != "/api/compose/v0/synthetic-project" || string(body) != "services:\n  synthetic: {}\n" {
			t.Fatalf("unexpected request: %s %s %q", r.Method, r.URL.Path, body)
		}
		io.WriteString(w, `{"result":"updated"}`)
	}))
	defer server.Close()
	composeFile := filepath.Join(t.TempDir(), "compose.yaml")
	if err := os.WriteFile(composeFile, []byte("services:\n  synthetic: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	code, output, err := runCompose(t, newComposeForServer(server), "update", "synthetic-compose", "synthetic-project", composeFile)
	if err != nil || code != 0 || output != "{\n  \"result\": \"updated\"\n}\n" {
		t.Fatalf("code=%d output=%q err=%v", code, output, err)
	}
}

func TestKillTaskUsesTaskRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/compose/v0/tasks/synthetic-task-id" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		io.WriteString(w, `{"result":"killed"}`)
	}))
	defer server.Close()
	code, output, err := runCompose(t, newComposeForServer(server), "kill", "synthetic-compose", "synthetic-task-id")
	if err != nil || code != 0 || !strings.Contains(output, "killed") {
		t.Fatalf("code=%d output=%q err=%v", code, output, err)
	}
}

func TestKillServiceUsesProjectServiceRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/compose/v0/synthetic-project/synthetic-service" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		io.WriteString(w, `{"result":"killed"}`)
	}))
	defer server.Close()
	code, output, err := runCompose(t, newComposeForServer(server), "kill", "synthetic-compose", "synthetic:synthetic-project:synthetic-service")
	if err != nil || code != 0 || !strings.Contains(output, "killed") {
		t.Fatalf("code=%d output=%q err=%v", code, output, err)
	}
}

func TestRestartRejectsTaskIDBecauseFrameworkOnlyRestartsServices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("task restart must be rejected before an HTTP request")
	}))
	defer server.Close()
	code, _, err := runCompose(t, newComposeForServer(server), "restart", "synthetic-compose", "synthetic-task-id")
	if err == nil || code != 1 || !strings.Contains(err.Error(), "service name") {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

func TestRestartServiceUsesProjectServiceRestartRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/compose/v0/synthetic-project/synthetic-service/restart" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		io.WriteString(w, `{"result":"restarted"}`)
	}))
	defer server.Close()
	code, output, err := runCompose(t, newComposeForServer(server), "restart", "synthetic-compose", "synthetic:synthetic-project:synthetic-service")
	if err != nil || code != 0 || !strings.Contains(output, "restarted") {
		t.Fatalf("code=%d output=%q err=%v", code, output, err)
	}
}

func TestFrameworkReregisterUsesReregisterRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/compose/v0/framework/reregister" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		io.WriteString(w, `{"result":"reregistered"}`)
	}))
	defer server.Close()
	code, output, err := runCompose(t, newComposeForServer(server), "framework", "synthetic-compose", "reregister")
	if err != nil || code != 0 || !strings.Contains(output, "reregistered") {
		t.Fatalf("code=%d output=%q err=%v", code, output, err)
	}
}

func TestFrameworkSuppressUsesCorrectlySpelledRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/compose/v0/framework/suppress" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		io.WriteString(w, `{"result":"suppressed"}`)
	}))
	defer server.Close()
	code, output, err := runCompose(t, newComposeForServer(server), "framework", "synthetic-compose", "suppress")
	if err != nil || code != 0 || !strings.Contains(output, "suppressed") {
		t.Fatalf("code=%d output=%q err=%v", code, output, err)
	}
}

func TestListReportsNoRunningTasks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `[]`)
	}))
	defer server.Close()
	code, output, err := runCompose(t, newComposeForServer(server), "list", "synthetic-compose")
	if err != nil || code != 0 || output != "There are no tasks running in the cluster.\n" {
		t.Fatalf("code=%d output=%q err=%v", code, output, err)
	}
}
