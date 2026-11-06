package main

import (
	"bytes"
	"testing"
)

func TestCommandRoundTrip(t *testing.T) {
	cmds := []Command{
		{Op: OpPut, Key: "alice", Value: "500"},
		{Op: OpDelete, Key: "alice"},
		{Op: OpPut, Key: "empty", Value: ""},
		{Op: OpPut, Key: "has space", Value: "line1\nline2\t\"quoted\""},
		{Op: OpPut, Key: "unicode-ключ", Value: "値"},
	}
	for _, want := range cmds {
		line, err := want.Encode()
		if err != nil {
			t.Fatalf("Encode(%+v): %v", want, err)
		}
		if bytes.Contains(line, []byte("\n")) {
			t.Errorf("Encode(%+v) contains a raw newline: %q", want, line)
		}
		got, err := DecodeCommand(line)
		if err != nil {
			t.Fatalf("DecodeCommand(%q): %v", line, err)
		}
		if got != want {
			t.Errorf("round trip: got %+v, want %+v", got, want)
		}
	}
}

func TestEncodeUnknownOp(t *testing.T) {
	if _, err := (Command{Op: "bogus", Key: "k"}).Encode(); err == nil {
		t.Error("Encode with unknown op should fail")
	}
}

func TestDecodeBadInput(t *testing.T) {
	bad := []string{
		``,
		`not json`,
		`{"op":"put","key":"a"`,    // truncated line, like a crash mid-write
		`{"op":"bogus","key":"a"}`, // valid JSON, unknown op
	}
	for _, line := range bad {
		if _, err := DecodeCommand([]byte(line)); err == nil {
			t.Errorf("DecodeCommand(%q) should fail", line)
		}
	}
}
