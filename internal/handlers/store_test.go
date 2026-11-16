package handlers

import (
	"strings"
	"testing"
	"time"
)

func TestStoreEvictsOldestWhenFull(t *testing.T) {
	s := newStore[int](3, time.Hour)
	for i, id := range []string{"a", "b", "c", "d", "e"} {
		s.put(id, i)
	}
	for _, id := range []string{"a", "b"} {
		if _, ok := s.get(id); ok {
			t.Errorf("%q should have been evicted", id)
		}
	}
	for i, id := range []string{"c", "d", "e"} {
		if v, ok := s.get(id); !ok || v != i+2 {
			t.Errorf("get(%q) = %v, %v", id, v, ok)
		}
	}
	if len(s.entries) != 3 || len(s.order) != 3 {
		t.Errorf("store holds %d entries / %d ids, want 3", len(s.entries), len(s.order))
	}
}

func TestStoreExpiresEntries(t *testing.T) {
	s := newStore[string](10, time.Minute)
	clock := time.Unix(1700000000, 0)
	s.now = func() time.Time { return clock }

	s.put("old", "x")
	clock = clock.Add(59 * time.Second)
	if _, ok := s.get("old"); !ok {
		t.Fatal("entry expired too early")
	}

	clock = clock.Add(2 * time.Second)
	if _, ok := s.get("old"); ok {
		t.Error("expired entry still readable")
	}

	// The next write removes it for real, freeing the memory.
	s.put("new", "y")
	if len(s.entries) != 1 {
		t.Errorf("store holds %d entries after expiry, want 1", len(s.entries))
	}
}

func TestNewIDIsUnpredictable(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := newID("img_")
		if !strings.HasPrefix(id, "img_") || len(id) != len("img_")+32 {
			t.Fatalf("unexpected ID format %q", id)
		}
		if seen[id] {
			t.Fatalf("duplicate ID %q", id)
		}
		seen[id] = true
	}
}
