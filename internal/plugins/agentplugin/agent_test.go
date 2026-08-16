package agentplugin

import (
	"bytes"
	"mesos-cli/internal/mesos"
	"testing"
)

type fakeClient struct {
	agents []mesos.Agent
	err    error
}

func (f fakeClient) Agents() ([]mesos.Agent, error) { return f.agents, f.err }

func TestListMatchesPythonColumnsAndValues(t *testing.T) {
	p := New(fakeClient{agents: []mesos.Agent{{ID: "agent-1", Hostname: "node.example.test", Active: true}}})
	var out bytes.Buffer
	code, err := p.Run([]string{"list"}, bytes.NewReader(nil), &out, &bytes.Buffer{})
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	want := "Agent ID  Hostname           Active  \nagent-1   node.example.test  True    \n"
	if out.String() != want {
		t.Fatalf("want %q got %q", want, out.String())
	}
}
func TestListReportsEmptyCluster(t *testing.T) {
	p := New(fakeClient{})
	var out bytes.Buffer
	_, err := p.Run([]string{"list"}, bytes.NewReader(nil), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "The cluster does not have any agents.\n" {
		t.Fatalf("out=%q", out.String())
	}
}
