// Package ptr provides generic helpers for working with pointers, removing the
// boilerplate of taking addresses of literals and safely dereferencing values.
// All helpers are allocation-minimal and inlinable.
package ptr

// Of returns a pointer to v. It is the generic replacement for the common
// `x := v; return &x` pattern and works with any type.
func Of[T any](v T) *T { return &v }

// Deref returns the value pointed to by p, or the zero value of T when p is nil.
func Deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// DerefOr returns the value pointed to by p, or def when p is nil.
func DerefOr[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}

// Equal reports whether two pointers reference equal values. Two nil pointers
// are equal; a nil and non-nil pointer are not.
func Equal[T comparable](a, b *T) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return *a == *b
	}
}

// NonZero returns a pointer to v, or nil when v is the zero value of T. This is
// useful for building sparse JSON payloads that omit empty fields.
func NonZero[T comparable](v T) *T {
	var zero T
	if v == zero {
		return nil
	}
	return &v
}
