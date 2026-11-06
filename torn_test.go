package main

import (
	"os"
	"path/filepath"
	"testing"
)

const goodLine = `{"op":"put","key":"a","value":"1"}` + "\n"

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTornLastLineIsIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kv.log")
	// A good command, then a second one cut off mid-write.
	writeFile(t, path, goodLine+`{"op":"put","key":"b","val`)

	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore should survive a torn last line: %v", err)
	}
	defer s.Close()

	if v, ok := s.Get("a"); !ok || v != "1" {
		t.Errorf("Get(a) = %q, %v; want \"1\", true", v, ok)
	}
	if _, ok := s.Get("b"); ok {
		t.Error("the half-written command must not be applied")
	}
}

func TestTornTailIsRemovedFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kv.log")
	writeFile(t, path, goodLine+`{"op":"put","key":"b","val`)

	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	// If the torn bytes were left in place, this append would glue onto them
	// and corrupt the line.
	if err := s.Put("c", "3"); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s2, err := OpenStore(path)
	if err != nil {
		t.Fatalf("reopen after append past a torn tail: %v", err)
	}
	defer s2.Close()
	for k, want := range map[string]string{"a": "1", "c": "3"} {
		if v, ok := s2.Get(k); !ok || v != want {
			t.Errorf("Get(%s) = %q, %v; want %q, true", k, v, ok, want)
		}
	}
	if _, ok := s2.Get("b"); ok {
		t.Error("b was never completed and should not exist")
	}
}

func TestUnterminatedButValidLastLineIsDropped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kv.log")
	// Complete JSON but the newline never made it: the write was not finished,
	// so Put never returned success for it.
	writeFile(t, path, goodLine+`{"op":"put","key":"b","value":"2"}`)

	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, ok := s.Get("b"); ok {
		t.Error("a line without its newline was not fully written and must be dropped")
	}
}

func TestEveryTruncationPointRecovers(t *testing.T) {
	// Simulate a crash at every possible byte of a three-command log.
	full := goodLine +
		`{"op":"put","key":"b","value":"2"}` + "\n" +
		`{"op":"delete","key":"a"}` + "\n"

	for n := 0; n <= len(full); n++ {
		path := filepath.Join(t.TempDir(), "kv.log")
		writeFile(t, path, full[:n])

		s, err := OpenStore(path)
		if err != nil {
			t.Fatalf("crash after %d bytes: OpenStore failed: %v", n, err)
		}
		s.Close()
	}
}

func TestCorruptionInTheMiddleStillFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kv.log")
	// A bad line with good data after it is not a torn write; refuse to guess.
	writeFile(t, path, goodLine+"garbage\n"+goodLine)

	if _, err := OpenStore(path); err == nil {
		t.Error("OpenStore should fail on corruption that is not at the tail")
	}
}
