package mem

import (
	"testing"
)

func TestNewResponse(t *testing.T) {
	t.Parallel()

	t.Run("nil input creates empty response", func(t *testing.T) {
		t.Parallel()

		r := NewResponse(nil)
		if r == nil {
			t.Fatal("NewResponse(nil) returned nil")
		}

		if len(r.Raw()) != 0 {
			t.Errorf("NewResponse(nil) raw map should be empty, got %v", r.Raw())
		}
	})

	t.Run("non-nil input preserved", func(t *testing.T) {
		t.Parallel()

		raw := map[string]any{"key": "value"}

		r := NewResponse(raw)
		if r.Raw()["key"] != "value" {
			t.Errorf("NewResponse() raw = %v, want key=value", r.Raw())
		}
	})
}

func TestResponseHas(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  map[string]any
		key  string
		want bool
	}{
		{name: "key present", raw: map[string]any{"a": 1}, key: "a", want: true},
		{name: "key absent", raw: map[string]any{"a": 1}, key: "b", want: false},
		{name: "nil value present", raw: map[string]any{"a": nil}, key: "a", want: true},
		{name: "empty map", raw: map[string]any{}, key: "x", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := NewResponse(tt.raw)
			if got := r.Has(tt.key); got != tt.want {
				t.Errorf("Has(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestResponseCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  map[string]any
		key  string
		want int
	}{
		{name: "count present", raw: map[string]any{"total": map[string]any{"count": 42.0}}, key: "total", want: 42},
		{name: "count float truncation", raw: map[string]any{"total": map[string]any{"count": 3.7}}, key: "total", want: 3},
		{name: "key missing", raw: map[string]any{}, key: "total", want: 0},
		{name: "value not a map", raw: map[string]any{"total": "string"}, key: "total", want: 0},
		{name: "count field missing", raw: map[string]any{"total": map[string]any{"other": 1}}, key: "total", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := NewResponse(tt.raw)
			if got := r.Count(tt.key); got != tt.want {
				t.Errorf("Count(%q) = %d, want %d", tt.key, got, tt.want)
			}
		})
	}
}

func TestResponseNodes(t *testing.T) {
	t.Parallel()

	t.Run("flat array of maps", func(t *testing.T) {
		t.Parallel()

		r := NewResponse(map[string]any{
			"results": []any{
				map[string]any{"name": "alice"},
				map[string]any{"name": "bob"},
			},
		})

		nodes := r.Nodes("results")
		if len(nodes) != 2 {
			t.Fatalf("Nodes() returned %d nodes, want 2", len(nodes))
		}

		if nodes[0].String("name") != "alice" {
			t.Errorf("Nodes[0].String(name) = %q, want alice", nodes[0].String("name"))
		}
	})

	t.Run("wrapped properties format", func(t *testing.T) {
		t.Parallel()

		r := NewResponse(map[string]any{
			"results": map[string]any{
				"properties": []any{
					map[string]any{"id": "1"},
					map[string]any{"id": "2"},
				},
			},
		})

		nodes := r.Nodes("results")
		if len(nodes) != 2 {
			t.Fatalf("Nodes() returned %d nodes, want 2", len(nodes))
		}
	})

	t.Run("key missing returns empty", func(t *testing.T) {
		t.Parallel()

		r := NewResponse(map[string]any{})

		nodes := r.Nodes("nonexistent")
		if len(nodes) != 0 {
			t.Errorf("Nodes() for missing key = %d, want 0", len(nodes))
		}
	})

	t.Run("non-map items are skipped", func(t *testing.T) {
		t.Parallel()

		r := NewResponse(map[string]any{
			"results": []any{
				map[string]any{"x": 1},
				"string item",
				42,
			},
		})

		nodes := r.Nodes("results")
		if len(nodes) != 1 {
			t.Errorf("Nodes() = %d, want 1 (only map items)", len(nodes))
		}
	})
}

func TestNodeString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data map[string]any
		key  string
		want string
	}{
		{name: "string value", data: map[string]any{"name": "hello"}, key: "name", want: "hello"},
		{name: "wrong type returns empty", data: map[string]any{"num": 42}, key: "num", want: ""},
		{name: "missing key returns empty", data: map[string]any{}, key: "x", want: ""},
		{name: "empty string", data: map[string]any{"s": ""}, key: "s", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			n := Node{data: tt.data}
			if got := n.String(tt.key); got != tt.want {
				t.Errorf("String(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestNodeFloat64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data map[string]any
		key  string
		want float64
	}{
		{name: "float64 value", data: map[string]any{"f": 3.14}, key: "f", want: 3.14},
		{name: "integer stored as float64", data: map[string]any{"n": 42.0}, key: "n", want: 42.0},
		{name: "wrong type returns zero", data: map[string]any{"s": "text"}, key: "s", want: 0},
		{name: "missing key returns zero", data: map[string]any{}, key: "x", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			n := Node{data: tt.data}
			if got := n.Float64(tt.key); got != tt.want {
				t.Errorf("Float64(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestNodeUint64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data map[string]any
		key  string
		want uint64
	}{
		{name: "converts from float64", data: map[string]any{"id": 123.0}, key: "id", want: 123},
		{name: "float truncation", data: map[string]any{"id": 3.9}, key: "id", want: 3},
		{name: "wrong type returns zero", data: map[string]any{"s": "text"}, key: "s", want: 0},
		{name: "missing key returns zero", data: map[string]any{}, key: "x", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			n := Node{data: tt.data}
			if got := n.Uint64(tt.key); got != tt.want {
				t.Errorf("Uint64(%q) = %d, want %d", tt.key, got, tt.want)
			}
		})
	}
}

func TestNodeBool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data map[string]any
		key  string
		want bool
	}{
		{name: "true", data: map[string]any{"active": true}, key: "active", want: true},
		{name: "false", data: map[string]any{"active": false}, key: "active", want: false},
		{name: "wrong type returns false", data: map[string]any{"n": 1}, key: "n", want: false},
		{name: "missing key returns false", data: map[string]any{}, key: "x", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			n := Node{data: tt.data}
			if got := n.Bool(tt.key); got != tt.want {
				t.Errorf("Bool(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestNodeRaw(t *testing.T) {
	t.Parallel()

	data := map[string]any{"key": "val"}

	n := Node{data: data}
	if got := n.Raw(); got["key"] != "val" {
		t.Errorf("Raw() = %v, want key=val", got)
	}
}
