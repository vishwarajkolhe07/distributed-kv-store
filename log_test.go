package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// readLog reads the log file and decodes every line.
func readLog(t *testing.T, path string) []Command {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	var cmds []Command
	for _, line := range bytes.Split(bytes.TrimSuffix(raw, []byte("\n")), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		c, err := DecodeCommand(line)
		if err != nil {
			t.Fatalf("decode %q: %v", line, err)
		}
		cmds = append(cmds, c)
	}
	return cmds
}

func TestOperationsAreLogged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kv.log")
	s, err := NewStoreWithLog(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Put("alice", "500")
	s.Put("bob", "300")
	s.Put("alice", "900")
	s.Delete("bob")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	want := []Command{
		{Op: OpPut, Key: "alice", Value: "500"},
		{Op: OpPut, Key: "bob", Value: "300"},
		{Op: OpPut, Key: "alice", Value: "900"},
		{Op: OpDelete, Key: "bob"},
	}
	got := readLog(t, path)
	if len(got) != len(want) {
		t.Fatalf("log has %d commands, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("log[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestLoggedStoreStillWorksInMemory(t *testing.T) {
	s, err := NewStoreWithLog(filepath.Join(t.TempDir(), "kv.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	s.Put("alice", "500")
	if v, ok := s.Get("alice"); !ok || v != "500" {
		t.Errorf("Get(alice) = %q, %v; want \"500\", true", v, ok)
	}
	s.Delete("alice")
	if _, ok := s.Get("alice"); ok {
		t.Error("alice should be gone after Delete")
	}
}

func TestLogAppendsAcrossOpens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kv.log")

	s1, _ := NewStoreWithLog(path)
	s1.Put("a", "1")
	s1.Close()

	s2, _ := NewStoreWithLog(path)
	s2.Put("b", "2")
	s2.Close()

	if got := readLog(t, path); len(got) != 2 {
		t.Errorf("log has %d commands, want 2 (second open must append, not truncate)", len(got))
	}
}

func TestPutFailsWhenLogFails(t *testing.T) {
	s, err := NewStoreWithLog(filepath.Join(t.TempDir(), "kv.log"))
	if err != nil {
		t.Fatal(err)
	}
	s.Close() // writing to a closed file now fails

	if err := s.Put("alice", "500"); err == nil {
		t.Fatal("Put should return an error when the log write fails")
	}
	if _, ok := s.Get("alice"); ok {
		t.Error("map must not change when the log write fails")
	}
}
