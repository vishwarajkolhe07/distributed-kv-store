package main

import "fmt"

func main() {
	s := NewStore()

	s.Put("alice", "500")
	s.Put("bob", "300")

	v, ok := s.Get("alice")
	fmt.Println("alice:", v, ok) // alice: 500 true

	s.Delete("alice")

	v, ok = s.Get("alice")
	fmt.Println("alice after delete:", v, ok) // alice after delete:  false
}