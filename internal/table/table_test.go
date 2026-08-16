package table

import "testing"

func TestStringMatchesPythonCLIFormatting(t *testing.T) {
	tbl, err := New([]string{"Agent ID", "Hostname", "Active"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tbl.AddRow([]string{"agent-1", "node.example.test", "True"}); err != nil {
		t.Fatal(err)
	}
	want := "Agent ID  Hostname           Active  \nagent-1   node.example.test  True    "
	if got := tbl.String(); got != want {
		t.Fatalf("table mismatch\nwant: %q\n got: %q", want, got)
	}
}

func TestAddRowRejectsWrongColumnCount(t *testing.T) {
	tbl, _ := New([]string{"A", "B"})
	if err := tbl.AddRow([]string{"one"}); err == nil || err.Error() != "Number of entries and columns do not match!" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewRejectsTripleSpacesInHeader(t *testing.T) {
	if _, err := New([]string{"bad   header"}); err == nil {
		t.Fatal("expected invalid header error")
	}
}
