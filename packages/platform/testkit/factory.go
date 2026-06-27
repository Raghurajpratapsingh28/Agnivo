package testkit

import (
	"fmt"
	"strings"

	"github.com/agnivo/agnivo/packages/platform/idx"
)

// RandomString returns a random alphanumeric string of length n.
func RandomString(n int) string {
	if n <= 0 {
		n = 8
	}
	return idx.Prefixed("t", n)
}

// RandomEmail returns a unique test email address.
func RandomEmail() string {
	return fmt.Sprintf("user-%s@test.local", idx.Hex(6))
}

// RandomSlug returns a URL-safe slug suitable for resource names.
func RandomSlug() string {
	return strings.ReplaceAll(RandomString(12), "_", "-")
}

// RandomUUID returns a random UUID string.
func RandomUUID() string { return idx.NewUUID() }
