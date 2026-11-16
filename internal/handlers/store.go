package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// store is a small in-memory cache with two limits: entries expire after a
// fixed lifetime, and when the store is full the oldest entry is evicted.
//
// The server keeps uploads and results only so the browser can fetch them in
// follow-up requests. Without both limits every upload would stay in memory
// for the life of the process.
type store[T any] struct {
	mu       sync.Mutex
	entries  map[string]storeEntry[T]
	order    []string // insertion order, oldest first
	capacity int
	lifetime time.Duration
	now      func() time.Time // replaceable clock, for tests
}

type storeEntry[T any] struct {
	value   T
	expires time.Time
}

func newStore[T any](capacity int, lifetime time.Duration) *store[T] {
	return &store[T]{
		entries:  make(map[string]storeEntry[T]),
		capacity: capacity,
		lifetime: lifetime,
		now:      time.Now,
	}
}

// put stores value under id, evicting expired entries and, if the store is
// still full, the oldest ones.
func (s *store[T]) put(id string, value T) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()

	// Entries are in insertion order and share one lifetime, so the expired
	// ones are always at the front.
	drop := 0
	for drop < len(s.order) {
		entry, ok := s.entries[s.order[drop]]
		if ok && now.Before(entry.expires) && len(s.order)-drop < s.capacity {
			break
		}
		delete(s.entries, s.order[drop])
		drop++
	}
	s.order = append(s.order[drop:], id)
	s.entries[id] = storeEntry[T]{value: value, expires: now.Add(s.lifetime)}
}

// get returns the value stored under id, if present and not expired.
func (s *store[T]) get(id string) (T, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.entries[id]
	if !ok || !s.now().Before(entry.expires) {
		var zero T
		return zero, false
	}
	return entry.value, true
}

// newID returns prefix followed by 128 random bits in hex. IDs are the only
// thing protecting one user's upload from another, so they must not be
// guessable; a counter would let anyone enumerate them.
func newID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return prefix + hex.EncodeToString(b[:])
}
