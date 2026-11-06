package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}

func TestCompactShrinksLogAndKeepsData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kv.log")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 200; i++ {
		s.Put("counter", fmt.Sprint(i)) // 200 overwrites of one key
	}
	s.Put("keep", "yes")
	s.Put("gone", "temp")
	s.Delete("gone")

	before := fileSize(t, path)
	if err := s.Compact(); err != nil {
		t.Fatal(err)
	}
	after := fileSize(t, path)
	if after >= before {
		t.Errorf("log did not shrink: %d -> %d bytes", before, after)
	}

	// Exactly one line per live key: counter and keep.
	if cmds := readLog(t, path); len(cmds) != 2 {
		t.Errorf("compacted log has %d commands, want 2: %+v", len(cmds), cmds)
	}
	s.Close()

	s2, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	for k, want := range map[string]string{"counter": "199", "keep": "yes"} {
		if v, ok := s2.Get(k); !ok || v != want {
			t.Errorf("Get(%s) = %q, %v; want %q, true", k, v, ok, want)
		}
	}
	if _, ok := s2.Get("gone"); ok {
		t.Error("deleted key came back after compaction")
	}
}

func TestWritesAfterCompactAreKept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kv.log")
	s, _ := OpenStore(path)
	s.Put("a", "1")
	s.Put("a", "2")
	if err := s.Compact(); err != nil {
		t.Fatal(err)
	}
	if err := s.Put("b", "3"); err != nil { // must append to the NEW file
		t.Fatal(err)
	}
	s.Delete("a")
	s.Close()

	s2, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if v, ok := s2.Get("b"); !ok || v != "3" {
		t.Errorf("Get(b) = %q, %v; want \"3\", true", v, ok)
	}
	if _, ok := s2.Get("a"); ok {
		t.Error("a was deleted after compaction and should stay deleted")
	}
}

func TestCompactTwiceIsStable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kv.log")
	s, _ := OpenStore(path)
	defer s.Close()
	s.Put("a", "1")
	s.Put("b", "2")
	s.Compact()
	first, _ := os.ReadFile(path)
	s.Compact()
	second, _ := os.ReadFile(path)
	if string(first) != string(second) {
		t.Errorf("compacting twice changed the file:\n%s\nvs\n%s", first, second)
	}
}

func TestCompactEmptyStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kv.log")
	s, _ := OpenStore(path)
	s.Put("a", "1")
	s.Delete("a")
	if err := s.Compact(); err != nil {
		t.Fatal(err)
	}
	if size := fileSize(t, path); size != 0 {
		t.Errorf("log of an empty store should be empty, got %d bytes", size)
	}
	s.Close()
	s2, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	s2.Close()
}

func TestCompactInMemoryStoreIsNoop(t *testing.T) {
	s := NewStore()
	s.Put("a", "1")
	if err := s.Compact(); err != nil {
		t.Errorf("Compact on an in-memory store should do nothing: %v", err)
	}
	if v, _ := s.Get("a"); v != "1" {
		t.Error("data lost")
	}
}

func TestStaleTmpFromCrashedCompactIsIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kv.log")
	s, _ := OpenStore(path)
	s.Put("a", "1")
	s.Close()

	// Pretend a crash during Compact left a half-written tmp file behind.
	writeFile(t, path+".tmp", `{"op":"put","key":"a","va`)

	s2, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if v, ok := s2.Get("a"); !ok || v != "1" {
		t.Errorf("Get(a) = %q, %v; want \"1\", true", v, ok)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("stale .tmp file should have been removed")
	}
}
