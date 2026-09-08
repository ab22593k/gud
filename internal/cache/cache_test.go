package cache

import (
	"testing"
	"time"
)

func TestCache_HitMissAndEviction(t *testing.T) {
	t.Parallel()

	c := New[string, string](2, time.Minute)

	if _, ok := c.Get("a"); ok {
		t.Fatal("Get(missing)=hit, want miss")
	}

	if c.Misses() != 1 {
		t.Fatalf("Misses=%d, want 1", c.Misses())
	}

	c.Set("a", "1")
	c.Set("b", "2")

	if v, ok := c.Get("a"); !ok || v != "1" {
		t.Fatalf("Get(a)=(%q,%v), want (1,true)", v, ok)
	}

	if c.Hits() != 1 {
		t.Fatalf("Hits=%d, want 1", c.Hits())
	}

	c.Set("c", "3") // Evicts least-recently-used ("b").

	if _, ok := c.Get("b"); ok {
		t.Fatal("Get(b) after eviction=hit, want miss (LRU)")
	}

	if c.Len() != 2 {
		t.Fatalf("Len=%d, want 2", c.Len())
	}
}

func TestCache_TTLExpiry(t *testing.T) {
	t.Parallel()

	c := New[string, string](8, 20*time.Millisecond)

	c.Set("x", "1")

	time.Sleep(40 * time.Millisecond)

	if _, ok := c.Get("x"); ok {
		t.Fatal("Get(x) after TTL=hit, want miss (expired)")
	}
}
