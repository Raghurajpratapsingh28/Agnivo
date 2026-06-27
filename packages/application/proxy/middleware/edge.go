// Package middleware provides reusable HTTP middleware for the edge layer:
// rate-limiting, real-IP extraction, security headers, request IDs,
// correlation IDs, CORS, WebSocket upgrade detection, and SSE support.
package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/cache/redis"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// realIPHeaders lists the headers consulted (in order) when extracting the
// real client IP behind a trusted reverse-proxy.
var realIPHeaders = []string{
	"CF-Connecting-IP",
	"X-Real-IP",
	"X-Forwarded-For",
}

// RealIP extracts the originating client IP from forwarded headers and stores
// it in the request context. It only consults headers when the connection
// arrives from a trusted CIDR.
func RealIP(trustedCIDRs []string) func(http.Handler) http.Handler {
	trusted := parseCIDRs(trustedCIDRs)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			remoteHost, _, _ := net.SplitHostPort(r.RemoteAddr)
			remoteIP := net.ParseIP(remoteHost)
			if remoteIP != nil && isTrusted(remoteIP, trusted) {
				for _, hdr := range realIPHeaders {
					if val := r.Header.Get(hdr); val != "" {
						// X-Forwarded-For may be a comma-list; take the leftmost.
						parts := strings.SplitN(val, ",", 2)
						ip := strings.TrimSpace(parts[0])
						if net.ParseIP(ip) != nil {
							r = r.WithContext(withClientIP(r.Context(), ip))
							break
						}
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// EdgeSecurityHeaders sets conservative security response headers suitable for
// a public-facing edge proxy.
func EdgeSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("X-XSS-Protection", "0")
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		next.ServeHTTP(w, r)
	})
}

// EdgeRequestID assigns a unique request ID and propagates it in both
// directions. Compatible with the platform middleware.RequestID but adds the
// edge correlation header used by downstream containers.
func EdgeRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = uuid.NewString()
		}
		corrID := r.Header.Get("X-Correlation-ID")
		if corrID == "" {
			corrID = id
		}
		// Propagate to upstream containers via request headers.
		r.Header.Set("X-Request-ID", id)
		r.Header.Set("X-Correlation-ID", corrID)
		// Echo to downstream clients via response headers.
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("X-Correlation-ID", corrID)
		next.ServeHTTP(w, r)
	})
}

// WebSocketUpgrade detects WebSocket upgrade requests and sets the appropriate
// Upgrade and Connection headers so Caddy (and Go HTTP servers) handle them.
func WebSocketUpgrade(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isWebSocketUpgrade(r) {
			w.Header().Set("Connection", "Upgrade")
			w.Header().Set("Upgrade", "websocket")
		}
		next.ServeHTTP(w, r)
	})
}

// StreamingSupport sets headers that prevent response buffering so SSE and
// streaming responses reach the client in real time.
func StreamingSupport(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Accel-Buffering", "no")
		next.ServeHTTP(w, r)
	})
}

// RateLimit is a Redis token-bucket rate limiter keyed by client IP.
// limit is the maximum number of requests per second (burst = limit).
func RateLimit(redisClient *redis.Client, limit int, _ time.Duration, log *zap.Logger) func(http.Handler) http.Handler {
	if redisClient == nil {
		// No Redis: fall through without limiting.
		return func(next http.Handler) http.Handler { return next }
	}
	// ratePerSec = limit (treat limit as requests/second).
	bucket := redisClient.NewTokenBucket(limit, float64(limit))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := ClientIP(r.Context())
			if ip == "" {
				host, _, _ := net.SplitHostPort(r.RemoteAddr)
				ip = host
			}
			key := "ratelimit:edge:" + ip
			result, err := bucket.Allow(r.Context(), key)
			if err != nil {
				// On Redis error, let the request through.
				log.Warn("edge: rate limit check failed", zap.String("ip", ip), zap.Error(err))
			} else if !result.Allowed {
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ─────────────────────────────── In-memory rate limiter (fallback) ───────────

type inMemoryBucket struct {
	count   int
	resetAt time.Time
}

// InMemoryRateLimit is a simple in-process rate limiter for environments
// without Redis. Not suitable for multi-instance deployments.
func InMemoryRateLimit(limit int, window time.Duration) func(http.Handler) http.Handler {
	var mu sync.Mutex
	buckets := make(map[string]*inMemoryBucket)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := ClientIP(r.Context())
			if ip == "" {
				host, _, _ := net.SplitHostPort(r.RemoteAddr)
				ip = host
			}
			now := time.Now()
			mu.Lock()
			b, ok := buckets[ip]
			if !ok || now.After(b.resetAt) {
				b = &inMemoryBucket{resetAt: now.Add(window)}
				buckets[ip] = b
			}
			b.count++
			count := b.count
			mu.Unlock()
			if count > limit {
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ─────────────────────────────── Context helpers ──────────────────────────────

type contextKey int

const clientIPKey contextKey = iota

func withClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, clientIPKey, ip)
}

// ClientIP returns the real client IP stored in context by RealIP middleware.
func ClientIP(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPKey).(string)
	return ip
}

// ─────────────────────────────── Helpers ─────────────────────────────────────

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

func parseCIDRs(cidrs []string) []*net.IPNet {
	var out []*net.IPNet
	for _, c := range cidrs {
		_, network, err := net.ParseCIDR(c)
		if err == nil {
			out = append(out, network)
		}
	}
	// Always trust loopback and link-local.
	for _, c := range []string{"127.0.0.0/8", "::1/128", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"} {
		_, n, _ := net.ParseCIDR(c)
		out = append(out, n)
	}
	return out
}

func isTrusted(ip net.IP, trusted []*net.IPNet) bool {
	for _, network := range trusted {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
