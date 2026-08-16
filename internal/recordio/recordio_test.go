package recordio

import (
	"bytes"
	"testing"
)

func TestEncodePrefixesByteLength(t *testing.T) {
	got := Encode([]byte("world!"))
	if want := []byte("6\nworld!"); !bytes.Equal(got, want) {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestDecoderBuffersFragmentedAndMultipleRecords(t *testing.T) {
	decoder := NewDecoder()
	if records, err := decoder.Decode([]byte("5\nhe")); err != nil || len(records) != 0 {
		t.Fatalf("first chunk: %v %q", err, records)
	}
	records, err := decoder.Decode([]byte("llo6\nworld!"))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || string(records[0]) != "hello" || string(records[1]) != "world!" {
		t.Fatalf("unexpected records: %q", records)
	}
}

func TestDecoderRemainsFailedAfterInvalidHeader(t *testing.T) {
	decoder := NewDecoder()
	if _, err := decoder.Decode([]byte("x\n")); err == nil {
		t.Fatal("expected invalid length")
	}
	if _, err := decoder.Decode([]byte("1\na")); err == nil || err.Error() != "Decoder is in a FAILED state" {
		t.Fatalf("unexpected error: %v", err)
	}
}
