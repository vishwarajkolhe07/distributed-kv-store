package main

import (
	"encoding/json"
	"fmt"
)

// Op is the kind of change a Command makes.
type Op string

const (
	OpPut    Op = "put"
	OpDelete Op = "delete"
)

// Command is one change to the store. The log is an ordered list of these.
type Command struct {
	Op    Op     `json:"op"`
	Key   string `json:"key"`
	Value string `json:"value,omitempty"` // unused for delete
}

// Encode turns a Command into a single line of bytes (no trailing newline).
// JSON escapes newlines inside keys and values, so one command is always one line.
func (c Command) Encode() ([]byte, error) {
	if c.Op != OpPut && c.Op != OpDelete {
		return nil, fmt.Errorf("unknown op %q", c.Op)
	}
	return json.Marshal(c)
}

// DecodeCommand parses a line produced by Encode.
func DecodeCommand(line []byte) (Command, error) {
	var c Command
	if err := json.Unmarshal(line, &c); err != nil {
		return Command{}, fmt.Errorf("bad command: %w", err)
	}
	if c.Op != OpPut && c.Op != OpDelete {
		return Command{}, fmt.Errorf("unknown op %q", c.Op)
	}
	return c, nil
}
