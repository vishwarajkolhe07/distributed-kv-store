package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
)

// Store is our key-value database.
type Store struct {
	data map[string]string
	path string   // log file path; empty for a purely in-memory store
	log  *os.File // open for appending; nil only if path is empty (or a Compact failed to reopen)
}

// NewStore creates an empty in-memory store (nothing is saved to disk).
func NewStore() *Store {
	return &Store{data: make(map[string]string)}
}

// OpenStore opens (or creates) the log file at path, replays it to rebuild
// the data, and then appends every later change to it.
func OpenStore(path string) (*Store, error) {
	s := NewStore()
	s.path = path
	os.Remove(path + ".tmp") // leftover from a crash during Compact; the real log is intact
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
	if s.path == "" {
		return nil
	}
	if s.log == nil {
		return errors.New("log is not open")
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

// Compact rewrites the log so it holds one put per live key and nothing
// else. Overwritten values and deleted keys disappear from the file.
//
// It is crash-safe: the new log is written to path+".tmp", fsynced, and then
// renamed over the old one. A crash at any point leaves either the complete
// old log or the complete new log.
func (s *Store) Compact() error {
	if s.path == "" {
		return nil // in-memory store, nothing to compact
	}
	tmp := s.path + ".tmp"

	if err := s.writeSnapshot(tmp); err != nil {
		os.Remove(tmp)
		return err
	}

	// Windows cannot replace a file that is open, so close the old log first.
	if s.log != nil {
		s.log.Close()
		s.log = nil
	}
	renameErr := os.Rename(tmp, s.path)
	if renameErr != nil {
		os.Remove(tmp)
	}

	// Reopen for appending whether or not the rename worked: after a failed
	// rename the old log is still the right one.
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("reopen log after compact: %w", err)
	}
	s.log = f
	if renameErr != nil {
		return fmt.Errorf("replace log: %w", renameErr)
	}
	return nil
}

// writeSnapshot writes one put per live key to a new file and fsyncs it.
// Keys are sorted so the output is deterministic.
func (s *Store) writeSnapshot(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create compacted log: %w", err)
	}
	defer f.Close()

	keys := make([]string, 0, len(s.data))
	for k := range s.data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	w := bufio.NewWriter(f)
	for _, k := range keys {
		line, err := Command{Op: OpPut, Key: k, Value: s.data[k]}.Encode()
		if err != nil {
			return err
		}
		w.Write(line)
		w.WriteByte('\n')
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("write compacted log: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync compacted log: %w", err)
	}
	return f.Close()
}

// Close closes the log file, if there is one.
func (s *Store) Close() error {
	if s.log == nil {
		return nil
	}
	return s.log.Close()
}
