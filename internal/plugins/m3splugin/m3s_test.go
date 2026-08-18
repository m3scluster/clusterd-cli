package m3splugin

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mesos-cli/internal/config"
	"mesos-cli/internal/mesos"
)

type fakeFrameworkClient struct{ frameworks []mesos.Framework }

func (f fakeFrameworkClient) Frameworks() ([]mesos.Framework, error) { return f.frameworks, nil }

type fakeCredentials struct{ credentials config.FrameworkCredentials }

func (f fakeCredentials) FrameworkCredentials(pluginName, frameworkName string) (config.FrameworkCredentials, error) {
	if pluginName != "m3s" || frameworkName != "synthetic-m3s" {
		return config.FrameworkCredentials{}, nil
	}
	return f.credentials, nil
}

func newM3S(frameworks []mesos.Framework, httpClient *http.Client) *M3SPlugin {
	return New(fakeFrameworkClient{frameworks: frameworks}, fakeCredentials{config.FrameworkCredentials{
		Principal: "synthetic-user", Secret: "synthetic-secret", SSLVerify: true,
	}}, httpClient)
}

func frameworkFor(server *httptest.Server) mesos.Framework {
	return mesos.Framework{ID: "synthetic-m3s-id", Name: "synthetic-m3s", Active: true, WebUIURL: server.URL}
}

func runM3S(t *testing.T, plugin *M3SPlugin, args ...string) (int, string, error) {
	t.Helper()
	var stdout bytes.Buffer
	code, err := plugin.Run(args, bytes.NewReader(nil), &stdout, io.Discard)
	return code, stdout.String(), err
}

func TestListFiltersInactiveFrameworksUnlessAllIsSet(t *testing.T) {
	plugin := newM3S([]mesos.Framework{
		{ID: "synthetic-active-id", Name: "synthetic-m3s-active", Active: true, WebUIURL: "http://active.example.test"},
		{ID: "synthetic-inactive-id", Name: "synthetic-m3s-inactive", Active: false, WebUIURL: "http://inactive.example.test"},
		{ID: "synthetic-other-id", Name: "synthetic-other", Active: true, WebUIURL: "http://other.example.test"},
	}, http.DefaultClient)

	code, output, err := runM3S(t, plugin, "list")
	if err != nil || code != 0 || !strings.Contains(output, "synthetic-active-id") || strings.Contains(output, "synthetic-inactive-id") || strings.Contains(output, "synthetic-other-id") {
		t.Fatalf("code=%d output=%q err=%v", code, output, err)
	}
	code, output, err = runM3S(t, plugin, "list", "--all")
	if err != nil || code != 0 || !strings.Contains(output, "synthetic-inactive-id") {
		t.Fatalf("--all code=%d output=%q err=%v", code, output, err)
	}
}

func TestKubeconfigReadsAuthenticatedFrameworkEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/m3s/v0/server/config" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		user, secret, ok := r.BasicAuth()
		if !ok || user != "synthetic-user" || secret != "synthetic-secret" {
			t.Fatalf("unexpected auth: %q %q %t", user, secret, ok)
		}
		io.WriteString(w, "synthetic-kubeconfig")
	}))
	defer server.Close()
	code, output, err := runM3S(t, newM3S([]mesos.Framework{frameworkFor(server)}, server.Client()), "kubeconfig", "synthetic-m3s")
	if err != nil || code != 0 || output != "synthetic-kubeconfig\n" {
		t.Fatalf("code=%d output=%q err=%v", code, output, err)
	}
}

func TestVersionReadsServerVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/m3s/v0/server/version" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		io.WriteString(w, `{"version":"synthetic-k8s"}`)
	}))
	defer server.Close()
	code, output, err := runM3S(t, newM3S([]mesos.Framework{frameworkFor(server)}, server.Client()), "version", "synthetic-m3s")
	if err != nil || code != 0 || !strings.Contains(output, "synthetic-k8s") {
		t.Fatalf("code=%d output=%q err=%v", code, output, err)
	}
}

func TestStatusM3SReadsM3SStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/m3s/v0/status/m3s" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		io.WriteString(w, "synthetic-m3s-status")
	}))
	defer server.Close()
	code, output, err := runM3S(t, newM3S([]mesos.Framework{frameworkFor(server)}, server.Client()), "status", "synthetic-m3s", "--m3s")
	if err != nil || code != 0 || output != "synthetic-m3s-status\n" {
		t.Fatalf("code=%d output=%q err=%v", code, output, err)
	}
}

func TestStatusCanReturnBothM3SAndKubernetesStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/m3s/v0/status/m3s":
			io.WriteString(w, "synthetic-m3s-status")
		case "/api/m3s/v0/status/k8s":
			io.WriteString(w, "synthetic-k8s-status")
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	code, output, err := runM3S(t, newM3S([]mesos.Framework{frameworkFor(server)}, server.Client()), "status", "synthetic-m3s", "--m3s", "--kubernetes")
	if err != nil || code != 0 || output != "synthetic-m3s-status\nsynthetic-k8s-status\n" {
		t.Fatalf("code=%d output=%q err=%v", code, output, err)
	}
}

func TestScaleAgentUsesAgentScaleRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/m3s/v0/agent/scale/3" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		io.WriteString(w, "synthetic-agent-scaled")
	}))
	defer server.Close()
	code, output, err := runM3S(t, newM3S([]mesos.Framework{frameworkFor(server)}, server.Client()), "scale", "synthetic-m3s", "3", "--agent")
	if err != nil || code != 0 || output != "synthetic-agent-scaled\n" {
		t.Fatalf("code=%d output=%q err=%v", code, output, err)
	}
}

func TestScaleEtcdUsesCurrentDatastoreScaleRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/m3s/v0/datastore/scale/3" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		io.WriteString(w, "synthetic-datastore-scaled")
	}))
	defer server.Close()
	code, output, err := runM3S(t, newM3S([]mesos.Framework{frameworkFor(server)}, server.Client()), "scale", "synthetic-m3s", "3", "--etcd")
	if err != nil || code != 0 || output != "synthetic-datastore-scaled\n" {
		t.Fatalf("code=%d output=%q err=%v", code, output, err)
	}
}

func TestClusterStopUsesShutdownRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/m3s/v0/cluster/shutdown" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		io.WriteString(w, "synthetic-cluster-stopped")
	}))
	defer server.Close()
	code, output, err := runM3S(t, newM3S([]mesos.Framework{frameworkFor(server)}, server.Client()), "cluster", "synthetic-m3s", "stop")
	if err != nil || code != 0 || output != "synthetic-cluster-stopped\n" {
		t.Fatalf("code=%d output=%q err=%v", code, output, err)
	}
}

func TestClusterStartUsesStartRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/m3s/v0/cluster/start" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		io.WriteString(w, "synthetic-cluster-started")
	}))
	defer server.Close()
	code, output, err := runM3S(t, newM3S([]mesos.Framework{frameworkFor(server)}, server.Client()), "cluster", "synthetic-m3s", "start")
	if err != nil || code != 0 || output != "synthetic-cluster-started\n" {
		t.Fatalf("code=%d output=%q err=%v", code, output, err)
	}
}

func TestClusterRestartUsesRestartRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/m3s/v0/cluster/restart" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		io.WriteString(w, "synthetic-cluster-restarted")
	}))
	defer server.Close()
	code, output, err := runM3S(t, newM3S([]mesos.Framework{frameworkFor(server)}, server.Client()), "cluster", "synthetic-m3s", "restart")
	if err != nil || code != 0 || output != "synthetic-cluster-restarted\n" {
		t.Fatalf("code=%d output=%q err=%v", code, output, err)
	}
}
