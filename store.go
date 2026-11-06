package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
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

// OpenStore opens (or creates) the log file at path, replays it to rebuild
// the data, and then appends every later change to it.
func OpenStore(path string) (*Store, error) {
	s := NewStore()
	validLen, err := s.replay(path)
	if err != nil {
		return nil, err
	}
	// Cut off a half-written last line so new appends start on a clean line.
	if info, statErr := os.Stat(path); statErr == nil && info.Size() > validLen {
		if err := os.Truncate(path, validLen); err != nil {
			return nil, fmt.Errorf("truncate torn tail: %w", err)
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log: %w", err)
	}
	s.log = f
	return s, nil
}

// replay reads the log from the top and applies each command to the map.
// It returns how many bytes of the file are valid.
//
// A missing file just means an empty store. A last line with no trailing
// newline is a torn write (the crash hit mid-append, before Put returned),
// so it is ignored and not counted as valid. A complete line that does not
// decode is real corruption and is an error.
func (s *Store) replay(path string) (validLen int64, err error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("open log for replay: %w", err)
	}
	defer f.Close()

	// bufio.Reader (not Scanner) so very long values don't hit a line limit.
	r := bufio.NewReader(f)
	for lineNo := 1; ; lineNo++ {
		line, rerr := r.ReadBytes('\n')
		if rerr == io.EOF {
			return validLen, nil // anything left in line is an unterminated torn tail
		}
		if rerr != nil {
			return 0, fmt.Errorf("read log: %w", rerr)
		}
		if len(bytes.TrimSpace(line)) > 0 {
			c, derr := DecodeCommand(line)
			if derr != nil {
				return 0, fmt.Errorf("log line %d: %w", lineNo, derr)
			}
			s.apply(c)
		}
		validLen += int64(len(line))
	}
}

// apply changes the map only; it never touches the log.
func (s *Store) apply(c Command) {
	switch c.Op {
	case OpPut:
		s.data[c.Key] = c.Value
	case OpDelete:
		delete(s.data, c.Key)
	}
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
	// Write only hands the bytes to the OS. Sync forces them onto the disk,
	// so a Put that returned nil survives a power cut.
	if err := s.log.Sync(); err != nil {
		return fmt.Errorf("sync log: %w", err)
	}
	return nil
}

// Put saves a value under a key (overwrites if it exists).
// The change is logged first; if logging fails the map is left untouched.
func (s *Store) Put(key, value string) error {
	if err := s.appendLog(Command{Op: OpPut, Key: key, Value: value}); err != nil {
		return err
	}
	s.apply(Command{Op: OpPut, Key: key, Value: value})
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
	s.apply(Command{Op: OpDelete, Key: key})
	return nil
}

// Close closes the log file, if there is one.
func (s *Store) Close() error {
	if s.log == nil {
		return nil
	}
	return s.log.Close()
}
