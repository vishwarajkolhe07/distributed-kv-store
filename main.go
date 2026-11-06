package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
)

// A tiny interactive shell for trying the store by hand.
//
//	go run . [-log kv.log]
//
// Commands: put <key> <value...> | get <key> | delete <key> | quit
func main() {
	path := flag.String("log", "kv.log", "path to the log file")
	flag.Parse()

	s, err := OpenStore(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open:", err)
		os.Exit(1)
	}
	defer s.Close()

	fmt.Printf("opened %s (put <k> <v> | get <k> | delete <k> | quit)\n", *path)
	in := bufio.NewScanner(os.Stdin)
	for fmt.Print("> "); in.Scan(); fmt.Print("> ") {
		f := strings.Fields(in.Text())
		if len(f) == 0 {
			continue
		}
		switch strings.ToLower(f[0]) {
		case "put":
			if len(f) < 3 {
				fmt.Println("usage: put <key> <value>")
				continue
			}
			// Value is the rest of the line, so it may contain spaces.
			value := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(in.Text())[len(f[0]):], " "))
			value = strings.TrimSpace(strings.TrimPrefix(value, f[1]))
			if err := s.Put(f[1], value); err != nil {
				fmt.Println("error:", err)
				continue
			}
			fmt.Println("ok")
		case "get":
			if len(f) != 2 {
				fmt.Println("usage: get <key>")
				continue
			}
			if v, ok := s.Get(f[1]); ok {
				fmt.Println(v)
			} else {
				fmt.Println("(not found)")
			}
		case "delete":
			if len(f) != 2 {
				fmt.Println("usage: delete <key>")
				continue
			}
			if err := s.Delete(f[1]); err != nil {
				fmt.Println("error:", err)
				continue
			}
			fmt.Println("ok")
		case "compact":
			if err := s.Compact(); err != nil {
				fmt.Println("error:", err)
				continue
			}
			fmt.Println("ok")
		case "quit", "exit":
			return
		default:
			fmt.Println("unknown command; try put, get, delete, quit")
		}
	}
}
