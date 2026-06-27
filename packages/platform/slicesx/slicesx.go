// Package slicesx provides generic slice transformations not covered by the
// standard library slices package, focused on the map/filter/reduce operations
// that recur throughout the platform. Helpers preallocate their result slices
// to keep allocations to a single growth.
package slicesx

// Map applies fn to every element of in and returns the results in order. The
// output slice is preallocated to len(in).
func Map[T, U any](in []T, fn func(T) U) []U {
	if in == nil {
		return nil
	}
	out := make([]U, len(in))
	for i, v := range in {
		out[i] = fn(v)
	}
	return out
}

// Filter returns the elements of in for which keep returns true, preserving
// order. The result is allocated lazily so an all-reject filter allocates
// nothing.
func Filter[T any](in []T, keep func(T) bool) []T {
	var out []T
	for _, v := range in {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out
}

// Reduce folds in into a single accumulator value left-to-right.
func Reduce[T, A any](in []T, init A, fn func(A, T) A) A {
	acc := init
	for _, v := range in {
		acc = fn(acc, v)
	}
	return acc
}

// Contains reports whether target is present in in.
func Contains[T comparable](in []T, target T) bool {
	for _, v := range in {
		if v == target {
			return true
		}
	}
	return false
}

// Unique returns the distinct elements of in, preserving first-seen order.
func Unique[T comparable](in []T) []T {
	if in == nil {
		return nil
	}
	seen := make(map[T]struct{}, len(in))
	out := make([]T, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// Chunk splits in into contiguous slices of at most size elements. The final
// chunk may be shorter. A size <= 0 returns in as a single chunk.
func Chunk[T any](in []T, size int) [][]T {
	if size <= 0 {
		return [][]T{in}
	}
	out := make([][]T, 0, (len(in)+size-1)/size)
	for i := 0; i < len(in); i += size {
		end := i + size
		if end > len(in) {
			end = len(in)
		}
		out = append(out, in[i:end])
	}
	return out
}

// KeyBy indexes in by the key produced by fn. Later elements overwrite earlier
// ones on key collision.
func KeyBy[T any, K comparable](in []T, fn func(T) K) map[K]T {
	out := make(map[K]T, len(in))
	for _, v := range in {
		out[fn(v)] = v
	}
	return out
}

// GroupBy partitions in into buckets keyed by fn, preserving element order
// within each bucket.
func GroupBy[T any, K comparable](in []T, fn func(T) K) map[K][]T {
	out := make(map[K][]T)
	for _, v := range in {
		k := fn(v)
		out[k] = append(out[k], v)
	}
	return out
}
