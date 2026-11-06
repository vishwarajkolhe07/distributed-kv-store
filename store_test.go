package main

import "testing"

func TestPutGet(t *testing.T) {
	s := NewStore()
	s.Put("alice", "500")

	v, ok := s.Get("alice")
	if !ok || v != "500" {
		t.Errorf("Get(alice) = %q, %v; want \"500\", true", v, ok)
	}
}

func TestGetMissing(t *testing.T) {
	s := NewStore()

	_, ok := s.Get("nobody")
	if ok {
		t.Errorf("Get(nobody) should not exist")
	}
}

func TestDelete(t *testing.T) {
	s := NewStore()
	s.Put("alice", "500")
	s.Delete("alice")

	_, ok := s.Get("alice")
	if ok {
		t.Errorf("alice should be gone after Delete")
	}
}
