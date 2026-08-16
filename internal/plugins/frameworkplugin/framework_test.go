package frameworkplugin

import (
	"bytes"
	"mesos-cli/internal/mesos"
	"testing"
)

type fakeClient struct {
	frameworks []mesos.Framework
	err        error
}

func (f fakeClient) Frameworks() ([]mesos.Framework, error) { return f.frameworks, f.err }
func TestListFiltersInactiveUnlessAll(t *testing.T) {
	items := []mesos.Framework{{ID: "active-1", Active: true, Hostname: "host-a", Name: "A"}, {ID: "inactive-1", Active: false, Hostname: "host-b", Name: "B"}}
	p := New(fakeClient{frameworks: items})
	var out bytes.Buffer
	_, err := p.Run([]string{"list"}, bytes.NewReader(nil), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out.Bytes(), []byte("inactive-1")) {
		t.Fatalf("inactive included: %q", out.String())
	}
	out.Reset()
	_, err = p.Run([]string{"list", "--all"}, bytes.NewReader(nil), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("inactive-1")) {
		t.Fatalf("inactive missing: %q", out.String())
	}
}
func TestInspectRemovesTaskCollections(t *testing.T) {
	raw := map[string]any{"id": "framework-1", "name": "Synthetic", "tasks": []any{1}, "unreachable_tasks": []any{2}, "completed_tasks": []any{3}}
	p := New(fakeClient{frameworks: []mesos.Framework{{ID: "framework-1", Raw: raw}}})
	var out bytes.Buffer
	_, err := p.Run([]string{"inspect", "framework-1"}, bytes.NewReader(nil), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n    \"id\": \"framework-1\",\n    \"name\": \"Synthetic\"\n}\n"
	if out.String() != want {
		t.Fatalf("want %q got %q", want, out.String())
	}
}
