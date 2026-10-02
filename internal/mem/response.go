// Package mem provides typed wrappers for HelixDB query responses,
// eliminating map[string]any access patterns throughout the codebase.
package mem

import "math"

// Response wraps a raw HelixDB query response with typed accessors.
// It handles both flat arrays and {"properties": [...]} wrapped results.
type Response struct {
	raw map[string]any
}

// NewResponse creates a Response from the raw map returned by HelixDB.
func NewResponse(raw map[string]any) *Response {
	if raw == nil {
		return &Response{raw: make(map[string]any)}
	}

	return &Response{raw: raw}
}

// Raw returns the underlying raw response for direct access.
func (r *Response) Raw() map[string]any {
	return r.raw
}

// Nodes returns all nodes for the given result key.
// Handles flat arrays (e.g. TextSearch/VectorSearch results) and
// {"properties": [...]} wrapped results (e.g. ValueMap traversals).
func (r *Response) Nodes(key string) []Node {
	items := extractResultItems(r.raw[key])

	nodes := make([]Node, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			nodes = append(nodes, Node{data: m})
		}
	}

	return nodes
}

// Count returns the integer count for the given key.
// HelixDB returns Count results as {"key": {"count": N}}.
//
// Values that cannot be represented are reported as 0 for the same reason as
// Node.Uint64: an overflowing float-to-int conversion is implementation-defined
// and yields garbage instead of failing.
func (r *Response) Count(key string) int {
	obj, ok := r.raw[key].(map[string]any)
	if !ok {
		return 0
	}

	count, ok := obj["count"].(float64)
	if !ok || math.IsNaN(count) || count < 0 || count >= math.Ldexp(1, 63) {
		return 0
	}

	return int(count)
}

// Has reports whether the given key is present in the response.
func (r *Response) Has(key string) bool {
	_, ok := r.raw[key]

	return ok
}

// Node represents a single result node from a HelixDB query.
// It provides typed accessors for common HelixDB value types.
type Node struct {
	data map[string]any
}

// String returns the string value for the given property.
func (n Node) String(key string) string {
	s, _ := n.data[key].(string)

	return s
}

// Float64 returns the float64 value for the given property.
func (n Node) Float64(key string) float64 {
	f, _ := n.data[key].(float64)

	return f
}

// Uint64 returns the uint64 value for the given property.
// HelixDB encodes integers as float64 in JSON, so this converts.
//
// Values that cannot be represented are reported as 0. Go leaves float-to-
// integer conversions that overflow implementation-defined, and in practice
// they silently produce garbage rather than failing: uint64(-1) is
// 18446744073709551615 on amd64, and NaN converts to 9223372036854775808.
// NaN must be tested explicitly because every comparison against it is false.
func (n Node) Uint64(key string) uint64 {
	f, ok := n.data[key].(float64)
	// 2^64 is the first float64 value that no longer fits a uint64. float64
	// cannot represent 2^64-1 exactly, so this is the only correct cutoff.
	// It also rejects +Inf; f < 0 rejects -Inf and every negative value.
	if !ok || math.IsNaN(f) || f < 0 || f >= math.Ldexp(1, 64) {
		return 0
	}

	return uint64(f)
}

// Bool returns the bool value for the given property.
func (n Node) Bool(key string) bool {
	b, _ := n.data[key].(bool)

	return b
}

// Raw returns the underlying map for direct access when needed.
func (n Node) Raw() map[string]any {
	return n.data
}
