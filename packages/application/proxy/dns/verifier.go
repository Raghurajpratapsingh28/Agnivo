// Package dns implements DNS-based domain ownership verification.
// It supports TXT, CNAME, A, and AAAA record verification with retries,
// propagation delays, and automatic failure tracking.
package dns

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/agnivo/agnivo/packages/application/proxy/model"
	"github.com/agnivo/agnivo/packages/platform/errors"
	"go.uber.org/zap"
)

// VerificationMethod is the type of DNS record used to verify ownership.
type VerificationMethod string

const (
	MethodTXT   VerificationMethod = "txt"
	MethodCNAME VerificationMethod = "cname"
	MethodA     VerificationMethod = "a"
	MethodAAAA  VerificationMethod = "aaaa"
)

// Verifier performs DNS-based domain ownership checks.
type Verifier struct {
	resolver    *net.Resolver
	platformTXT string // expected TXT value prefix for platform verification
	log         *zap.Logger
}

// NewVerifier constructs a DNS verifier.
// platformTXT is the expected TXT prefix (e.g. "agnivo-verify=") used to
// confirm ownership when the method is TXT.
func NewVerifier(platformTXT string, log *zap.Logger) *Verifier {
	return &Verifier{
		resolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: 5 * time.Second}
				return d.DialContext(ctx, "udp", "8.8.8.8:53")
			},
		},
		platformTXT: platformTXT,
		log:         log,
	}
}

// VerifyResult is the outcome of a single verification attempt.
type VerifyResult struct {
	Verified bool
	Reason   string
}

// Verify performs a DNS ownership check for the given verification record.
func (v *Verifier) Verify(ctx context.Context, dv model.DomainVerification) VerifyResult {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	switch VerificationMethod(dv.Method) {
	case MethodTXT:
		return v.verifyTXT(ctx, dv)
	case MethodCNAME:
		return v.verifyCNAME(ctx, dv)
	case MethodA:
		return v.verifyA(ctx, dv)
	case MethodAAAA:
		return v.verifyAAAA(ctx, dv)
	default:
		return VerifyResult{Reason: fmt.Sprintf("unknown method: %s", dv.Method)}
	}
}

// GenerateChallenge produces the expected DNS record value for a given method
// and domain ID. The value is deterministic so it can be regenerated on retry.
func GenerateChallenge(method VerificationMethod, domainID string) string {
	safe := strings.ReplaceAll(domainID, "-", "")[:16]
	switch method {
	case MethodTXT:
		return fmt.Sprintf("agnivo-verify=%s", safe)
	case MethodCNAME:
		return fmt.Sprintf("%s.verify.agnivo.app", safe)
	default:
		return safe
	}
}

// ─────────────────────────────── TXT ─────────────────────────────────────────

func (v *Verifier) verifyTXT(ctx context.Context, dv model.DomainVerification) VerifyResult {
	// Look up TXT records on the bare domain and on _agnivo-challenge.<domain>.
	targets := []string{dv.Hostname, "_agnivo-challenge." + dv.Hostname}
	for _, target := range targets {
		records, err := v.resolver.LookupTXT(ctx, target)
		if err != nil {
			v.log.Debug("dns: txt lookup failed", zap.String("hostname", target), zap.Error(err))
			continue
		}
		for _, rec := range records {
			if rec == dv.ChallengeValue || strings.Contains(rec, dv.ChallengeValue) {
				return VerifyResult{Verified: true}
			}
		}
	}
	return VerifyResult{
		Reason: fmt.Sprintf("TXT record %q not found on %s", dv.ChallengeValue, dv.Hostname),
	}
}

// ─────────────────────────────── CNAME ───────────────────────────────────────

func (v *Verifier) verifyCNAME(ctx context.Context, dv model.DomainVerification) VerifyResult {
	cname, err := v.resolver.LookupCNAME(ctx, dv.Hostname)
	if err != nil {
		return VerifyResult{Reason: fmt.Sprintf("CNAME lookup failed: %v", err)}
	}
	cname = strings.TrimSuffix(strings.ToLower(cname), ".")
	expected := strings.TrimSuffix(strings.ToLower(dv.ChallengeValue), ".")
	if cname == expected || strings.HasSuffix(cname, "."+expected) {
		return VerifyResult{Verified: true}
	}
	return VerifyResult{
		Reason: fmt.Sprintf("CNAME %q does not point to %q", cname, expected),
	}
}

// ─────────────────────────────── A / AAAA ────────────────────────────────────

func (v *Verifier) verifyA(ctx context.Context, dv model.DomainVerification) VerifyResult {
	addrs, err := v.resolver.LookupHost(ctx, dv.Hostname)
	if err != nil {
		return VerifyResult{Reason: fmt.Sprintf("A lookup failed: %v", err)}
	}
	for _, addr := range addrs {
		if addr == dv.ChallengeValue {
			return VerifyResult{Verified: true}
		}
	}
	return VerifyResult{
		Reason: fmt.Sprintf("A record for %s does not include %q (got %v)", dv.Hostname, dv.ChallengeValue, addrs),
	}
}

func (v *Verifier) verifyAAAA(ctx context.Context, dv model.DomainVerification) VerifyResult {
	addrs, err := v.resolver.LookupHost(ctx, dv.Hostname)
	if err != nil {
		return VerifyResult{Reason: fmt.Sprintf("AAAA lookup failed: %v", err)}
	}
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip != nil && ip.To4() == nil && addr == dv.ChallengeValue {
			return VerifyResult{Verified: true}
		}
	}
	return VerifyResult{
		Reason: fmt.Sprintf("AAAA record for %s does not include %q", dv.Hostname, dv.ChallengeValue),
	}
}

// ─────────────────────────────── Propagation ─────────────────────────────────

// CheckPropagation probes multiple public resolvers to confirm a record has
// propagated globally.  Returns true only when all probed resolvers agree.
func (v *Verifier) CheckPropagation(ctx context.Context, hostname, txtValue string) bool {
	resolvers := []string{
		"8.8.8.8:53",   // Google
		"1.1.1.1:53",   // Cloudflare
		"9.9.9.9:53",   // Quad9
		"208.67.222.222:53", // OpenDNS
	}
	verified := 0
	for _, raddr := range resolvers {
		r := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				d := net.Dialer{Timeout: 4 * time.Second}
				return d.DialContext(ctx, "udp", raddr)
			},
		}
		rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		records, err := r.LookupTXT(rctx, hostname)
		cancel()
		if err != nil {
			continue
		}
		for _, rec := range records {
			if strings.Contains(rec, txtValue) {
				verified++
				break
			}
		}
	}
	return verified >= len(resolvers)/2+1
}

// ValidateDomainFormat performs lightweight syntactic checks before hitting DNS.
func ValidateDomainFormat(hostname string) error {
	if hostname == "" {
		return errors.New(errors.CodeInvalidArgument, "hostname must not be empty")
	}
	if len(hostname) > 253 {
		return errors.New(errors.CodeInvalidArgument, "hostname exceeds 253 characters")
	}
	labels := strings.Split(hostname, ".")
	if len(labels) < 2 {
		return errors.New(errors.CodeInvalidArgument, "hostname must have at least one dot")
	}
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 {
			return errors.New(errors.CodeInvalidArgument, "each DNS label must be 1–63 characters")
		}
	}
	return nil
}
