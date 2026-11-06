package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDataSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kv.log")

	s1, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	s1.Put("alice", "500")
	s1.Put("bob", "300")
	s1.Put("alice", "900") // overwrite
	s1.Delete("bob")       // delete
	s1.Close()

	s2, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	if v, ok := s2.Get("alice"); !ok || v != "900" {
		t.Errorf("Get(alice) = %q, %v; want \"900\", true", v, ok)
	}
	if _, ok := s2.Get("bob"); ok {
		t.Error("bob was deleted before restart and should stay deleted")
	}
}

func TestWritesAfterRestartAreKept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kv.log")

	s1, _ := OpenStore(path)
	s1.Put("a", "1")
	s1.Close()

	s2, _ := OpenStore(path)
	s2.Put("b", "2")
	s2.Close()

	s3, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s3.Close()
	for k, want := range map[string]string{"a": "1", "b": "2"} {
		if v, ok := s3.Get(k); !ok || v != want {
			t.Errorf("Get(%s) = %q, %v; want %q, true", k, v, ok, want)
		}
	}
}

func TestOpenMissingFileIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.log")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, ok := s.Get("anything"); ok {
		t.Error("new store should be empty")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("log file should be created: %v", err)
	}
}

func TestReplayLargeValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kv.log")
	big := strings.Repeat("x", 1<<20) // 1 MiB, well past bufio.Scanner's 64 KiB limit

	s1, _ := OpenStore(path)
	s1.Put("big", big)
	s1.Close()

	s2, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if v, ok := s2.Get("big"); !ok || v != big {
		t.Errorf("large value did not survive restart (ok=%v, len=%d)", ok, len(v))
	}
}

func TestOpenCorruptLogFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kv.log")
	content := `{"op":"put","key":"a","value":"1"}` + "\n" + "garbage\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenStore(path); err == nil {
		t.Error("OpenStore should fail on a corrupt line (chunk 4 will relax this for a torn last line)")
	}
}
