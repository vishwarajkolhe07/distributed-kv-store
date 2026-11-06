package main

import (
	"fmt"
	"os"
)

// Store is our key-value database.
type Store struct {
	data map[string]string
	log  *os.File // nil for a purely in-memory store
}

// NewStore creates an empty in-memory store (nothing is saved to disk).
func NewStore() *Store {
	return &Store{data: make(map[string]string)}
}

// NewStoreWithLog creates an empty store that appends every change to the
// log file at path (creating it if needed). It does not read existing
// contents yet; replay on startup comes in the next chunk.
func NewStoreWithLog(path string) (*Store, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log: %w", err)
	}
	return &Store{data: make(map[string]string), log: f}, nil
}

// appendLog writes one command as one line to the log. No-op without a log.
func (s *Store) appendLog(c Command) error {
	if s.log == nil {
		return nil
	}
	line, err := c.Encode()
	if err != nil {
		return err
	}
	if _, err := s.log.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write log: %w", err)
	}
	return nil
}

// Put saves a value under a key (overwrites if it exists).
// The change is logged first; if logging fails the map is left untouched.
func (s *Store) Put(key, value string) error {
	if err := s.appendLog(Command{Op: OpPut, Key: key, Value: value}); err != nil {
		return err
	}
	s.data[key] = value
	return nil
}

// Get returns the value for a key, and whether the key exists.
func (s *Store) Get(key string) (string, bool) {
	value, ok := s.data[key]
	return value, ok
}

// Delete removes a key. Deleting a missing key does nothing to the data,
// but is still logged.
func (s *Store) Delete(key string) error {
	if err := s.appendLog(Command{Op: OpDelete, Key: key}); err != nil {
		return err
	}
	delete(s.data, key)
	return nil
}

// Close closes the log file, if there is one.
func (s *Store) Close() error {
	if s.log == nil {
		return nil
	}
	return s.log.Close()
}
