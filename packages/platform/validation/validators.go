package validation

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// Regular expressions for the platform's custom validators. They are compiled
// once at package load for zero per-call allocation.
var (
	// slugRe matches lowercase, hyphen-separated identifiers (no leading/
	// trailing/double hyphens), suitable for URLs and resource names.
	slugRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

	// domainRe matches an RFC 1035-ish hostname with at least one dot and a TLD.
	domainRe = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)

	// envVarRe matches a POSIX environment variable name.
	envVarRe = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

	// dockerRefRe matches a Docker image reference: optional registry/host,
	// repository path, and optional :tag or @sha256:digest.
	dockerRefRe = regexp.MustCompile(`^(?:[a-z0-9.-]+(?::[0-9]+)?/)?[a-z0-9]+(?:[._/-][a-z0-9]+)*(?::[a-zA-Z0-9._-]+)?(?:@sha256:[a-f0-9]{64})?$`)
)

// IsUUID reports whether s is a valid UUID.
func IsUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

// IsSlug reports whether s is a valid URL-safe slug.
func IsSlug(s string) bool { return slugRe.MatchString(s) }

// IsDomain reports whether s is a syntactically valid domain name.
func IsDomain(s string) bool {
	return len(s) <= 253 && domainRe.MatchString(strings.ToLower(s))
}

// IsURL reports whether s is an absolute http(s) URL with a host.
func IsURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

// IsDockerImage reports whether s is a valid Docker image reference (registry,
// repository, tag, and/or digest).
func IsDockerImage(s string) bool {
	return s != "" && len(s) <= 255 && dockerRefRe.MatchString(s)
}

// IsGitRepo reports whether s is a plausible Git repository URL: an https(s)/git
// URL or scp-style SSH form (git@host:owner/repo.git).
func IsGitRepo(s string) bool {
	if s == "" {
		return false
	}
	// scp-style: user@host:path
	if scp := strings.SplitN(s, ":", 2); strings.Contains(scp[0], "@") && !strings.Contains(scp[0], "/") {
		return len(scp) == 2 && scp[1] != ""
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return false
	}
	switch u.Scheme {
	case "http", "https", "git", "ssh":
		return u.Path != "" && u.Path != "/"
	default:
		return false
	}
}

// IsEnvVarName reports whether s is a valid POSIX environment variable name.
func IsEnvVarName(s string) bool { return envVarRe.MatchString(s) }

// SecretStrength describes the minimum requirements for a secret value.
type SecretStrength struct {
	MinLength int
}

// DefaultSecretStrength requires at least 16 characters, a reasonable floor for
// API keys and tokens.
var DefaultSecretStrength = SecretStrength{MinLength: 16}

// IsStrongSecret reports whether s meets the default secret strength policy:
// sufficient length and a mix of character classes to resist guessing.
func IsStrongSecret(s string) bool { return isSecret(s, DefaultSecretStrength) }

func isSecret(s string, policy SecretStrength) bool {
	if len(s) < policy.MinLength {
		return false
	}
	var hasLower, hasUpper, hasDigit, hasSymbol bool
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= '0' && r <= '9':
			hasDigit = true
		default:
			hasSymbol = true
		}
	}
	// Require at least three of the four classes.
	classes := 0
	for _, ok := range []bool{hasLower, hasUpper, hasDigit, hasSymbol} {
		if ok {
			classes++
		}
	}
	return classes >= 3
}
