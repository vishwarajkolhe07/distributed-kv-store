package main

// Store is our key-value database.
type Store struct {
	data map[string]string
}

// NewStore creates an empty store.
func NewStore() *Store {
	return &Store{data: make(map[string]string)}
}

// Put saves a value under a key (overwrites if it exists).
func (s *Store) Put(key, value string) {
	s.data[key] = value
}

// Get returns the value for a key, and whether the key exists.
func (s *Store) Get(key string) (string, bool) {
	value, ok := s.data[key]
	return value, ok
}

// Delete removes a key. Deleting a missing key does nothing.
func (s *Store) Delete(key string) {
	delete(s.data, key)
}
